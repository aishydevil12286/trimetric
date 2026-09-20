// Package gtfs provides types and helpers for reading static GTFS feeds and
// GTFS-realtime feeds.
//
// Nothing here is tied to a particular agency: every endpoint is supplied by
// the caller. The defaults below target Delhi's Open Transit Data portal
// (https://otd.delhi.gov.in), which publishes DTC and DIMTS cluster buses.
package gtfs

// Default endpoints for Delhi's Open Transit Data portal.
const (
	// DefaultStaticURL is the OTD static GTFS download. The portal serves it
	// from a CSRF-protected Django view, so it needs a POST rather than a
	// plain GET -- see StaticSource.
	DefaultStaticURL = "https://otd.delhi.gov.in/data/static/"

	// DefaultVehiclePositionsURL is the OTD GTFS-realtime VehiclePositions
	// feed. It is refreshed roughly every 10 seconds.
	DefaultVehiclePositionsURL = "https://otd.delhi.gov.in/api/realtime/VehiclePositions.pb"

	// DefaultTripUpdatesURL is where a GTFS-realtime TripUpdates feed would
	// live. OTD does not currently document one, so trip updates stay off
	// unless a URL is configured explicitly.
	DefaultTripUpdatesURL = ""

	// DefaultTimezone is the agency timezone, used to resolve which GTFS
	// service day "now" falls in.
	DefaultTimezone = "Asia/Kolkata"

	// APIKeyParam is the query parameter OTD expects the private key in.
	// TriMet and some other agencies use "appID" instead.
	APIKeyParam = "key"
)
