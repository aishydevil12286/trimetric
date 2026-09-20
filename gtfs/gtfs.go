package gtfs

//go:generate msgp

import (
	"time"

	"github.com/pkg/errors"
)

// CalendarDate ...
type CalendarDate struct {
	ServiceID     string    `json:"service_id" msg:"service_id"`
	Date          time.Time `json:"date" msg:"date"`
	ExceptionType int       `json:"exception_type" msg:"exception_type"`
}

const calendarDateLayout = "20060102"

// NewCalendarDateFromRow takes a single row from processing a
// calendar_dates.txt and creates a CalendarDate.
func NewCalendarDateFromRow(row Row) (*CalendarDate, error) {
	cd := CalendarDate{ServiceID: row.Get("service_id")}
	var err error
	cd.Date, err = time.Parse(calendarDateLayout, row.Get("date"))
	if err != nil {
		return nil, errors.Wrapf(err, "calendar_dates.txt: bad date %q", row.Get("date"))
	}
	cd.ExceptionType, err = parseInt(row.Get("exception_type"), 1)
	if err != nil {
		return nil, errors.Wrap(err, "calendar_dates.txt: bad exception_type")
	}
	return &cd, nil
}

// Calendar is a weekly service pattern from calendar.txt.
//
// Feeds may describe service with calendar.txt, with calendar_dates.txt, or
// with both. TriMet used calendar_dates.txt alone; Delhi's feed leans on
// calendar.txt, so both have to be understood to resolve a service day.
type Calendar struct {
	ServiceID string    `json:"service_id"`
	Days      [7]bool   `json:"days"`
	StartDate time.Time `json:"start_date"`
	EndDate   time.Time `json:"end_date"`
}

// weekdayColumns is indexed by time.Weekday, which starts at Sunday.
var weekdayColumns = [7]string{
	"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday",
}

// NewCalendarFromRow takes a single row from calendar.txt and creates a
// Calendar.
func NewCalendarFromRow(row Row) (*Calendar, error) {
	c := Calendar{ServiceID: row.Get("service_id")}

	for i, col := range weekdayColumns {
		n, err := parseInt(row.Get(col), 0)
		if err != nil {
			return nil, errors.Wrapf(err, "calendar.txt: bad %s", col)
		}
		c.Days[i] = n == 1
	}

	var err error
	c.StartDate, err = time.Parse(calendarDateLayout, row.Get("start_date"))
	if err != nil {
		return nil, errors.Wrapf(err, "calendar.txt: bad start_date %q", row.Get("start_date"))
	}
	c.EndDate, err = time.Parse(calendarDateLayout, row.Get("end_date"))
	if err != nil {
		return nil, errors.Wrapf(err, "calendar.txt: bad end_date %q", row.Get("end_date"))
	}
	return &c, nil
}

// Route represents a single route from a GTFS feed.
type Route struct {
	RouteID   string `json:"id" msg:"id"`
	AgencyID  string `json:"agency_id" msg:"agency_id"`
	ShortName string `json:"short_name" msg:"short_name"`
	LongName  string `json:"long_name" msg:"long_name"`
	Type      int    `json:"type" msg:"type"`
	URL       string `json:"url" msg:"url"`
	Color     string `json:"color" msg:"color"`
	TextColor string `json:"text_color" msg:"text_color"`
	SortOrder int    `json:"sort_order" msg:"sort_order"`
}

// NewRouteFromRow takes a single row from routes.txt and creates a Route.
func NewRouteFromRow(row Row) (*Route, error) {
	r := Route{
		RouteID:   row.Get("route_id"),
		AgencyID:  row.Get("agency_id"),
		ShortName: row.Get("route_short_name"),
		LongName:  row.Get("route_long_name"),
		URL:       row.Get("route_url"),
		Color:     row.Get("route_color"),
		TextColor: row.Get("route_text_color"),
	}

	var err error
	// route_type is required by the spec, but default to bus rather than
	// reject the whole feed over one malformed route.
	r.Type, err = parseInt(row.Get("route_type"), int(RouteTypeBus))
	if err != nil {
		return nil, errors.Wrapf(err, "routes.txt: bad route_type for route %q", r.RouteID)
	}

	// route_sort_order is optional and absent from most feeds.
	r.SortOrder, err = parseInt(row.Get("route_sort_order"), 0)
	if err != nil {
		return nil, errors.Wrapf(err, "routes.txt: bad route_sort_order for route %q", r.RouteID)
	}

	return &r, nil
}

// Shape describe the physical path that a vehicle takes, and are defined in the
// file shapes.txt. Shapes belong to Trips, and consist of a sequence of points.
// Tracing the points in order provides the path of the vehicle.
// The points do not need to match stop locations.
type Shape struct {
	ID            string   `json:"id" msg:"id"`
	PointLat      float64  `json:"pt_lat" msg:"pt_lat"`
	PointLng      float64  `json:"pt_lng" msg:"pt_lng"`
	PointSequence int      `json:"pt_sequence" msg:"pt_sequence"`
	DistTraveled  *float64 `json:"dist_traveled" msg:"dist_traveled"`
}

// NewShapeFromRow ...
func NewShapeFromRow(row Row) (*Shape, error) {
	var err error
	shape := Shape{ID: row.Get("shape_id")}

	shape.PointLat, err = parseFloat(row.Get("shape_pt_lat"), 0)
	if err != nil {
		return nil, errors.Wrapf(err, "shapes.txt: bad shape_pt_lat for shape %q", shape.ID)
	}

	shape.PointLng, err = parseFloat(row.Get("shape_pt_lon"), 0)
	if err != nil {
		return nil, errors.Wrapf(err, "shapes.txt: bad shape_pt_lon for shape %q", shape.ID)
	}

	shape.PointSequence, err = parseInt(row.Get("shape_pt_sequence"), 0)
	if err != nil {
		return nil, errors.Wrapf(err, "shapes.txt: bad shape_pt_sequence for shape %q", shape.ID)
	}

	shape.DistTraveled, err = parseNullableFloat(row.Get("shape_dist_traveled"))
	if err != nil {
		return nil, errors.Wrapf(err, "shapes.txt: bad shape_dist_traveled for shape %q", shape.ID)
	}
	return &shape, nil
}

// Stop represents a single stop from a GTFS feed.
type Stop struct {
	ID                 string  `json:"id" msg:"id"`
	Code               string  `json:"code" msg:"code"`
	Name               string  `json:"name" msg:"name"`
	Desc               string  `json:"desc" msg:"desc"`
	Lat                float64 `json:"lat" msg:"lat"`
	Lon                float64 `json:"lng" msg:"lng"`
	ZoneID             string  `json:"zone_id" msg:"zone_id"`
	URL                string  `json:"url" msg:"url"`
	LocationType       int     `json:"location_type" msg:"location_type"`
	ParentStation      string  `json:"parent_station" msg:"parent_station"`
	Direction          string  `json:"direction" msg:"direction"`
	Position           string  `json:"position" msg:"position"`
	WheelchairBoarding int     `json:"wheelchair_boarding" msg:"wheelchair_boarding"`
}

// NewStopFromRow takes a single row from processing a
// stops.txt file and creates a Stop.
//
// direction and position are TriMet extensions to the spec; feeds that do not
// publish them simply leave them empty.
func NewStopFromRow(row Row) (*Stop, error) {
	var err error
	stop := Stop{
		ID:            row.Get("stop_id"),
		Code:          row.Get("stop_code"),
		Name:          row.Get("stop_name"),
		Desc:          row.Get("stop_desc"),
		ZoneID:        row.Get("zone_id"),
		URL:           row.Get("stop_url"),
		ParentStation: row.Get("parent_station"),
		Direction:     row.Get("direction"),
		Position:      row.Get("position"),
	}

	stop.Lat, err = parseFloat(row.Get("stop_lat"), 0)
	if err != nil {
		return nil, errors.Wrapf(err, "stops.txt: bad stop_lat for stop %q", stop.ID)
	}
	stop.Lon, err = parseFloat(row.Get("stop_lon"), 0)
	if err != nil {
		return nil, errors.Wrapf(err, "stops.txt: bad stop_lon for stop %q", stop.ID)
	}
	stop.LocationType, err = parseInt(row.Get("location_type"), 0)
	if err != nil {
		return nil, errors.Wrapf(err, "stops.txt: bad location_type for stop %q", stop.ID)
	}
	stop.WheelchairBoarding, err = parseInt(row.Get("wheelchair_boarding"), 0)
	if err != nil {
		return nil, errors.Wrapf(err, "stops.txt: bad wheelchair_boarding for stop %q", stop.ID)
	}
	return &stop, nil
}

// StopTime is a single stop time from a GTFS feed.
type StopTime struct {
	TripID            string   `json:"trip_id" msg:"trip_id"`
	ArrivalTime       *Time    `json:"arrival_time" msg:"arrival_time,extension"`
	DepartureTime     *Time    `json:"departure_time" msg:"departure_time,extension"`
	StopID            string   `json:"stop_id" msg:"stop_id"`
	StopSequence      int      `json:"stop_sequence" msg:"stop_sequence"`
	StopHeadsign      *string  `json:"stop_headsign" msg:"stop_headsign"`
	PickupType        int      `json:"pickup_type" msg:"pickup_type"`
	DropOffType       int      `json:"drop_off_type" msg:"drop_off_type"`
	ShapeDistTraveled *float64 `json:"shape_dist_traveled" msg:"shape_dist_traveled"`
	Timepoint         *int     `json:"timepoint" msg:"timepoint"`
	ContinuousDropOff int      `json:"continuous_drop_off" msg:"continuous_drop_off"`
	ContinuousPickup  int      `json:"continuous_pickup" msg:"continuous_pickup"`
}

// NewStopTimeFromRow takes a single row from processing a
// stop_times.txt file and creates a StopTime.
func NewStopTimeFromRow(row Row) (*StopTime, error) {
	st := StopTime{
		TripID: row.Get("trip_id"),
		StopID: row.Get("stop_id"),
	}
	var err error

	st.ArrivalTime, err = parseDuration(row.Get("arrival_time"))
	if err != nil {
		return nil, errors.Wrapf(err, "stop_times.txt: bad arrival_time for trip %q", st.TripID)
	}
	st.DepartureTime, err = parseDuration(row.Get("departure_time"))
	if err != nil {
		return nil, errors.Wrapf(err, "stop_times.txt: bad departure_time for trip %q", st.TripID)
	}

	st.StopSequence, err = parseInt(row.Get("stop_sequence"), 0)
	if err != nil {
		return nil, errors.Wrapf(err, "stop_times.txt: bad stop_sequence for trip %q", st.TripID)
	}

	st.StopHeadsign = parseNullableString(row.Get("stop_headsign"))

	st.PickupType, err = parseInt(row.Get("pickup_type"), 0)
	if err != nil {
		return nil, errors.Wrapf(err, "stop_times.txt: bad pickup_type for trip %q", st.TripID)
	}

	st.DropOffType, err = parseInt(row.Get("drop_off_type"), 0)
	if err != nil {
		return nil, errors.Wrapf(err, "stop_times.txt: bad drop_off_type for trip %q", st.TripID)
	}

	st.ShapeDistTraveled, err = parseNullableFloat(row.Get("shape_dist_traveled"))
	if err != nil {
		return nil, errors.Wrapf(err, "stop_times.txt: bad shape_dist_traveled for trip %q", st.TripID)
	}

	st.Timepoint, err = parseNullableInt(row.Get("timepoint"))
	if err != nil {
		return nil, errors.Wrapf(err, "stop_times.txt: bad timepoint for trip %q", st.TripID)
	}

	// Continuous pickup/drop-off default to 1 ("no continuous service") per
	// the spec, not to 0.
	st.ContinuousDropOff, err = parseInt(row.Get("continuous_drop_off"), 1)
	if err != nil {
		return nil, errors.Wrapf(err, "stop_times.txt: bad continuous_drop_off for trip %q", st.TripID)
	}

	st.ContinuousPickup, err = parseInt(row.Get("continuous_pickup"), 1)
	if err != nil {
		return nil, errors.Wrapf(err, "stop_times.txt: bad continuous_pickup for trip %q", st.TripID)
	}

	return &st, nil
}

// Trip ...
type Trip struct {
	ID                   string  `json:"id" msg:"id"`
	RouteID              string  `json:"route_id" msg:"route_id"`
	ServiceID            string  `json:"service_id" msg:"service_id"`
	DirectionID          *int    `json:"direction_id" msg:"direction_id"`
	BlockID              *string `json:"block_id" msg:"block_id"`
	ShapeID              *string `json:"shape_id" msg:"shape_id"`
	Headsign             *string `json:"headsign" msg:"headsign"`
	ShortName            *string `json:"short_name" msg:"short_name"`
	BikesAllowed         int     `json:"bikes_allowed" msg:"bikes_allowed"`
	WheelchairAccessible int     `json:"wheelchair_accessible" msg:"wheelchair_accessible"`
}

// NewTripFromRow takes a single row from processing a
// trips.txt file and creates a Trip.
func NewTripFromRow(row Row) (*Trip, error) {
	t := Trip{
		ID:        row.Get("trip_id"),
		RouteID:   row.Get("route_id"),
		ServiceID: row.Get("service_id"),
		BlockID:   parseNullableString(row.Get("block_id")),
		ShapeID:   parseNullableString(row.Get("shape_id")),
		Headsign:  parseNullableString(row.Get("trip_headsign")),
		ShortName: parseNullableString(row.Get("trip_short_name")),
	}

	var err error

	t.DirectionID, err = parseNullableInt(row.Get("direction_id"))
	if err != nil {
		return nil, errors.Wrapf(err, "trips.txt: bad direction_id for trip %q", t.ID)
	}

	t.BikesAllowed, err = parseInt(row.Get("bikes_allowed"), 0)
	if err != nil {
		return nil, errors.Wrapf(err, "trips.txt: bad bikes_allowed for trip %q", t.ID)
	}

	t.WheelchairAccessible, err = parseInt(row.Get("wheelchair_accessible"), 0)
	if err != nil {
		return nil, errors.Wrapf(err, "trips.txt: bad wheelchair_accessible for trip %q", t.ID)
	}

	return &t, nil
}
