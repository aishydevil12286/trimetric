package logic

import (
	"testing"
	"time"

	"github.com/bsdavidson/trimetric/gtfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func strPtr(s string) *string { return &s }

// nowSeconds is the current unix time; positions older than five minutes are
// filtered out of query results.
func nowSeconds() int64 { return time.Now().Unix() }

// GTFS-realtime reports bearing as a float. The column used to be a smallint,
// which meant every insert failed with a type error against any feed that
// sends a fractional bearing, and no vehicle ever reached the map.
func TestUpsertVehiclePositionFractionalBearing(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	vds := VehicleSQLDataset{DB: db}
	v := gtfs.VehiclePosition{
		Trip:    gtfs.TripDescriptor{TripID: strPtr("t1"), RouteID: strPtr("101")},
		Vehicle: gtfs.VehicleDescriptor{ID: strPtr("DL1PC0001"), Label: strPtr("DL1PC0001")},
		Position: gtfs.Position{
			Latitude:  28.6328,
			Longitude: 77.2197,
			Bearing:   86.81691,
		},
		Timestamp: uint64(nowSeconds()),
	}
	require.NoError(t, vds.UpsertVehiclePosition(&v))

	got, err := vds.FetchVehiclePositions(0)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.InDelta(t, 86.81691, got[0].Position.Bearing, 0.001)
}

// GTFS defines shape_id as an arbitrary string, and the shapes table stores it
// as text. Both shape queries used to scan it into an int, which only worked
// because TriMet numbers its shapes.
func TestFetchRouteShapesWithStringShapeIDs(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	lds := LoaderSQLDataset{DB: db}
	require.NoError(t, lds.LoadGTFSData(gtfs.StaticSource{
		File: zipFixture(t, "./testdata/delhi"),
	}))

	// The fixture's shape ids look like "sh_101", and its routes are buses.
	shds := ShapeSQLDataset{DB: db, RouteLineTypes: []int{int(gtfs.RouteTypeBus)}}
	shapes, err := shds.FetchRouteShapes()
	require.NoError(t, err)
	require.NotEmpty(t, shapes)

	routeIDs := map[string]bool{}
	for _, s := range shapes {
		routeIDs[s.RouteID] = true
		assert.NotEmpty(t, s.Points)
	}
	assert.True(t, routeIDs["101"], "expected a shape for route 101, got %v", routeIDs)
}

// Route lines default to rail-like route types. A bus-only feed draws none
// unless buses are asked for explicitly.
func TestFetchRouteShapesDefaultsToRailTypes(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	lds := LoaderSQLDataset{DB: db}
	require.NoError(t, lds.LoadGTFSData(gtfs.StaticSource{
		File: zipFixture(t, "./testdata/delhi"),
	}))

	shds := ShapeSQLDataset{DB: db}
	shapes, err := shds.FetchRouteShapes()
	require.NoError(t, err)
	assert.Empty(t, shapes)
}

// FetchTripShapes backs the arrivals endpoint, so it has to survive string
// shape ids too.
func TestFetchTripShapesWithStringShapeIDs(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	lds := LoaderSQLDataset{DB: db}
	require.NoError(t, lds.LoadGTFSData(gtfs.StaticSource{
		File: zipFixture(t, "./testdata/delhi"),
	}))

	shds := ShapeSQLDataset{DB: db}
	shapes, err := shds.FetchTripShapes([]string{"t_101_up"})
	require.NoError(t, err)
	require.Contains(t, shapes, "t_101_up")
	assert.NotEmpty(t, shapes["t_101_up"].Points)
	assert.Equal(t, "101", shapes["t_101_up"].RouteID)
}
