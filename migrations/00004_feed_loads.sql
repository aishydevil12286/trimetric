-- +goose Up
-- Remembers when the static feed was last loaded, so a restart does not
-- re-download a feed that is still fresh. This used to live in Redis, which
-- was the only thing Redis was used for.
CREATE TABLE feed_loads (
  -- A one-row table: the CHECK constraint makes true the only usable key.
  id boolean NOT NULL PRIMARY KEY DEFAULT true CHECK (id),
  loaded_at timestamptz NOT NULL
);

-- +goose Down
DROP TABLE feed_loads;
