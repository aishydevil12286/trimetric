package logic

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bsdavidson/trimetric/gtfs"
	postgis "github.com/cridenour/go-postgis"
	"github.com/lib/pq"
	"github.com/pkg/errors"
)

func rollbackError(rberr error, err error) error {
	if rberr == nil {
		return errors.WithStack(err)
	}
	return errors.Wrapf(err, "error rolling back: %s", rberr)
}

// StopWithDistance augments the Stop struct with a distance field.
// Distance is calculated via PostGIS and the result is used to
// provide a list of stops within a specified distance from a specific point.
type StopWithDistance struct {
	gtfs.Stop
	Distance float64 `json:"distance"`
}

// StopDataset provides methods to query and update a database table of Stops
type StopDataset interface {
	FetchAllStops() ([]StopWithDistance, error)
	FetchWithinDistance(lat, lng, dist string) ([]StopWithDistance, error)
	FetchWithinBox(w, s, e, n string) ([]StopWithDistance, error)
	FetchArrivals(stopIDs []string) ([]Arrival, error)
}

// StopSQLDataset stores a DB instance and provides access to methods to
// retrieve and update stops from the database
type StopSQLDataset struct {
	DB *sql.DB

	// Timezone is the agency's timezone, used to work out which GTFS service
	// day "now" falls in. Schedules are written in local time with no offset,
	// so this cannot be inferred from the data.
	Timezone string
}

// timezone returns the configured timezone, defaulting to the feed package's.
func (sd *StopSQLDataset) timezone() string {
	if sd.Timezone == "" {
		return gtfs.DefaultTimezone
	}
	return sd.Timezone
}

// FetchAllStops ...
func (sd *StopSQLDataset) FetchAllStops() ([]StopWithDistance, error) {
	q := `
		SELECT
			id, code, name, "desc", lat_lon, zone_id, url, location_type,
			parent_station, direction, position
		FROM stops
	`
	rows, err := sd.DB.Query(q)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer rows.Close()
	stops := []StopWithDistance{}

	for rows.Next() {
		var s StopWithDistance
		var lonLat postgis.PointS
		err := rows.Scan(
			&s.ID, &s.Code, &s.Name, &s.Desc, &lonLat, &s.ZoneID, &s.URL,
			&s.LocationType, &s.ParentStation, &s.Direction, &s.Position)
		if err != nil {
			return nil, err
		}
		s.Lat = lonLat.Y
		s.Lon = lonLat.X
		stops = append(stops, s)
	}
	return stops, nil
}

// FetchWithinDistance takes a point (lat,lng) and finds all stops located within 'dist'.
// It uses the PostGIS extension to calculate the distance to stops stored in the DB.
func (sd *StopSQLDataset) FetchWithinDistance(lat, lng, dist string) ([]StopWithDistance, error) {
	q := `
		SELECT
			id, code, name, "desc", lat_lon, zone_id, url, location_type,
			parent_station, direction, position,
			ST_Distance(ST_GeogFromText($1), lat_lon) AS distance
		FROM stops
		WHERE ST_DWithin(ST_GeogFromText($1), lat_lon, $2)
		ORDER BY distance ASC
	`
	rows, err := sd.DB.Query(q, fmt.Sprintf("SRID=4326;POINT(%s %s)", lng, lat), dist)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer rows.Close()
	stops := []StopWithDistance{}

	for rows.Next() {
		var s StopWithDistance
		var lonLat postgis.PointS
		err := rows.Scan(
			&s.ID, &s.Code, &s.Name, &s.Desc, &lonLat, &s.ZoneID, &s.URL,
			&s.LocationType, &s.ParentStation, &s.Direction, &s.Position, &s.Distance)
		if err != nil {
			return nil, err
		}
		s.Lat = lonLat.Y
		s.Lon = lonLat.X
		stops = append(stops, s)
	}
	return stops, nil
}

// FetchWithinBox ...
func (sd *StopSQLDataset) FetchWithinBox(w, s, e, n string) ([]StopWithDistance, error) {
	q := `
		SELECT
			id, code, name, "desc", lat_lon, zone_id, url, location_type,
			parent_station, direction, position
		FROM stops s
		WHERE s.lat_lon && ST_MakeEnvelope($1, $2, $3, $4, 4326)
	`
	rows, err := sd.DB.Query(q, w, s, e, n)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer rows.Close()
	stops := []StopWithDistance{}

	for rows.Next() {
		var s StopWithDistance
		var lonLat postgis.PointS
		err := rows.Scan(
			&s.ID, &s.Code, &s.Name, &s.Desc, &lonLat, &s.ZoneID, &s.URL,
			&s.LocationType, &s.ParentStation, &s.Direction, &s.Position)
		if err != nil {
			return nil, err
		}
		s.Lat = lonLat.Y
		s.Lon = lonLat.X
		stops = append(stops, s)
	}
	return stops, nil
}

// Arrival ...
type Arrival struct {
	RouteID         string        `json:"route_id"`
	RouteShortName  string        `json:"route_short_name"`
	RouteLongName   string        `json:"route_long_name"`
	RouteType       int           `json:"route_type"`
	RouteColor      string        `json:"route_color"`
	RouteTextColor  string        `json:"route_text_color"`
	TripID          string        `json:"trip_id"`
	StopID          string        `json:"stop_id"`
	Headsign        string        `json:"headsign"`
	ArrivalTime     *gtfs.Time    `json:"arrival_time"`
	DepartureTime   *gtfs.Time    `json:"departure_time"`
	VehicleID       *string       `json:"vehicle_id"`
	VehicleLabel    *string       `json:"vehicle_label"`
	VehiclePosition gtfs.Position `json:"vehicle_position"`
	Date            time.Time     `json:"date"`
}

func parseDuration(s string) (*time.Duration, error) {
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return nil, errors.Errorf("expected 3 parts, found %d", len(parts))
	}

	var intParts [3]time.Duration
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		intParts[i] = time.Duration(n)
	}
	dur := intParts[0]*time.Hour + intParts[1]*time.Minute + intParts[2]*time.Second
	return &dur, nil
}

// FetchArrivals returns a list of arrivals by combining multiple data sets.
func (sd *StopSQLDataset) FetchArrivals(stopIDs []string) ([]Arrival, error) {
	var err error
	// A GTFS arrival_time is an offset from the start of its service day, and
	// can run past 24:00:00 for trips that cross midnight. So "arriving in the
	// next hour" has to be asked of two service days at once: today, and
	// yesterday's day still running past midnight.
	q := `
		WITH local_now AS (
			SELECT now() AT TIME ZONE $2::text AS ts
		), service_days AS (
			SELECT
				ts::date AS service_date,
				ts::time - time '00:00:00' AS elapsed,
				interval '0' AS day_offset
			FROM local_now
			UNION ALL
			SELECT
				(ts - interval '1 day')::date,
				ts::time - time '00:00:00',
				interval '24 hours'
			FROM local_now
		)
		SELECT
			r.id, r.short_name, r.long_name, r.type, r.color, r.text_color, t.id,
			st.stop_id, COALESCE(st.stop_headsign, t.headsign), st.arrival_time,
			st.departure_time, v.vehicle_id, v.position_lon_lat, v.position_bearing,
			v.vehicle_label, d.service_date
		FROM routes r
		JOIN trips t ON t.route_id = r.id
		JOIN stop_times st ON st.trip_id = t.id
		LEFT JOIN (
			SELECT
			DISTINCT ON (trip_id) vehicle_id, trip_id, position_lon_lat, position_bearing, vehicle_label
			FROM vehicle_positions
			ORDER BY trip_id, vehicle_id ASC
		) v ON v.trip_id = t.id
		JOIN service_days d ON service_active(t.service_id, d.service_date)
		WHERE st.stop_id = ANY($1)
			AND st.arrival_time >= d.day_offset + d.elapsed
			AND st.arrival_time <= d.day_offset + d.elapsed + interval '1 hour'
		ORDER BY st.arrival_time ASC;
	`
	rows, err := sd.DB.Query(q, pq.Array(stopIDs), sd.timezone())
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer rows.Close()
	arrivals := []Arrival{}

	for rows.Next() {
		var a Arrival
		var bearing *float32
		var lonLat *postgis.PointS

		err := rows.Scan(
			&a.RouteID, &a.RouteShortName, &a.RouteLongName, &a.RouteType,
			&a.RouteColor, &a.RouteTextColor, &a.TripID, &a.StopID, &a.Headsign,
			&a.ArrivalTime, &a.DepartureTime, &a.VehicleID, &lonLat, &bearing,
			&a.VehicleLabel, &a.Date,
		)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if bearing != nil {
			a.VehiclePosition.Bearing = *bearing
		}
		if lonLat != nil {
			a.VehiclePosition.Longitude = float32(lonLat.X)
			a.VehiclePosition.Latitude = float32(lonLat.Y)
		}

		arrivals = append(arrivals, a)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.WithStack(err)
	}

	return arrivals, nil
}
