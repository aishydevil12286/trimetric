package trimet

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// RequestArrivals makes a request to the trimet arrivals API endpoint.
// It MUST have an array of ids as trimet limits reponses to 10 locations.
func RequestArrivals(apiKey string, ids []int) ([]byte, error) {
	query := url.Values{}

	var sids []string
	for _, id := range ids {
		sids = append(sids, strconv.Itoa(id))
	}

	query.Set("appID", apiKey)
	query.Set("locIDs", strings.Join(sids, ","))
	query.Set("json", "true")

	resp, err := http.Get(fmt.Sprintf("%s?%s", BaseTrimetURL+Arrivals, query.Encode()))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return b, nil
}
