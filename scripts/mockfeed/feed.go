package main

import (
	"archive/zip"
	"fmt"
	"os"
	"strings"
	"time"
)

// serviceStart and serviceEnd bound the generated calendar. The window is
// deliberately wide so the feed does not expire and quietly stop producing
// arrivals.
const (
	serviceStart = "20200101"
	serviceEnd   = "20351231"
)

// schedule describes the span of the generated service day.
type schedule struct {
	FirstDeparture time.Duration
	LastDeparture  time.Duration
	Headway        time.Duration
	// RunTime is how long a vehicle takes to cover a whole route.
	RunTime time.Duration
}

var defaultSchedule = schedule{
	FirstDeparture: 5 * time.Hour,
	LastDeparture:  23 * time.Hour,
	Headway:        20 * time.Minute,
	RunTime:        70 * time.Minute,
}

// gtfsTime formats a duration since midnight as a GTFS time, which may run
// past 24:00:00 for a trip that crosses midnight.
func gtfsTime(d time.Duration) string {
	total := int(d.Seconds())
	return fmt.Sprintf("%02d:%02d:%02d", total/3600, (total%3600)/60, total%60)
}

// csvQuote wraps a field in quotes when it contains a comma or a quote.
func csvQuote(s string) string {
	if !strings.ContainsAny(s, ",\"\n") {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// writeFeed writes the generated network out as a GTFS zip.
//
// Columns are deliberately not in the order the GTFS spec lists them, and
// optional columns are left out, because a feed is free to do both and the
// loader has to cope.
func writeFeed(path string, n *network, sched schedule) (tripCount, stopTimeCount int, err error) {
	f, err := os.Create(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	z := zip.NewWriter(f)

	write := func(name string, lines []string) error {
		w, err := z.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write([]byte(strings.Join(lines, "\n") + "\n"))
		return err
	}

	agency := []string{
		"agency_id,agency_name,agency_url,agency_timezone,agency_lang",
		"DTC,Delhi Transport Corporation,https://otd.delhi.gov.in,Asia/Kolkata,en",
		"DIMTS,Delhi Integrated Multi-Modal Transit System,https://otd.delhi.gov.in,Asia/Kolkata,en",
	}
	if err := write("agency.txt", agency); err != nil {
		return 0, 0, err
	}

	// Weekday and weekend services, so both days of the week resolve to a
	// running schedule.
	calendar := []string{
		"service_id,monday,tuesday,wednesday,thursday,friday,saturday,sunday,start_date,end_date",
		"WD,1,1,1,1,1,0,0," + serviceStart + "," + serviceEnd,
		"WE,0,0,0,0,0,1,1," + serviceStart + "," + serviceEnd,
	}
	if err := write("calendar.txt", calendar); err != nil {
		return 0, 0, err
	}

	stops := []string{"stop_id,stop_name,stop_lat,stop_lon,stop_code"}
	for _, s := range n.Stops {
		stops = append(stops, fmt.Sprintf("%s,%s,%.6f,%.6f,%s",
			s.ID, csvQuote(s.Name), s.Pos.Lat, s.Pos.Lng, s.Code))
	}
	if err := write("stops.txt", stops); err != nil {
		return 0, 0, err
	}

	routes := []string{"route_long_name,route_id,route_type,route_short_name,agency_id,route_color"}
	for i, r := range n.Routes {
		agencyID := "DTC"
		if i%3 == 0 {
			agencyID = "DIMTS"
		}
		routes = append(routes, fmt.Sprintf("%s,%s,3,%s,%s,%s",
			csvQuote(r.LongName), r.ID, r.ShortName, agencyID, r.Color))
	}
	if err := write("routes.txt", routes); err != nil {
		return 0, 0, err
	}

	shapes := []string{"shape_id,shape_pt_lat,shape_pt_lon,shape_pt_sequence,shape_dist_traveled"}
	for _, r := range n.Routes {
		cum, _ := r.Shape.lengths()
		for i, p := range r.Shape {
			shapes = append(shapes, fmt.Sprintf("SH_%s,%.6f,%.6f,%d,%.1f",
				r.ID, p.Lat, p.Lng, i+1, cum[i]*111000))
		}
	}
	if err := write("shapes.txt", shapes); err != nil {
		return 0, 0, err
	}

	trips := []string{"trip_id,route_id,service_id,trip_headsign,direction_id,shape_id"}
	stopTimes := []string{"trip_id,stop_id,stop_sequence,arrival_time,departure_time"}

	for _, r := range n.Routes {
		// Space stops evenly over the run time. Real feeds do not, but the
		// arrivals list only cares that the times are ordered and plausible.
		gap := sched.RunTime / time.Duration(len(r.StopIDs))

		for _, service := range []string{"WD", "WE"} {
			for _, direction := range []int{0, 1} {
				ids := append([]string(nil), r.StopIDs...)
				headsign := "Terminal"
				if direction == 1 {
					for i, j := 0, len(ids)-1; i < j; i, j = i+1, j-1 {
						ids[i], ids[j] = ids[j], ids[i]
					}
				}
				if last := n.stopByID(ids[len(ids)-1]); last != nil {
					headsign = last.Name
				}

				run := 0
				for dep := sched.FirstDeparture; dep <= sched.LastDeparture; dep += sched.Headway {
					tripID := fmt.Sprintf("T_%s_%s_%d_%03d", r.ID, service, direction, run)
					run++
					trips = append(trips, fmt.Sprintf("%s,%s,%s,%s,%d,SH_%s",
						tripID, r.ID, service, csvQuote(headsign), direction, r.ID))
					tripCount++

					for i, stopID := range ids {
						t := dep + gap*time.Duration(i)
						ts := gtfsTime(t)
						stopTimes = append(stopTimes,
							fmt.Sprintf("%s,%s,%d,%s,%s", tripID, stopID, i+1, ts, ts))
						stopTimeCount++
					}
				}
			}
		}
	}

	if err := write("trips.txt", trips); err != nil {
		return 0, 0, err
	}
	if err := write("stop_times.txt", stopTimes); err != nil {
		return 0, 0, err
	}

	return tripCount, stopTimeCount, z.Close()
}

// stopByID looks a stop up by its generated id.
func (n *network) stopByID(id string) *stop {
	for _, s := range n.Stops {
		if s.ID == id {
			return s
		}
	}
	return nil
}
