-- +goose Up
-- Feeds may describe service with calendar.txt, with calendar_dates.txt, or
-- with both. Only calendar_dates was stored before, which left feeds that
-- describe regular weekly service (Delhi's among them) with no schedule at
-- all.
CREATE TABLE calendar (
  service_id text NOT NULL PRIMARY KEY,
  monday boolean NOT NULL DEFAULT false,
  tuesday boolean NOT NULL DEFAULT false,
  wednesday boolean NOT NULL DEFAULT false,
  thursday boolean NOT NULL DEFAULT false,
  friday boolean NOT NULL DEFAULT false,
  saturday boolean NOT NULL DEFAULT false,
  sunday boolean NOT NULL DEFAULT false,
  start_date date NOT NULL,
  end_date date NOT NULL,
  FOREIGN KEY (service_id) REFERENCES services (id) ON DELETE CASCADE
);
CREATE INDEX ON calendar (start_date, end_date);

-- service_active answers "does this service run on this date?" the way the
-- GTFS spec defines it: a calendar_dates exception is authoritative for the
-- date it names, otherwise the weekly pattern in calendar applies, and a
-- service the feed says nothing about does not run.
-- +goose StatementBegin
CREATE FUNCTION service_active(p_service_id text, p_date date) RETURNS boolean AS $$
  SELECT COALESCE(
    (
      SELECT cd.exception_type = 1
      FROM calendar_dates cd
      WHERE cd.service_id = p_service_id AND cd.date = p_date
    ),
    (
      SELECT CASE extract(dow FROM p_date)
        WHEN 0 THEN c.sunday
        WHEN 1 THEN c.monday
        WHEN 2 THEN c.tuesday
        WHEN 3 THEN c.wednesday
        WHEN 4 THEN c.thursday
        WHEN 5 THEN c.friday
        WHEN 6 THEN c.saturday
      END
      FROM calendar c
      WHERE c.service_id = p_service_id AND p_date BETWEEN c.start_date AND c.end_date
    ),
    false);
$$ LANGUAGE sql STABLE;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION service_active(text, date);
DROP TABLE calendar;
