package gtfs

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/pkg/errors"
)

// bomPrefix is the UTF-8 byte order mark.
const bomPrefix = "\ufeff"

// CSV reads one file out of a GTFS feed.
//
// Columns are addressed by header name, never by position. GTFS fixes the set
// of column names but not their order, and agencies omit any column they have
// no data for, so a feed's column layout is only discoverable from its header
// row.
type CSV struct {
	rc     io.ReadCloser
	cr     *csv.Reader
	header map[string]int
}

// Row is a single record, addressed by column name.
type Row struct {
	rec    []string
	header map[string]int
}

// Get returns the named column with surrounding whitespace trimmed, or "" if
// this feed does not supply the column.
func (r Row) Get(name string) string {
	i, ok := r.header[name]
	if !ok || i >= len(r.rec) {
		return ""
	}
	return strings.TrimSpace(r.rec[i])
}

// Has reports whether the feed supplies a value for the named column in this
// row. It distinguishes "column absent or empty" from "column present", which
// matters for GTFS fields whose default differs from their zero value.
func (r Row) Has(name string) bool {
	return r.Get(name) != ""
}

// Columns returns the column names this file declares.
func (c *CSV) Columns() []string {
	names := make([]string, 0, len(c.header))
	for name := range c.header {
		names = append(names, name)
	}
	return names
}

// Read returns the next record. It returns io.EOF when the file is exhausted.
func (c *CSV) Read() (Row, error) {
	rec, err := c.cr.Read()
	if err != nil {
		return Row{}, err
	}
	return Row{rec: rec, header: c.header}, nil
}

// Close releases the underlying file.
func (c *CSV) Close() error {
	if c.rc == nil {
		return nil
	}
	return c.rc.Close()
}

// newCSV consumes the header row and indexes it so rows can be read by column
// name.
func newCSV(rc io.ReadCloser, name string) (*CSV, error) {
	cr := csv.NewReader(rc)

	// Real feeds are ragged: trailing optional columns get dropped on some
	// rows, and free-text fields (stop names especially) contain bare quotes.
	// Neither should abort a load of 400k stop times.
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	cr.ReuseRecord = true

	head, err := cr.Read()
	if err == io.EOF {
		return nil, errors.Errorf("gtfs: %s is empty", name)
	} else if err != nil {
		return nil, errors.Wrapf(err, "gtfs: reading header of %s", name)
	}

	header := make(map[string]int, len(head))
	for i, col := range head {
		// Feeds exported from spreadsheets routinely carry a UTF-8 BOM on the
		// first column name, which would otherwise make it unmatchable.
		col = strings.TrimPrefix(col, bomPrefix)
		header[strings.ToLower(strings.TrimSpace(col))] = i
	}

	return &CSV{rc: rc, cr: cr, header: header}, nil
}

// ReadGTFSCSV opens a GTFS txt file from disk.
func ReadGTFSCSV(filename string) (*CSV, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	c, err := newCSV(f, filename)
	if err != nil {
		f.Close()
		return nil, err
	}
	return c, nil
}

// ReadZippedGTFSCSV extracts fileName from an open GTFS zip.
//
// It returns an error identifiable with IsFileMissing if the feed does not
// contain the file, so callers can skip optional files such as calendar.txt.
func ReadZippedGTFSCSV(z *zip.ReadCloser, fileName string) (*CSV, error) {
	var zf *zip.File
	for _, f := range z.File {
		// Some feeds nest their files inside a directory in the zip.
		if f.Name == fileName || strings.HasSuffix(f.Name, "/"+fileName) {
			zf = f
			break
		}
	}
	if zf == nil {
		return nil, &FileMissingError{Name: fileName}
	}

	rc, err := zf.Open()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	c, err := newCSV(rc, fileName)
	if err != nil {
		rc.Close()
		return nil, err
	}
	return c, nil
}

// FileMissingError reports that a feed does not contain a given file.
type FileMissingError struct {
	Name string
}

func (e *FileMissingError) Error() string {
	return fmt.Sprintf("gtfs: feed does not contain %s", e.Name)
}

// IsFileMissing reports whether err means a file was absent from the feed.
func IsFileMissing(err error) bool {
	var missing *FileMissingError
	return errors.As(err, &missing)
}

// StaticSource describes where to get a static GTFS zip from.
type StaticSource struct {
	// File is the path to an already-downloaded zip. When set, nothing is
	// fetched over the network and the other fields are ignored.
	File string

	// URL is the address to download the zip from.
	URL string

	// CSRF fetches the zip with a CSRF-protected POST instead of a GET. The
	// Open Transit Data portal serves its static feed from a Django form, so
	// a plain GET returns the HTML page rather than the zip.
	CSRF bool
}

// RequestGTFSFile obtains a static GTFS feed and opens it as a zip.
//
// The caller is responsible for closing the returned reader.
func RequestGTFSFile(src StaticSource) (*zip.ReadCloser, error) {
	if src.File != "" {
		z, err := zip.OpenReader(src.File)
		if err != nil {
			return nil, errors.Wrapf(err, "gtfs: opening static feed %s", src.File)
		}
		return z, nil
	}

	if src.URL == "" {
		return nil, errors.New("gtfs: no static feed configured; set a URL or a file path")
	}

	f, err := os.CreateTemp("", "gtfs-static-*.zip")
	if err != nil {
		return nil, errors.Wrap(err, "gtfs: creating temp file")
	}
	// The zip stays open on the path after we return, so it can only be
	// removed once the caller is done with it. Removing it now would break
	// zip.OpenReader below on platforms without unlink-on-open semantics, so
	// clean up only on the error paths.
	cleanup := func() {
		f.Close()
		os.Remove(f.Name())
	}

	resp, err := fetchStatic(src)
	if err != nil {
		cleanup()
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		cleanup()
		return nil, errors.Errorf("gtfs: downloading %s: unexpected status %s", src.URL, resp.Status)
	}

	// A portal that wants a login, or a CSRF POST we got wrong, answers 200
	// with an HTML page. Catch that here rather than reporting a corrupt zip.
	var head [2]byte
	n, err := io.ReadFull(resp.Body, head[:])
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		cleanup()
		return nil, errors.Wrapf(err, "gtfs: downloading %s", src.URL)
	}
	if n < 2 || string(head[:n]) != "PK" {
		cleanup()
		return nil, errors.Errorf(
			"gtfs: %s did not return a zip file (got %q); "+
				"the portal may require a login or a different fetch mode -- "+
				"download the feed by hand and point --gtfs-static-file at it",
			src.URL, string(head[:n]))
	}

	if _, err := f.Write(head[:n]); err != nil {
		cleanup()
		return nil, errors.WithStack(err)
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		cleanup()
		return nil, errors.Wrapf(err, "gtfs: downloading %s", src.URL)
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return nil, errors.WithStack(err)
	}

	z, err := zip.OpenReader(f.Name())
	if err != nil {
		os.Remove(f.Name())
		return nil, errors.Wrapf(err, "gtfs: opening downloaded feed from %s", src.URL)
	}
	return z, nil
}

// fetchStatic issues the request for a static feed, using a CSRF-protected
// POST when the source calls for one.
func fetchStatic(src StaticSource) (*http.Response, error) {
	if !src.CSRF {
		resp, err := http.Get(src.URL)
		return resp, errors.WithStack(err)
	}

	// Django's CSRF check compares the csrfmiddlewaretoken field against the
	// csrftoken cookie; it does not care what the value is, only that the two
	// agree. Over HTTPS it additionally requires a same-origin Referer.
	token, err := csrfToken()
	if err != nil {
		return nil, err
	}

	form := url.Values{"csrfmiddlewaretoken": {token}}
	req, err := http.NewRequest(http.MethodPost, src.URL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", src.URL)
	req.AddCookie(&http.Cookie{Name: "csrftoken", Value: token})

	resp, err := http.DefaultClient.Do(req)
	return resp, errors.WithStack(err)
}

// csrfToken returns a random 64-character token.
func csrfToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", errors.WithStack(err)
	}
	return hex.EncodeToString(b), nil
}

func getLines(r io.Reader) (int, error) {
	buf := make([]byte, 32*1024)
	count := 0
	lineSep := []byte{'\n'}

	for {
		c, err := r.Read(buf)
		count += bytes.Count(buf[:c], lineSep)

		switch {
		case err == io.EOF:
			return count, nil

		case err != nil:
			return count, err
		}
	}
}
