package logic

import (
	"archive/zip"
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/bsdavidson/trimetric/gtfs"
	"github.com/lib/pq"
	"github.com/pkg/errors"
)

var staticTables = []string{
	"calendar",
	"calendar_dates",
	"routes",
	"services",
	"shapes",
	"stop_times",
	"stops",
	"trips",
}

var realtimeTables = []string{
	"stop_time_updates",
	"trip_updates",
	"vehicle_positions",
}

// errSkipRow tells bulkReplace to drop a row without failing the load.
var errSkipRow = errors.New("skip row")

// calendarDateKeyLayout formats a date for de-duplicating calendar_dates rows.
const calendarDateKeyLayout = "20060102"

// PollGTFSData periodically reloads the static GTFS feed.
//
// The time of the last successful load is kept in the database, so a restart
// does not re-download a feed that is still fresh, while a first run against
// an empty database loads immediately.
func PollGTFSData(ctx context.Context, ld LoaderDataset, src gtfs.StaticSource, dur time.Duration) {
	for {
		lastLoad, err := ld.LastLoadTime()
		if err != nil {
			log.Println("gtfs loader:", err)
		}

		if wait := time.Until(lastLoad.Add(dur)); wait > 0 {
			log.Printf("gtfs loader: feed last loaded %s, next load in %s",
				lastLoad.Format(time.RFC3339), wait.Truncate(time.Second))
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
		}

		if err := ld.LoadGTFSData(src); err != nil {
			log.Println("gtfs loader:", err)
			// Back off before retrying so a misconfigured feed does not spin.
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Minute):
			}
			continue
		}

		if err := ld.MarkLoaded(time.Now()); err != nil {
			log.Println("gtfs loader:", err)
		}

		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

// bulkReplace COPYs every row of c into table.
//
// A callback returning errSkipRow drops that row; the number of dropped rows
// is returned alongside the number loaded.
func bulkReplace(tx *sql.Tx, c *gtfs.CSV, table string, columns []string, callback func(stmt *sql.Stmt, row gtfs.Row) error) (loaded int, skipped int, err error) {
	defer c.Close()

	stmt, err := tx.Prepare(pq.CopyIn(table, columns...))
	if err != nil {
		return 0, 0, errors.WithStack(err)
	}

	for {
		row, rerr := c.Read()
		if rerr == io.EOF {
			break
		} else if rerr != nil {
			stmt.Exec()
			return 0, 0, errors.Wrapf(rerr, "reading rows for %s", table)
		}

		if cerr := callback(stmt, row); cerr == errSkipRow {
			skipped++
			continue
		} else if cerr != nil {
			stmt.Exec()
			return 0, 0, cerr
		}
		loaded++
	}

	if _, err := stmt.Exec(); err != nil {
		return 0, 0, errors.Wrapf(err, "flushing %s", table)
	}

	if err := stmt.Close(); err != nil {
		return 0, 0, errors.WithStack(err)
	}

	return loaded, skipped, nil
}

// LoaderDataset provides methods to bulk load static GTFS data.
type LoaderDataset interface {
	LoadGTFSData(src gtfs.StaticSource) error
	LastLoadTime() (time.Time, error)
	MarkLoaded(t time.Time) error
}

// LoaderSQLDataset implements LoaderDataset for a SQL database.
type LoaderSQLDataset struct {
	DB *sql.DB
}

// LastLoadTime returns when the static feed was last loaded successfully, or
// the zero time if it never has been.
func (ld *LoaderSQLDataset) LastLoadTime() (time.Time, error) {
	var t time.Time
	err := ld.DB.QueryRow(`SELECT loaded_at FROM feed_loads`).Scan(&t)
	if err == sql.ErrNoRows {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, errors.WithStack(err)
	}
	return t, nil
}

// MarkLoaded records the time of a successful load.
func (ld *LoaderSQLDataset) MarkLoaded(t time.Time) error {
	_, err := ld.DB.Exec(`
		INSERT INTO feed_loads (loaded_at) VALUES ($1)
		ON CONFLICT (id) DO UPDATE SET loaded_at = EXCLUDED.loaded_at
	`, t)
	return errors.WithStack(err)
}

// LoadGTFSData fetches the static GTFS feed and replaces the schedule data in
// the database with it.
//
// Files load in dependency order, and rows referencing something the feed
// never defines are dropped rather than aborting the load. Published feeds
// regularly carry a handful of dangling references, and losing a stop time is
// a much better outcome than losing the whole schedule.
func (ld *LoaderSQLDataset) LoadGTFSData(src gtfs.StaticSource) error {
	if src.File != "" {
		log.Printf("gtfs loader: reading static feed from %s", src.File)
	} else {
		log.Printf("gtfs loader: downloading static feed from %s", src.URL)
	}

	feed, err := gtfs.RequestGTFSFile(src)
	if err != nil {
		return err
	}
	defer feed.Close()
	log.Println("gtfs loader: finished reading static feed")

	tx, err := ld.DB.Begin()
	if err != nil {
		return errors.WithStack(err)
	}

	if _, err := tx.Exec(fmt.Sprintf("TRUNCATE %s RESTART IDENTITY", strings.Join(staticTables, ", "))); err != nil {
		return rollbackError(tx.Rollback(), errors.Wrap(err, "truncating static tables"))
	}

	// services is not a GTFS file. It is the union of every service_id named
	// by calendar.txt and calendar_dates.txt, giving the other tables a key
	// to hang their foreign keys off.
	services, err := ld.LoadServices(tx, feed)
	if err != nil {
		return rollbackError(tx.Rollback(), err)
	}
	if len(services) == 0 {
		return rollbackError(tx.Rollback(),
			errors.New("feed defines no services: it needs calendar.txt or calendar_dates.txt"))
	}

	if err := loadOptional(feed, "calendar.txt", func(c *gtfs.CSV) error {
		return ld.LoadCalendar(tx, c, services)
	}); err != nil {
		return rollbackError(tx.Rollback(), err)
	}

	if err := loadOptional(feed, "calendar_dates.txt", func(c *gtfs.CSV) error {
		return ld.LoadCalendarDates(tx, c, services)
	}); err != nil {
		return rollbackError(tx.Rollback(), err)
	}

	routes := map[string]struct{}{}
	if err := loadRequired(feed, "routes.txt", func(c *gtfs.CSV) error {
		return ld.LoadRoutes(tx, c, routes)
	}); err != nil {
		return rollbackError(tx.Rollback(), err)
	}

	if err := loadOptional(feed, "shapes.txt", func(c *gtfs.CSV) error {
		return ld.LoadShapes(tx, c)
	}); err != nil {
		return rollbackError(tx.Rollback(), err)
	}

	trips := map[string]struct{}{}
	if err := loadRequired(feed, "trips.txt", func(c *gtfs.CSV) error {
		return ld.LoadTrips(tx, c, routes, services, trips)
	}); err != nil {
		return rollbackError(tx.Rollback(), err)
	}

	stops := map[string]struct{}{}
	if err := loadRequired(feed, "stops.txt", func(c *gtfs.CSV) error {
		return ld.LoadStops(tx, c, stops)
	}); err != nil {
		return rollbackError(tx.Rollback(), err)
	}

	if err := loadRequired(feed, "stop_times.txt", func(c *gtfs.CSV) error {
		return ld.LoadStopTimes(tx, c, trips, stops)
	}); err != nil {
		return rollbackError(tx.Rollback(), err)
	}

	if err := tx.Commit(); err != nil {
		return rollbackError(tx.Rollback(), err)
	}
	log.Println("gtfs loader: static feed loaded")
	return nil
}

// loadRequired runs fn over a file the feed must contain.
func loadRequired(feed *zip.ReadCloser, name string, fn func(*gtfs.CSV) error) error {
	c, err := gtfs.ReadZippedGTFSCSV(feed, name)
	if err != nil {
		return err
	}
	log.Printf("gtfs loader: loading %s", name)
	return fn(c)
}

// loadOptional runs fn over a file the feed may omit.
func loadOptional(feed *zip.ReadCloser, name string, fn func(*gtfs.CSV) error) error {
	c, err := gtfs.ReadZippedGTFSCSV(feed, name)
	if gtfs.IsFileMissing(err) {
		log.Printf("gtfs loader: feed has no %s, skipping", name)
		return nil
	} else if err != nil {
		return err
	}
	log.Printf("gtfs loader: loading %s", name)
	return fn(c)
}

// logLoad reports what a single file contributed.
func logLoad(name string, loaded, skipped int) {
	if skipped > 0 {
		log.Printf("gtfs loader: %s: loaded %d rows, dropped %d referencing unknown ids", name, loaded, skipped)
		return
	}
	log.Printf("gtfs loader: %s: loaded %d rows", name, loaded)
}

// LoadServices populates the services table with every service_id the feed
// mentions, from calendar.txt and calendar_dates.txt alike. It returns the set
// of ids so later files can be checked against it.
func (ld *LoaderSQLDataset) LoadServices(tx *sql.Tx, feed *zip.ReadCloser) (map[string]struct{}, error) {
	ids := map[string]struct{}{}

	for _, name := range []string{"calendar.txt", "calendar_dates.txt"} {
		c, err := gtfs.ReadZippedGTFSCSV(feed, name)
		if gtfs.IsFileMissing(err) {
			continue
		} else if err != nil {
			return nil, err
		}

		for {
			row, err := c.Read()
			if err == io.EOF {
				break
			} else if err != nil {
				c.Close()
				return nil, errors.Wrapf(err, "reading %s", name)
			}
			if id := row.Get("service_id"); id != "" {
				ids[id] = struct{}{}
			}
		}
		c.Close()
	}

	stmt, err := tx.Prepare(pq.CopyIn("services", "id"))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for id := range ids {
		if _, err := stmt.Exec(id); err != nil {
			stmt.Exec()
			return nil, errors.WithStack(err)
		}
	}
	if _, err := stmt.Exec(); err != nil {
		return nil, errors.Wrap(err, "flushing services")
	}
	if err := stmt.Close(); err != nil {
		return nil, errors.WithStack(err)
	}

	logLoad("services", len(ids), 0)
	return ids, nil
}

// LoadCalendar loads calendar.txt.
func (ld *LoaderSQLDataset) LoadCalendar(tx *sql.Tx, c *gtfs.CSV, services map[string]struct{}) error {
	cols := []string{
		"service_id", "monday", "tuesday", "wednesday", "thursday", "friday",
		"saturday", "sunday", "start_date", "end_date",
	}
	seen := map[string]struct{}{}
	loaded, skipped, err := bulkReplace(tx, c, "calendar", cols, func(stmt *sql.Stmt, row gtfs.Row) error {
		cal, err := gtfs.NewCalendarFromRow(row)
		if err != nil {
			return err
		}
		if _, ok := services[cal.ServiceID]; !ok {
			return errSkipRow
		}
		if _, ok := seen[cal.ServiceID]; ok {
			return errSkipRow
		}
		seen[cal.ServiceID] = struct{}{}

		// Calendar.Days is indexed by time.Weekday, which starts at Sunday.
		_, err = stmt.Exec(
			cal.ServiceID, cal.Days[time.Monday], cal.Days[time.Tuesday],
			cal.Days[time.Wednesday], cal.Days[time.Thursday], cal.Days[time.Friday],
			cal.Days[time.Saturday], cal.Days[time.Sunday], cal.StartDate, cal.EndDate)
		return errors.WithStack(err)
	})
	if err != nil {
		return err
	}
	logLoad("calendar.txt", loaded, skipped)
	return nil
}

// LoadCalendarDates loads calendar_dates.txt.
func (ld *LoaderSQLDataset) LoadCalendarDates(tx *sql.Tx, c *gtfs.CSV, services map[string]struct{}) error {
	cols := []string{"service_id", "date", "exception_type"}
	seen := map[string]struct{}{}
	loaded, skipped, err := bulkReplace(tx, c, "calendar_dates", cols, func(stmt *sql.Stmt, row gtfs.Row) error {
		cd, err := gtfs.NewCalendarDateFromRow(row)
		if err != nil {
			return err
		}
		if _, ok := services[cd.ServiceID]; !ok {
			return errSkipRow
		}
		// (service_id, date) is the primary key, and duplicates do occur.
		key := cd.ServiceID + "\x00" + cd.Date.Format(calendarDateKeyLayout)
		if _, ok := seen[key]; ok {
			return errSkipRow
		}
		seen[key] = struct{}{}

		_, err = stmt.Exec(cd.ServiceID, cd.Date, cd.ExceptionType)
		return errors.WithStack(err)
	})
	if err != nil {
		return err
	}
	logLoad("calendar_dates.txt", loaded, skipped)
	return nil
}

// LoadRoutes loads routes.txt, recording the ids it loaded in routes.
func (ld *LoaderSQLDataset) LoadRoutes(tx *sql.Tx, c *gtfs.CSV, routes map[string]struct{}) error {
	cols := []string{
		"id", "agency_id", "short_name", "long_name",
		"type", "url", "color", "text_color",
		"sort_order",
	}
	loaded, skipped, err := bulkReplace(tx, c, "routes", cols, func(stmt *sql.Stmt, row gtfs.Row) error {
		r, err := gtfs.NewRouteFromRow(row)
		if err != nil {
			return err
		}
		if r.RouteID == "" {
			return errSkipRow
		}
		if _, ok := routes[r.RouteID]; ok {
			return errSkipRow
		}
		routes[r.RouteID] = struct{}{}

		_, err = stmt.Exec(
			r.RouteID, r.AgencyID, r.ShortName, r.LongName, r.Type, r.URL,
			r.Color, r.TextColor, r.SortOrder)
		return errors.WithStack(err)
	})
	if err != nil {
		return err
	}
	logLoad("routes.txt", loaded, skipped)
	return nil
}

// LoadShapes loads shapes.txt.
func (ld *LoaderSQLDataset) LoadShapes(tx *sql.Tx, c *gtfs.CSV) error {
	cols := []string{"id", "pt_lon_lat", "pt_sequence", "dist_traveled"}
	seen := map[string]struct{}{}
	loaded, skipped, err := bulkReplace(tx, c, "shapes", cols, func(stmt *sql.Stmt, row gtfs.Row) error {
		sh, err := gtfs.NewShapeFromRow(row)
		if err != nil {
			return err
		}
		if sh.ID == "" {
			return errSkipRow
		}
		// (id, pt_sequence) is the primary key.
		key := fmt.Sprintf("%s\x00%d", sh.ID, sh.PointSequence)
		if _, ok := seen[key]; ok {
			return errSkipRow
		}
		seen[key] = struct{}{}

		lngLat := fmt.Sprintf("SRID=4326;POINT(%f %f)", sh.PointLng, sh.PointLat)
		_, err = stmt.Exec(sh.ID, lngLat, sh.PointSequence, sh.DistTraveled)
		return errors.WithStack(err)
	})
	if err != nil {
		return err
	}
	logLoad("shapes.txt", loaded, skipped)
	return nil
}

// LoadStops loads stops.txt, recording the ids it loaded in stops.
func (ld *LoaderSQLDataset) LoadStops(tx *sql.Tx, c *gtfs.CSV, stops map[string]struct{}) error {
	cols := []string{
		"id", "code", "name", "desc", "lat_lon", "zone_id", "url",
		"location_type", "parent_station", "direction", "position",
		"wheelchair_boarding",
	}
	loaded, skipped, err := bulkReplace(tx, c, "stops", cols, func(stmt *sql.Stmt, row gtfs.Row) error {
		st, err := gtfs.NewStopFromRow(row)
		if err != nil {
			return err
		}
		if st.ID == "" {
			return errSkipRow
		}
		if _, ok := stops[st.ID]; ok {
			return errSkipRow
		}
		stops[st.ID] = struct{}{}

		lonLat := fmt.Sprintf("SRID=4326;POINT(%f %f)", st.Lon, st.Lat)
		_, err = stmt.Exec(
			st.ID, st.Code, st.Name, st.Desc, lonLat, st.ZoneID, st.URL,
			st.LocationType, st.ParentStation, st.Direction, st.Position,
			st.WheelchairBoarding)
		return errors.WithStack(err)
	})
	if err != nil {
		return err
	}
	logLoad("stops.txt", loaded, skipped)
	return nil
}

// LoadStopTimes loads stop_times.txt, dropping rows for trips or stops the
// feed never defined.
func (ld *LoaderSQLDataset) LoadStopTimes(tx *sql.Tx, c *gtfs.CSV, trips, stops map[string]struct{}) error {
	cols := []string{
		"trip_id", "arrival_time", "departure_time", "stop_id",
		"stop_sequence", "stop_headsign", "pickup_type", "drop_off_type",
		"shape_dist_traveled", "timepoint", "continuous_drop_off",
		"continuous_pickup",
	}
	seen := map[string]struct{}{}
	loaded, skipped, err := bulkReplace(tx, c, "stop_times", cols, func(stmt *sql.Stmt, row gtfs.Row) error {
		st, err := gtfs.NewStopTimeFromRow(row)
		if err != nil {
			return err
		}
		if _, ok := trips[st.TripID]; !ok {
			return errSkipRow
		}
		if _, ok := stops[st.StopID]; !ok {
			return errSkipRow
		}
		// (trip_id, stop_sequence) is the primary key.
		key := fmt.Sprintf("%s\x00%d", st.TripID, st.StopSequence)
		if _, ok := seen[key]; ok {
			return errSkipRow
		}
		seen[key] = struct{}{}

		_, err = stmt.Exec(
			st.TripID, st.ArrivalTime, st.DepartureTime, st.StopID, st.StopSequence,
			st.StopHeadsign, st.PickupType, st.DropOffType, st.ShapeDistTraveled,
			st.Timepoint, st.ContinuousDropOff, st.ContinuousPickup)
		return errors.WithStack(err)
	})
	if err != nil {
		return err
	}
	logLoad("stop_times.txt", loaded, skipped)
	return nil
}

// LoadTrips loads trips.txt, dropping trips whose route or service the feed
// never defined and recording the ids it loaded in trips.
func (ld *LoaderSQLDataset) LoadTrips(tx *sql.Tx, c *gtfs.CSV, routes, services, trips map[string]struct{}) error {
	cols := []string{
		"route_id", "service_id", "id", "headsign",
		"short_name", "direction_id", "block_id", "shape_id",
		"wheelchair_accessible", "bikes_allowed",
	}
	loaded, skipped, err := bulkReplace(tx, c, "trips", cols, func(stmt *sql.Stmt, row gtfs.Row) error {
		t, err := gtfs.NewTripFromRow(row)
		if err != nil {
			return err
		}
		if t.ID == "" {
			return errSkipRow
		}
		if _, ok := routes[t.RouteID]; !ok {
			return errSkipRow
		}
		if _, ok := services[t.ServiceID]; !ok {
			return errSkipRow
		}
		if _, ok := trips[t.ID]; ok {
			return errSkipRow
		}
		trips[t.ID] = struct{}{}

		_, err = stmt.Exec(
			t.RouteID, t.ServiceID, t.ID, t.Headsign, t.ShortName,
			t.DirectionID, t.BlockID, t.ShapeID, t.WheelchairAccessible,
			t.BikesAllowed)
		return errors.WithStack(err)
	})
	if err != nil {
		return err
	}
	logLoad("trips.txt", loaded, skipped)
	return nil
}
