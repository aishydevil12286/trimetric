package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bsdavidson/trimetric/logic"
	"github.com/bsdavidson/trimetric/trimet"
	"github.com/gorilla/websocket"
)

func httpError(w http.ResponseWriter, prefix string, err error, code int) {
	log.Println(prefix, err)
	http.Error(w, http.StatusText(code), code)
}

func commaSplit(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func commaSplitInts(s string) ([]int, error) {
	var nums []int
	for _, sn := range commaSplit(s) {
		n, err := strconv.Atoi(sn)
		if err != nil {
			return nil, err
		}
		nums = append(nums, n)
	}
	return nums, nil
}

type stopsWithDistanceResponse struct {
	Stops []logic.StopWithDistance `json:"stops"`
}

// HandleStops provides responses for the /api/v1/stops endpoint.
// It searches for stops within a specified distance from a lat/lon.
func HandleStops(sd logic.StopDataset) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lat := r.URL.Query().Get("lat")
		lng := r.URL.Query().Get("lng")
		dist := r.URL.Query().Get("distance")
		south := r.URL.Query().Get("south")
		north := r.URL.Query().Get("north")
		east := r.URL.Query().Get("east")
		west := r.URL.Query().Get("west")

		var stops []logic.StopWithDistance
		var err error
		if south != "" && north != "" && east != "" && west != "" {
			stops, err = sd.FetchWithinBox(west, south, east, north)
			if err != nil {
				httpError(w, "HandleStops:", err, http.StatusInternalServerError)
				return
			}
		} else if lat != "" && lng != "" && dist != "" {
			stops, err = sd.FetchWithinDistance(lat, lng, dist)
			if err != nil {
				httpError(w, "HandleStops:", err, http.StatusInternalServerError)
				return
			}
		} else {
			stops, err = sd.FetchAllStops()
			if err != nil {
				httpError(w, "HandleStops:", err, http.StatusInternalServerError)
				return
			}
		}

		b, err := json.Marshal(stopsWithDistanceResponse{Stops: stops})
		if err != nil {
			httpError(w, "HandleStops:", err, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(b); err != nil {
			httpError(w, "HandleStops:", err, http.StatusInternalServerError)
			return
		}
	}
}

// HandleTrimetArrivals provides responses for the /api/v1/arrivals endpoint.
// It proxies requests mostly untouched to the trimet API and returns a list of
// arrivals for the specified location IDs.
func HandleTrimetArrivals(apiKey string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		ids, err := commaSplitInts(r.URL.Query().Get("locIDs"))
		if err != nil {
			http.Error(w, fmt.Sprintf("error parsing ids: %v", err), http.StatusBadRequest)
			return
		}

		b, err := trimet.RequestArrivals(apiKey, ids)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)

	}
}

// HandleVehiclePositions provides responses for the /api/v1/vehicles endpoint.
// It returns a list of vehicles pulled from a local DB populated from a GTFS feed.
func HandleVehiclePositions(vd logic.VehicleDataset) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var err error
		sinceStr := r.URL.Query().Get("since")
		var since int
		if sinceStr != "" {
			since, err = strconv.Atoi(sinceStr)
			if err != nil {
				http.Error(w, fmt.Sprintf("error parsing since: %v", err), http.StatusBadRequest)
				return
			}
		}

		vehicles, err := vd.FetchVehiclePositions(since)
		if err != nil {
			httpError(w, "HandleVehiclePositions:", err, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		b, err := json.Marshal(vehicles)
		if err != nil {
			httpError(w, "HandleVehiclePositions:", err, http.StatusInternalServerError)
			return
		}
		w.Write(b)

	}
}

// HandleTripUpdates returns a list of trip updates as json
func HandleTripUpdates(tds logic.TripUpdatesDataset) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		tu, err := tds.FetchTripUpdates()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		b, err := json.Marshal(tu)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)

	}
}

func parseArgs(argStr, sep string) []string {
	argsSplit := strings.Split(argStr, sep)
	var argsArr []string
	if len(argsSplit) == 1 && argsSplit[0] == "" {
		return nil
	}

	for _, a := range argsSplit {
		if a != "" {
			argsArr = append(argsArr, a)
		}
	}
	if len(argsArr) == 0 {
		return nil
	}
	return argsArr
}

// ArrivalWithTrip ...
type ArrivalWithTrip struct {
	logic.Arrival
	TripShape *logic.TripShape `json:"trip_shape"`
}

// HandleArrivals returns a list of upcoming arrivals for a list of stops.
func HandleArrivals(sds logic.StopDataset, shds logic.ShapeDataset) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ids := parseArgs(r.URL.Query().Get("stop_ids"), ",")
		if ids == nil {
			http.Error(w, "error: a list of stop IDs are required", http.StatusBadRequest)
			return
		}
		ar, err := sds.FetchArrivals(ids)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		awts := []ArrivalWithTrip{}

		trips := []string{}
		for _, a := range ar {
			trips = append(trips, a.TripID)
		}
		shapes, err := shds.FetchTripShapes(trips)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		for _, a := range ar {
			awt := ArrivalWithTrip{
				Arrival:   a,
				TripShape: shapes[a.TripID],
			}
			awts = append(awts, awt)
		}

		b, err := json.Marshal(awts)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(ar) == 0 {
			log.Println("arrivals empty")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}
}

// HandleRoutes returns a list of routes
func HandleRoutes(rds logic.RouteDataset) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		routes, err := rds.FetchRoutes()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		b, err := json.Marshal(routes)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)

	}
}

// HandleShapes returns a list of shapes
func HandleShapes(sds logic.ShapeDataset) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		shapeIDs := parseArgs(r.URL.Query().Get("shape_ids"), ",")
		routeIDs := parseArgs(r.URL.Query().Get("route_ids"), ",")
		if shapeIDs == nil && routeIDs == nil {
			http.Error(w, "must specify route_id or shape_id", http.StatusBadRequest)
			return
		}

		shapes, err := sds.FetchShapes(routeIDs, shapeIDs)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		b, err := json.Marshal(shapes)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)

	}
}

// HandleRouteLines returns a list of routelines
func HandleRouteLines(sds logic.ShapeDataset) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		lines, err := sds.FetchRouteShapes()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		b, err := json.Marshal(lines)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)

	}
}

type wsMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

var count int64

func init() {
	var lastCount int64
	go func() {
		for {
			if count != lastCount {
				atomic.StoreInt64(&lastCount, count)
			}
			time.Sleep(2 * time.Second)
		}
	}()
}

// HandleWS maintains a websocket connection and pushes content to
// connected web clients.
func HandleWS(vds logic.VehicleDataset, shds logic.ShapeDataset, sds logic.StopDataset, rds logic.RouteDataset) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("Client Connecting")
		chunkify := true
		sendStaticData := true
		var version int64
		var err error

		verStr := r.URL.Query().Get("version")
		if verStr != "" {
			version, err = strconv.ParseInt(verStr, 10, 8)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		cStr := r.URL.Query().Get("chunkify")
		if cStr != "" {
			chunkify, err = strconv.ParseBool(cStr)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		sStr := r.URL.Query().Get("static")
		if sStr != "" {
			sendStaticData, err = strconv.ParseBool(sStr)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}

		var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Print(err)
			return
		}
		defer c.Close()
		log.Printf("Client v%d Connected", version)

		atomic.AddInt64(&count, 1)
		defer atomic.AddInt64(&count, -1)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			for {
				_, _, err := c.ReadMessage() // just reading in case we need to kill conn
				if err != nil {
					cancel()
					log.Println(err)
					break
				}
			}
		}()

		wg := &sync.WaitGroup{}
		wg.Add(4)
		var since uint64
		var vehicles []logic.VehiclePositionWithRouteType
		go func() {
			defer wg.Done()
			var err error
			vehicles, err = vds.FetchVehiclePositions(0)
			if err != nil {
				log.Println(err)
				cancel()
			}
			for _, v := range vehicles {
				if v.Timestamp > since {
					since = v.Timestamp
				}
			}
		}()

		var stops []logic.StopWithDistance
		go func() {
			defer wg.Done()
			var err error
			stops, err = sds.FetchAllStops()
			if err != nil {
				log.Println(err)
				cancel()
			}
		}()

		var shapes []*logic.RouteShape
		go func() {
			defer wg.Done()
			var err error
			shapes, err = shds.FetchRouteShapes()
			if err != nil {
				log.Println(err)
				cancel()
			}
		}()

		var routes []trimet.Route
		go func() {
			defer wg.Done()
			var err error
			routes, err = rds.FetchRoutes()
			if err != nil {
				log.Println(err)
				cancel()
			}
		}()

		wg.Wait()
		ticker := time.NewTicker(5000 * time.Millisecond)
		timer := time.NewTimer(1250 * time.Millisecond)
		defer timer.Stop()
		defer ticker.Stop()
	LOOP:
		for {
			select {
			case <-ctx.Done():
				break LOOP
			case <-timer.C:
				err := c.WriteJSON(wsMessage{
					Type: "totals",
					Data: map[string]int{
						"stops":        len(stops),
						"routes":       len(routes),
						"route_shapes": len(shapes),
						"vehicles":     len(vehicles),
					},
				})
				if err != nil {
					log.Println(err)
					return
				}

				if sendStaticData {
					if chunkify {
						var stopsPacket [100]logic.StopWithDistance
						for i, s := range stops {
							if i%100 == 0 || i == len(stops)-1 {
								if err := c.WriteJSON(wsMessage{Type: "stops", Data: stopsPacket}); err != nil {
									log.Println(err)
									return
								}
								time.Sleep(25 * time.Millisecond)
							} else {
								stopsPacket[i%100] = s
							}
						}
					} else {
						if err := c.WriteJSON(wsMessage{Type: "stops", Data: stops}); err != nil {
							log.Println(err)
							return
						}
					}

					if err := c.WriteJSON(wsMessage{Type: "routes", Data: routes}); err != nil {
						log.Println(err)
						return
					}
					time.Sleep(25 * time.Millisecond)

					if err := c.WriteJSON(wsMessage{Type: "route_shapes", Data: shapes}); err != nil {
						log.Println(err)
						return
					}
				}

				if err := c.WriteJSON(wsMessage{Type: "vehicles", Data: vehicles}); err != nil {
					log.Println(err)
					continue
				}

			case <-ticker.C:
			}
			// 0 sends all positions on each push.
			vehicles, err := vds.FetchVehiclePositions(0) //int(since)
			if err != nil {
				log.Println(err)
				continue
			}
			for _, v := range vehicles {
				if v.Timestamp > since {
					since = v.Timestamp
				}
			}

			if len(vehicles) == 0 {
				continue
			}
			if err := c.WriteJSON(wsMessage{Type: "vehicles", Data: vehicles}); err != nil {
				log.Println(err)
				continue
			}
		}
	}
}
