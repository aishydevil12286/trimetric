-- +goose Up
-- GTFS-realtime reports Position.bearing as a float, in degrees clockwise from
-- true north. Storing it as a smallint meant every vehicle insert failed with
-- a type error for any feed that sends a fractional bearing, and no vehicles
-- ever reached the map. TriMet happens to send whole degrees, which is why
-- this held up for one agency.
ALTER TABLE vehicle_positions ALTER COLUMN position_bearing TYPE real;

-- +goose Down
ALTER TABLE vehicle_positions ALTER COLUMN position_bearing TYPE smallint;
