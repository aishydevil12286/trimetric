package gtfs

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTime(t *testing.T) {

	testTime1 := Time(time.Duration(time.Hour))

	sqlInterval, err := testTime1.Value()
	require.NoError(t, err)
	assert.NotNil(t, sqlInterval)

	b, err := testTime1.MarshalText()
	require.NoError(t, err)
	assert.Equal(t, "01:00:00", string(b))

	var testTime2 Time
	require.NoError(t, testTime2.UnmarshalText(b))

	dv, err := testTime2.Value()
	require.NoError(t, err)
	assert.Equal(t, "01:00:00", dv)

}

func TestParseDuration(t *testing.T) {
	nilDur := Time(-1)

	tests := []struct {
		input string
		dur   Time
		err   error
	}{
		{
			input: "10:11:12",
			dur:   Time(10*time.Hour + 11*time.Minute + 12*time.Second),
			err:   nil,
		},
		{
			input: "",
			dur:   nilDur,
			err:   nil,
		},
		{
			input: "12:23",
			dur:   nilDur,
			err:   errors.New("expected 3 parts, found 2"),
		},
		{
			input: "12:BAD:TEXT",
			dur:   nilDur,
			err:   errors.New("strconv.Atoi: parsing \"BAD\": invalid syntax"),
		},
	}

	for _, test := range tests {
		dur, err := parseDuration(test.input)

		if test.err == nil {
			assert.Nil(t, err)
		} else {
			assert.EqualError(t, err, test.err.Error())
		}
		if test.dur == nilDur {
			assert.Nil(t, dur)
		} else if assert.NotNil(t, dur) {
			assert.Equal(t, *dur, test.dur)
		}
	}

}

// testRow builds a Row from parallel column-name and value slices, standing
// in for a line read out of a real feed.
func testRow(cols, vals []string) Row {
	header := map[string]int{}
	for i, c := range cols {
		header[c] = i
	}
	return Row{rec: vals, header: header}
}

func TestNewCalendarDateFromRow(t *testing.T) {
	cd, err := NewCalendarDateFromRow(testRow(
		[]string{"service_id", "date", "exception_type"},
		[]string{"U.497", "20180225", "1"}))
	require.NoError(t, err)
	assert.Equal(t, "2018-02-25 00:00:00 +0000 UTC", cd.Date.String())
	assert.Equal(t, 1, cd.ExceptionType)
	assert.Equal(t, "U.497", cd.ServiceID)
}

func TestNewCalendarFromRow(t *testing.T) {
	cal, err := NewCalendarFromRow(testRow(
		[]string{"service_id", "monday", "tuesday", "wednesday", "thursday",
			"friday", "saturday", "sunday", "start_date", "end_date"},
		[]string{"WD", "1", "1", "1", "1", "1", "0", "0", "20240101", "20241231"}))
	require.NoError(t, err)
	assert.Equal(t, "WD", cal.ServiceID)
	assert.True(t, cal.Days[time.Monday])
	assert.True(t, cal.Days[time.Friday])
	assert.False(t, cal.Days[time.Saturday])
	assert.False(t, cal.Days[time.Sunday])
	assert.Equal(t, "2024-01-01", cal.StartDate.Format("2006-01-02"))
	assert.Equal(t, "2024-12-31", cal.EndDate.Format("2006-01-02"))
}

func TestNewTripFromRow(t *testing.T) {
	tr, err := NewTripFromRow(testRow(
		[]string{"route_id", "service_id", "trip_id", "direction_id", "block_id", "shape_id"},
		[]string{"290", "C.500", "7735337", "0", "9068", "353218"}))
	require.NoError(t, err)
	assert.Equal(t, "7735337", tr.ID)
	assert.Equal(t, 0, *tr.DirectionID)
	assert.Equal(t, "353218", *tr.ShapeID)
}

// Columns are addressed by name, so a feed may order them however it likes
// and omit any it has no data for.
func TestNewTripFromRowReorderedColumns(t *testing.T) {
	tr, err := NewTripFromRow(testRow(
		[]string{"trip_id", "trip_headsign", "shape_id", "service_id", "route_id"},
		[]string{"7735337", "Kashmere Gate", "353218", "C.500", "290"}))
	require.NoError(t, err)
	assert.Equal(t, "7735337", tr.ID)
	assert.Equal(t, "290", tr.RouteID)
	assert.Equal(t, "C.500", tr.ServiceID)
	assert.Equal(t, "Kashmere Gate", *tr.Headsign)
	assert.Equal(t, "353218", *tr.ShapeID)
	// direction_id is absent from this feed rather than empty.
	assert.Nil(t, tr.DirectionID)
}

func TestStopTimeFromRow(t *testing.T) {
	st, err := NewStopTimeFromRow(testRow(
		[]string{"trip_id", "arrival_time", "departure_time", "stop_id",
			"stop_sequence", "stop_headsign", "pickup_type", "drop_off_type",
			"shape_dist_traveled", "timepoint", "continuous_drop_off", "continuous_pickup"},
		[]string{"7718078", "06:56:50", "06:56:50", "198", "10", "45th Ave",
			"0", "0", "9289.4", "0", "", ""}))
	require.NoError(t, err)
	arrTime, err := st.ArrivalTime.MarshalText()
	require.NoError(t, err)
	depTime, err := st.DepartureTime.MarshalText()
	require.NoError(t, err)
	assert.Equal(t, "06:56:50", string(arrTime))
	assert.Equal(t, "06:56:50", string(depTime))

	assert.Equal(t, 0, st.PickupType)
	assert.Equal(t, "45th Ave", *st.StopHeadsign)
	assert.Equal(t, 9289.4, *st.ShapeDistTraveled)
}

// A GTFS time may run past 24:00:00 for a trip that crosses midnight.
func TestStopTimeFromRowPastMidnight(t *testing.T) {
	st, err := NewStopTimeFromRow(testRow(
		[]string{"trip_id", "arrival_time", "departure_time", "stop_id", "stop_sequence"},
		[]string{"t1", "25:10:00", "25:12:30", "s1", "3"}))
	require.NoError(t, err)
	arr, err := st.ArrivalTime.MarshalText()
	require.NoError(t, err)
	assert.Equal(t, "25:10:00", string(arr))
	assert.Equal(t, 3, st.StopSequence)
	// Continuous pickup/drop-off default to 1, not 0, when the feed omits them.
	assert.Equal(t, 1, st.ContinuousPickup)
	assert.Equal(t, 1, st.ContinuousDropOff)
}

// A minimal stops.txt, without TriMet's direction/position extensions or any
// of the optional spec columns.
func TestNewStopFromRowMinimal(t *testing.T) {
	st, err := NewStopFromRow(testRow(
		[]string{"stop_id", "stop_name", "stop_lat", "stop_lon"},
		[]string{"1418", "Anand Vihar ISBT", "28.6469", "77.3159"}))
	require.NoError(t, err)
	assert.Equal(t, "1418", st.ID)
	assert.Equal(t, "Anand Vihar ISBT", st.Name)
	assert.InDelta(t, 28.6469, st.Lat, 0.00001)
	assert.InDelta(t, 77.3159, st.Lon, 0.00001)
	assert.Equal(t, "", st.Direction)
	assert.Equal(t, 0, st.LocationType)
}

// route_sort_order and route_text_color are optional; the database columns
// they feed are NOT NULL, so they must fall back to usable zero values.
func TestNewRouteFromRowMinimal(t *testing.T) {
	r, err := NewRouteFromRow(testRow(
		[]string{"route_id", "route_short_name", "route_long_name", "route_type"},
		[]string{"101", "101", "Nehru Place - Mehrauli", "3"}))
	require.NoError(t, err)
	assert.Equal(t, "101", r.RouteID)
	assert.Equal(t, int(RouteTypeBus), r.Type)
	assert.Equal(t, 0, r.SortOrder)
	assert.Equal(t, "", r.TextColor)
}
