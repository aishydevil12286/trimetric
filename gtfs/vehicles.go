package gtfs

//go:generate msgp

// RouteType indicates the type of vehicle serving the route
type RouteType int

// Defines the types of vehicles serving a route.
const (
	RouteTypeTram RouteType = iota
	RouteTypeSubway
	RouteTypeRail
	RouteTypeBus
	RouteTypeFerry
	RouteTypeCableCar
	RouteTypeGondola
	RouteTypeFunicular
)

// VehiclePosition is the realtime position information for a given vehicle.
type VehiclePosition struct {
	Trip                TripDescriptor    `json:"trip" msg:"trip"`
	Vehicle             VehicleDescriptor `json:"vehicle" msg:"vehicle"`
	Position            Position          `json:"position" msg:"position"`
	CurrentStopSequence uint32            `json:"current_stop_sequence" msg:"current_stop_sequence"`
	StopID              string            `json:"stop_id" msg:"stop_id"`
	CurrentStatus       int32             `json:"current_status" msg:"current_status"`
	Timestamp           uint64            `json:"timestamp" msg:"timestamp"`
	CongestionLevel     int32             `json:"congestion_level" msg:"congestion_level"`
	OccupancyStatus     int32             `json:"occupancy_status" msg:"occupancy_status"`
}

// VehicleDescriptor contains identification information for a vehicle
// performing a trip.
type VehicleDescriptor struct {
	ID    *string `json:"id" msg:"id"`
	Label *string `json:"label" msg:"label"`
	// LicensePlate *string `json:"license_plate"`
}

// Position is a geographic position of a vehicle.
type Position struct {
	Latitude  float32 `json:"lat"  msg:"lat"`
	Longitude float32 `json:"lng"  msg:"lng"`
	Bearing   float32 `json:"bearing"  msg:"bearing"`
	Odometer  float64 `json:"odometer"  msg:"odometer"`
	Speed     float32 `json:"speed"  msg:"speed"`
}

// RequestVehiclePositions downloads a GTFS-realtime VehiclePositions feed and
// converts each entity into a VehiclePosition.
//
// Entities without a vehicle id are skipped: the id is the primary key the
// positions are stored under, so an entity without one cannot be tracked.
func RequestVehiclePositions(feedURL, apiKey string) ([]VehiclePosition, error) {
	feed, err := fetchFeed(feedURL, apiKey)
	if err != nil {
		return nil, err
	}

	var vps []VehiclePosition
	for _, e := range feed.Entity {
		v := e.GetVehicle()
		if v == nil || v.GetVehicle().GetId() == "" {
			continue
		}

		vp := VehiclePosition{
			CurrentStopSequence: v.GetCurrentStopSequence(),
			StopID:              v.GetStopId(),
			CurrentStatus:       int32(v.GetCurrentStatus()),
			Timestamp:           v.GetTimestamp(),
			CongestionLevel:     int32(v.GetCongestionLevel()),
			OccupancyStatus:     int32(v.GetOccupancyStatus()),
		}
		if v.Trip != nil {
			vp.Trip = TripDescriptor{
				TripID:  v.Trip.TripId,
				RouteID: v.Trip.RouteId,
			}
		}
		vp.Vehicle = VehicleDescriptor{
			ID:    v.Vehicle.Id,
			Label: v.Vehicle.Label,
		}
		if v.Position != nil {
			vp.Position = Position{
				Latitude:  v.Position.GetLatitude(),
				Longitude: v.Position.GetLongitude(),
				Bearing:   v.Position.GetBearing(),
				Odometer:  v.Position.GetOdometer(),
				Speed:     v.Position.GetSpeed(),
			}
		}
		vps = append(vps, vp)
	}
	return vps, nil
}
