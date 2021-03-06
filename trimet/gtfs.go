package trimet

//go:generate msgp

import (
	"strconv"
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
func NewCalendarDateFromRow(row []string) (*CalendarDate, error) {
	cd := CalendarDate{ServiceID: row[0]}
	var err error
	cd.Date, err = time.Parse(calendarDateLayout, row[1])
	if err != nil {
		return nil, errors.WithStack(err)
	}
	cd.ExceptionType, err = strconv.Atoi(row[2])
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &cd, nil
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
func NewShapeFromRow(row []string) (*Shape, error) {
	var err error
	shape := Shape{
		ID: row[0],
	}

	shape.PointLat, err = parseFloat(row[1], 0)
	if err != nil {
		return nil, err
	}

	shape.PointLng, err = parseFloat(row[2], 0)
	if err != nil {
		return nil, err
	}

	shape.PointSequence, err = parseInt(row[3], 0)
	if err != nil {
		return nil, err
	}

	shape.DistTraveled, err = parseNullableFloat(row[4])
	if err != nil {
		return nil, err
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
func NewStopFromRow(row []string) (*Stop, error) {
	var err error
	stop := Stop{
		ID:            row[0],
		Code:          row[1],
		Name:          row[2],
		Desc:          row[3],
		ZoneID:        row[6],
		URL:           row[7],
		ParentStation: row[9],
		Direction:     row[10],
		Position:      row[11],
	}

	stop.Lat, err = parseFloat(row[4], 0)
	if err != nil {
		return nil, err
	}
	stop.Lon, err = parseFloat(row[5], 0)
	if err != nil {
		return nil, err
	}
	stop.LocationType, err = parseInt(row[8], 0)
	if err != nil {
		return nil, err
	}
	if len(row) >= 13 {
		stop.WheelchairBoarding, err = parseInt(row[12], 0)
		if err != nil {
			return nil, err
		}
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
func NewStopTimeFromRow(row []string) (*StopTime, error) {
	st := StopTime{
		TripID: row[0],
		StopID: row[3],
	}
	var err error

	st.ArrivalTime, err = parseDuration(row[1])
	if err != nil {
		return nil, errors.WithStack(err)
	}
	st.DepartureTime, err = parseDuration(row[2])
	if err != nil {
		return nil, errors.WithStack(err)
	}

	st.StopSequence, err = strconv.Atoi(row[4])
	if err != nil {
		return nil, errors.WithStack(err)
	}

	st.StopHeadsign = parseNullableString(row[5])

	st.PickupType, err = parseInt(row[6], 0)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	st.DropOffType, err = parseInt(row[7], 0)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	st.ShapeDistTraveled, err = parseNullableFloat(row[8])
	if err != nil {
		return nil, errors.WithStack(err)
	}

	st.Timepoint, err = parseNullableInt(row[9])
	if err != nil {
		return nil, errors.WithStack(err)
	}

	st.ContinuousDropOff, err = parseInt(row[10], 0)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	st.ContinuousPickup, err = parseInt(row[11], 0)
	if err != nil {
		return nil, errors.WithStack(err)
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
func NewTripFromRow(row []string) (*Trip, error) {
	t := Trip{
		ID:        row[2],
		RouteID:   row[0],
		ServiceID: row[1],
		BlockID:   parseNullableString(row[4]),
		ShapeID:   parseNullableString(row[5]),
		// Commented out fields are in the spec but not provided by Trimet.
		// TripHeadsign:  parseNullableString(row[3]),
		// TripShortName: parseNullableString(row[4]),
	}

	var err error

	t.DirectionID, err = parseNullableInt(row[3])
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// t.BikesAllowed, err = parseInt(row[9], 0)
	// if err != nil {
	// 	return nil, errors.WithStack(err)
	// }

	// t.WheelchairAccessible, err = parseInt(row[8], 0)
	// if err != nil {
	// 	return nil, errors.WithStack(err)
	// }

	return &t, nil
}
