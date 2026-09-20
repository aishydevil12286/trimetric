package logic

import (
	"archive/zip"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/bsdavidson/trimetric/gtfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// zipFixture packs a directory of GTFS .txt files into a zip and returns its
// path. Keeping the fixture as text in the repo means a reviewer can see what
// the feed under test actually contains.
func zipFixture(t *testing.T, dir string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "gtfs.zip")
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()

	w := zip.NewWriter(f)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		require.NoError(t, err)
		zf, err := w.Create(e.Name())
		require.NoError(t, err)
		_, err = zf.Write(b)
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	return path
}

func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow("SELECT count(*) FROM "+table).Scan(&n))
	return n
}

// TestLoadGTFSDataFromURL checks the download path, using TriMet's feed to
// confirm the header-driven loader still reads the layout it was written for.
func TestLoadGTFSDataFromURL(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	zd, err := os.ReadFile("./testdata/gtfs.zip")
	require.NoError(t, err)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(zd)
	}))
	defer ts.Close()

	lds := LoaderSQLDataset{DB: db}
	require.NoError(t, lds.LoadGTFSData(gtfs.StaticSource{URL: ts.URL}))

	assert.NotZero(t, countRows(t, db, "stops"))
	assert.NotZero(t, countRows(t, db, "routes"))
	assert.NotZero(t, countRows(t, db, "trips"))
}

// TestLoadDelhiFeed loads a feed shaped like Delhi's: columns in their own
// order, optional columns absent, service described by calendar.txt rather
// than calendar_dates.txt, a BOM on one header, and a couple of rows pointing
// at ids the feed never defines.
func TestLoadDelhiFeed(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	lds := LoaderSQLDataset{DB: db}
	require.NoError(t, lds.LoadGTFSData(gtfs.StaticSource{
		File: zipFixture(t, "./testdata/delhi"),
	}))

	assert.Equal(t, 3, countRows(t, db, "services"), "services come from calendar.txt")
	assert.Equal(t, 3, countRows(t, db, "calendar"))
	assert.Equal(t, 0, countRows(t, db, "calendar_dates"), "this feed has no calendar_dates.txt")
	assert.Equal(t, 3, countRows(t, db, "routes"))
	assert.Equal(t, 5, countRows(t, db, "stops"))
	assert.Equal(t, 5, countRows(t, db, "shapes"))

	// t_ghost names a route the feed never defines, so it is dropped along
	// with its stop time; one further stop time names an unknown stop.
	assert.Equal(t, 5, countRows(t, db, "trips"))
	assert.Equal(t, 10, countRows(t, db, "stop_times"))

	// The BOM on the stops.txt header must not end up in the column name, or
	// every stop would have come through with an empty id.
	var name string
	require.NoError(t, db.QueryRow(`SELECT name FROM stops WHERE id = 's_anand'`).Scan(&name))
	assert.Equal(t, "Anand Vihar ISBT", name)

	// A quoted field containing a comma stays one field.
	require.NoError(t, db.QueryRow(`SELECT name FROM stops WHERE id = 's_mehrauli'`).Scan(&name))
	assert.Equal(t, "Mehrauli Terminal, Gate 2", name)

	// Optional columns the feed omits fall back to usable defaults.
	var sortOrder int
	var textColor string
	require.NoError(t, db.QueryRow(`SELECT sort_order, text_color FROM routes WHERE id = '101'`).
		Scan(&sortOrder, &textColor))
	assert.Equal(t, 0, sortOrder)
	assert.Equal(t, "", textColor)
}

// TestServiceActive covers the calendar/calendar_dates precedence rules the
// arrivals query depends on.
func TestServiceActive(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	lds := LoaderSQLDataset{DB: db}
	require.NoError(t, lds.LoadGTFSData(gtfs.StaticSource{
		File: zipFixture(t, "./testdata/delhi"),
	}))

	tests := []struct {
		name    string
		service string
		date    string
		want    bool
	}{
		{"weekday service on a Wednesday", "WD", "2025-06-11", true},
		{"weekday service on a Sunday", "WD", "2025-06-15", false},
		{"weekend service on a Sunday", "WE", "2025-06-15", true},
		{"weekend service on a Wednesday", "WE", "2025-06-11", false},
		{"service outside its date range", "EXPIRED", "2025-06-11", false},
		{"service the feed never defines", "NOPE", "2025-06-11", false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var active bool
			require.NoError(t, db.QueryRow(
				`SELECT service_active($1, $2::date)`, test.service, test.date).Scan(&active))
			assert.Equal(t, test.want, active)
		})
	}
}

// TestLoadGTFSDataRejectsNonZip covers the case where a portal answers a
// static feed request with its HTML login page.
func TestLoadGTFSDataRejectsNonZip(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<!DOCTYPE html><html><body>Please sign in</body></html>"))
	}))
	defer ts.Close()

	lds := LoaderSQLDataset{DB: nil}
	err := lds.LoadGTFSData(gtfs.StaticSource{URL: ts.URL})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "did not return a zip file")
}
