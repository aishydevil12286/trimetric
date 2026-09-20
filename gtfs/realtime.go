package gtfs

import (
	"io"
	"net/http"
	"net/url"

	rt "github.com/MobilityData/gtfs-realtime-bindings/golang/gtfs"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

// fetchFeed downloads and decodes a GTFS-realtime feed.
//
// apiKey is appended as a query parameter when set. Errors quote feedURL as
// it was configured rather than the URL actually requested, so the key never
// reaches a log.
func fetchFeed(feedURL, apiKey string) (*rt.FeedMessage, error) {
	if feedURL == "" {
		return nil, errors.New("gtfs: no realtime feed url configured")
	}

	u, err := url.Parse(feedURL)
	if err != nil {
		return nil, errors.Wrapf(err, "gtfs: bad feed url %q", feedURL)
	}
	if apiKey != "" {
		q := u.Query()
		q.Set(APIKeyParam, apiKey)
		u.RawQuery = q.Encode()
	}

	resp, err := http.Get(u.String())
	if err != nil {
		return nil, errors.Wrapf(err, "gtfs: requesting %s", feedURL)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// OTD answers 401/403 for a missing or rejected key, which is the
		// single most likely thing to go wrong on a first run.
		return nil, errors.Errorf("gtfs: %s returned %s (check the API key)", feedURL, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrapf(err, "gtfs: reading %s", feedURL)
	}

	feed := &rt.FeedMessage{}
	if err := proto.Unmarshal(body, feed); err != nil {
		return nil, errors.Wrapf(err, "gtfs: decoding %s", feedURL)
	}
	return feed, nil
}
