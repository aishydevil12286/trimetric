package logic

import (
	"database/sql"
	"fmt"

	"github.com/bsdavidson/trimetric/gtfs"

	postgis "github.com/cridenour/go-postgis"
	"github.com/lib/pq"
	"github.com/pkg/errors"
)

// ShapeDataset provides methods to query and update a database table of Shapes
type ShapeDataset interface {
	FetchRouteShapes() ([]*RouteShape, error)
	FetchShapes(routeIDS, shapeIDs []string) ([]Shape, error)
	FetchTripShapes(tripIDs []string) (map[string]*TripShape, error)
}

// ShapeSQLDataset stores a DB instance and provides access to methods to
// retrieve and update shapes from the database
type ShapeSQLDataset struct {
	DB *sql.DB

	// RouteLineTypes selects which GTFS route types get their shape drawn as
	// a line on the map. Empty means the default set.
	RouteLineTypes []int
}

// defaultRouteLineTypes are the rail-like route types: tram, subway and rail.
//
// Drawing every route's shape is only reasonable for a network with a handful
// of trunk lines. A bus network has thousands of shapes, which is both an
// unreadable map and a very large payload, so buses are opt-in.
var defaultRouteLineTypes = []int{
	int(gtfs.RouteTypeTram),
	int(gtfs.RouteTypeSubway),
	int(gtfs.RouteTypeRail),
}

// routeLineTypes returns the configured route types, or the default set.
func (sd *ShapeSQLDataset) routeLineTypes() []int {
	if len(sd.RouteLineTypes) == 0 {
		return defaultRouteLineTypes
	}
	return sd.RouteLineTypes
}

// Shape represents the shape line for a given route.
type Shape struct {
	ID          string  `json:"id"`
	DirectionID int     `json:"direction_id"`
	RouteID     string  `json:"route_id"`
	Point       []Point `json:"point"`
}

// Point represents a single point along a route shape
type Point struct {
	Lat          float64 `json:"lat"`
	Lng          float64 `json:"lng"`
	DistTraveled float64 `json:"dist_traveled"`
}

// FetchShapes takes a slice of routes  or shapeID's and returns an array of shapes.
func (sd *ShapeSQLDataset) FetchShapes(routeIDs, shapeIDs []string) ([]Shape, error) {
	// TODO: Query BAsed on trip_IDS
	q := `
		SELECT id, pt_lon_lat, dist_traveled, route_shapes.route_id, route_shapes.direction_id
		FROM shapes
	`
	var args []interface{}

	if len(routeIDs) > 0 {
		q += `
			JOIN (
				SELECT DISTINCT trips.shape_id, routes.id as route_id, trips.direction_id as direction_id
				FROM routes
				JOIN trips ON trips.route_id = routes.id
				AND routes.id = ANY($1)
			) AS route_shapes ON route_shapes.shape_id = shapes.id
		`
		args = append(args, pq.Array(routeIDs))
	}

	if len(shapeIDs) > 0 {
		q += fmt.Sprintf("WHERE id = ANY($%d)", len(args)+1)
		args = append(args, pq.Array(shapeIDs))
	}

	q += `ORDER BY id, pt_sequence ASC`

	rows, err := sd.DB.Query(q, args...)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer rows.Close()
	shapes := []Shape{}

	var lastShape *Shape
	for rows.Next() {
		var id string
		var routeID string
		var direction int
		var p Point
		var lonLat postgis.PointS
		err := rows.Scan(&id, &lonLat, &p.DistTraveled, &routeID, &direction)
		if err != nil {
			return nil, err
		}
		p.Lat = lonLat.Y
		p.Lng = lonLat.X

		if lastShape == nil || lastShape.ID != id {
			shapes = append(shapes, Shape{ID: id, RouteID: routeID, DirectionID: direction})
			lastShape = &shapes[len(shapes)-1]
		}
		lastShape.Point = append(lastShape.Point, p)

	}

	return shapes, nil
}
