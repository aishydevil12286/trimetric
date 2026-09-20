// Command mockfeed generates a synthetic transit network shaped like a large
// city bus system and serves it as GTFS plus GTFS-realtime.
//
// It exists so the app can be run and looked at without an Open Transit Data
// key, and so the ingestion path can be exercised against a feed big enough to
// be representative. Every coordinate in it is invented: the place names are
// real Delhi localities, but nothing here is Delhi's actual network.
//
//	go run ./scripts/mockfeed -zip data/mock-gtfs.zip
//
// then point the app at the generated zip and at this process:
//
//	go run ./cmd/trimetric -gtfs-static-file=data/mock-gtfs.zip \
//	  -vehicle-positions-url=http://localhost:8899/api/realtime/VehiclePositions.pb \
//	  -route-line-types=3 -api-key=mock
package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"time"

	rt "github.com/MobilityData/gtfs-realtime-bindings/golang/gtfs"
	"google.golang.org/protobuf/proto"
)

// bus is one simulated vehicle running a loop of its route.
type bus struct {
	ID      string
	RouteID string
	TripID  string
	Route   *route

	// phase is where on the route the bus started, as a fraction of the
	// route's length. period is how long a full traversal takes.
	phase   float64
	period  time.Duration
	reverse bool
}

// positionAt interpolates the bus along its route for a given time.
func (b *bus) positionAt(now time.Time) (point, float64) {
	elapsed := now.Sub(time.Unix(0, 0)).Seconds()
	frac := b.phase + elapsed/b.period.Seconds()
	if b.reverse {
		frac = -frac
	}
	pos, bearing := b.Route.Shape.at(frac)
	if b.reverse {
		bearing = float64(int(bearing+180) % 360)
	}
	return pos, bearing
}

func main() {
	zipPath := flag.String("zip", "data/mock-gtfs.zip", "Where to write the generated GTFS zip")
	addr := flag.String("addr", ":8899", "Address to serve the realtime feed on")
	seed := flag.Int64("seed", 7, "Random seed, so a given seed always builds the same network")
	radials := flag.Int("radials", 18, "Number of radial corridors")
	rings := flag.Int("rings", 3, "Number of ring roads")
	crosstown := flag.Int("crosstown", 10, "Number of cross-town routes")
	perRoute := flag.Int("buses-per-route", 10, "Vehicles running on each route")
	serveOnly := flag.Bool("serve-only", false, "Skip regenerating the zip and just serve the realtime feed")
	flag.Parse()

	log.SetFlags(0)

	net := buildNetwork(*seed, *radials, *rings, *crosstown)
	log.Printf("generated %d routes and %d stops", len(net.Routes), len(net.Stops))

	if !*serveOnly {
		if err := os.MkdirAll(filepath.Dir(*zipPath), 0o755); err != nil {
			log.Fatal(err)
		}
		trips, stopTimes, err := writeFeed(*zipPath, net, defaultSchedule)
		if err != nil {
			log.Fatal(err)
		}
		info, err := os.Stat(*zipPath)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("wrote %s: %d trips, %d stop times, %.1f MiB",
			*zipPath, trips, stopTimes, float64(info.Size())/(1<<20))
	}

	fleet := buildFleet(net, *perRoute, *seed)
	log.Printf("simulating %d vehicles", len(fleet))

	http.HandleFunc("/api/realtime/VehiclePositions.pb", func(w http.ResponseWriter, r *http.Request) {
		// The real portal rejects an unkeyed request, so this one does too;
		// it is the first thing to go wrong in a real setup.
		if r.URL.Query().Get("key") == "" {
			http.Error(w, "missing key", http.StatusUnauthorized)
			return
		}
		b, err := encodeFeed(fleet, time.Now())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.Write(b)
	})

	log.Printf("serving realtime vehicle positions on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

// buildFleet spreads vehicles evenly around each route, running in both
// directions and at slightly different speeds so they spread out over time.
func buildFleet(n *network, perRoute int, seed int64) []*bus {
	rnd := rand.New(rand.NewSource(seed + 1))

	var fleet []*bus
	for _, r := range n.Routes {
		for i := 0; i < perRoute; i++ {
			// A full loop takes between 25 and 45 minutes of simulated time,
			// which is fast enough that movement is visible between polls.
			period := time.Duration(25+rnd.Intn(20)) * time.Minute
			fleet = append(fleet, &bus{
				// Delhi registration plates look like DL1PC1234.
				ID:      fmt.Sprintf("DL1PC%04d", len(fleet)+1),
				RouteID: r.ID,
				TripID: fmt.Sprintf("T_%s_WD_%d_%03d",
					r.ID, i%2, rnd.Intn(30)),
				Route:   r,
				phase:   float64(i)/float64(perRoute) + rnd.Float64()*0.02,
				period:  period,
				reverse: i%2 == 1,
			})
		}
	}
	return fleet
}

// encodeFeed renders the fleet's current positions as a GTFS-realtime
// VehiclePositions message.
func encodeFeed(fleet []*bus, now time.Time) ([]byte, error) {
	ts := uint64(now.Unix())
	incrementality := rt.FeedHeader_FULL_DATASET
	version := "2.0"

	feed := &rt.FeedMessage{
		Header: &rt.FeedHeader{
			GtfsRealtimeVersion: &version,
			Incrementality:      &incrementality,
			Timestamp:           &ts,
		},
	}

	for _, b := range fleet {
		pos, bearing := b.positionAt(now)

		id, routeID, tripID := b.ID, b.RouteID, b.TripID
		lat, lng := float32(pos.Lat), float32(pos.Lng)
		bearing32 := float32(bearing)
		vts := ts

		feed.Entity = append(feed.Entity, &rt.FeedEntity{
			Id: &id,
			Vehicle: &rt.VehiclePosition{
				Trip:    &rt.TripDescriptor{TripId: &tripID, RouteId: &routeID},
				Vehicle: &rt.VehicleDescriptor{Id: &id, Label: &id},
				Position: &rt.Position{
					Latitude:  &lat,
					Longitude: &lng,
					Bearing:   &bearing32,
				},
				Timestamp: &vts,
			},
		})
	}

	return proto.Marshal(feed)
}
