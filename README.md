# trimetric

Trimetric is a realtime visualization of a transit system, built on [GTFS] and
[GTFS-realtime] feeds. This fork visualizes **Delhi's bus network** — DTC and
DIMTS cluster buses — using the feeds published by [Open Transit Data Delhi].

It downloads the static GTFS schedule periodically and ingests it into a local
[Postgres] database for querying with PostGIS. The realtime vehicle positions
feed is polled and written to the same database.

The front end is a responsive web app built in React, using [MapLibre] and
[deck.gl] to render the map and icon layers. It needs no map access token.
When zoomed in, Trimetric shows a dynamic list of the stops visible on screen,
which you can click to see upcoming arrivals.

> Upstream, Trimetric visualizes [TriMet] in Portland, OR. See
> [Differences from upstream](#differences-from-upstream).


## Running it

You need [Docker] and an **Open Transit Data API key**, which is free: register
at <https://otd.delhi.gov.in/data/realtime/> and you will be issued a private
key.

```sh
git clone <this repo>
cd trimetric
echo "OTD_API_KEY=your_key_here" > .env
docker compose up --build
```

Then open <http://localhost:8080>.

The first start migrates the database and downloads the static schedule, which
can take a few minutes on a feed the size of Delhi's. Watch the logs for
`gtfs loader: static feed loaded`. Vehicles start appearing on the map as soon
as the first realtime poll lands.

### If the static feed will not download

The portal serves its static GTFS zip from a CSRF-protected form rather than a
plain file, and that is the most fragile part of the setup: if the portal
changes, the download breaks. The app tells you when the response was not a zip
rather than reporting a corrupt archive.

The reliable fallback is to fetch the zip yourself, from the Static Data
section of <https://otd.delhi.gov.in>, and point the app at it:

```sh
mkdir -p data
cp ~/Downloads/gtfs.zip data/gtfs.zip
echo "GTFS_STATIC_FILE=/data/gtfs.zip" >> .env
docker compose up --build
```

`./data` is mounted read-only at `/data` inside the container.

### Reloading the schedule

The schedule is reloaded every 24 hours, and the time of the last load is kept
in the database so restarts do not re-download a feed that is still fresh. To
force a reload from scratch:

```sh
make clean   # drops the database volume
docker compose up --build
```


## Configuration

Everything is set by environment variable (via `.env`) or by flag. Run
`docker compose run --rm api -h` for the full list.

| Variable | Default | Purpose |
| --- | --- | --- |
| `OTD_API_KEY` | *(required)* | Your Open Transit Data private key |
| `GTFS_STATIC_URL` | `https://otd.delhi.gov.in/data/static/` | Static schedule download |
| `GTFS_STATIC_FILE` | *(empty)* | Path to an already-downloaded zip; wins over the URL |
| `GTFS_STATIC_CSRF` | `true` | Fetch the static feed with a CSRF POST, as the portal requires |
| `GTFS_VEHICLE_POSITIONS_URL` | `https://otd.delhi.gov.in/api/realtime/VehiclePositions.pb` | Realtime vehicle positions |
| `GTFS_TRIP_UPDATES_URL` | *(empty)* | Realtime trip updates; empty disables them |
| `GTFS_TIMEZONE` | `Asia/Kolkata` | Agency timezone, used to resolve GTFS service days |
| `GTFS_ROUTE_LINE_TYPES` | *(empty)* | Route types drawn as lines on the map; empty means rail-like only (see below) |
| `KAFKA_BROKERS` | *(empty)* | Empty keeps the realtime pipeline in-process |
| `POSTGRES_PASSWORD` | `example` | Database password |

Because nothing above is Delhi-specific in the code, pointing this at another
agency's GTFS feeds is a matter of changing these values.

### Pointing it at another city

For example, to run it against the original TriMet feeds:

```sh
GTFS_STATIC_URL=https://developer.trimet.org/schedule/gtfs.zip
GTFS_STATIC_CSRF=false
GTFS_VEHICLE_POSITIONS_URL=https://developer.trimet.org/ws/gtfs/VehiclePositions
GTFS_TIMEZONE=America/Los_Angeles
```

Note that TriMet expects its key in an `appID` query parameter rather than
`key`; that name is a constant in `gtfs/feed.go`.


## Seeing it without a key

`make demo` generates a synthetic bus network and serves it as GTFS plus
GTFS-realtime, so you can look at the visualization without an OTD key and
without waiting on the portal. It needs a Postgres, so start one first:

```sh
docker compose up -d postgres
npm install && npm run build
make demo
```

The generator lives in `scripts/mockfeed`. It builds radial corridors, ring
roads and cross-town routes around Connaught Place, hangs shared stops off
them, writes a full schedule, and then animates buses along the shapes. Nothing
in it is Delhi's real network — the place names are real localities, the
geometry is invented. Defaults produce 31 routes, ~370 stops, ~6800 trips,
~83000 stop times and 310 moving vehicles; `-radials`, `-rings`, `-crosstown`
and `-buses-per-route` change the scale.


## Route lines

The coloured lines under the vehicles are route shapes. Which routes get one is
controlled by `GTFS_ROUTE_LINE_TYPES`, and the default is rail-like types only
(tram, subway, rail).

That default is inherited from upstream, where it drew Portland's MAX light
rail and nothing else. **Delhi's feed is all buses, so by default no route
lines are drawn.** Set `GTFS_ROUTE_LINE_TYPES=3` to draw bus corridors. On a
subset of the network that looks good; on the whole of Delhi's feed it is a lot
of geometry to push over the websocket and a dense map, so try it and decide.


## Development

```sh
make dev
```

This mounts the source, serves the web app through webpack-dev-server with hot
reload on <http://localhost:8080>, and runs the API from source behind it on
8081. The web app reloads on save; after changing Go code, run
`docker compose restart api`.

To work without Docker, you need Go 1.24, Node 22, and a Postgres with PostGIS:

```sh
npm install && npm run build
go run ./cmd/trimetric \
  -addr=:8080 -pg-host=localhost -api-key=your_key_here \
  -gtfs-static-file=./data/gtfs.zip
```


## Testing

The Go tests need a Postgres with PostGIS; `docker compose up` provides one and
creates the `test_trimetric` database the tests use.

```sh
make test          # Go and JavaScript
make test-go
make test-web
```

`logic/testdata/delhi/` holds a small feed shaped like Delhi's — columns in
their own order, optional columns absent, service described by `calendar.txt`,
a UTF-8 BOM on one header, and rows referencing ids the feed never defines. The
tests zip it up and load it, which is what keeps the ingestion honest about
handling feeds it was not written against.


## Optional: Kafka

The original Trimetric moved realtime data from the feed pollers to the
database through Kafka. Here that is off by default — with no brokers
configured, producers hand messages to consumers in-process over the same
encode/decode path — because a broker shuttling messages between two goroutines
in one process costs more than it gives. To run the original arrangement:

```sh
make kafka
```

That adds a single-node Kafka (in KRaft mode, so there is no ZooKeeper) and
sets `KAFKA_BROKERS` for the API.


## Differences from upstream

Upstream targets TriMet in Portland. Beyond repointing the feeds, this fork
changes:

- **GTFS parsing is by column name, not column position.** Upstream read rows
  by fixed index, assuming TriMet's exact column order, including its
  non-standard `direction` and `position` columns in `stops.txt`. No other
  agency's feed would load.
- **`calendar.txt` is supported.** Upstream read only `calendar_dates.txt`,
  which left feeds describing regular weekly service — Delhi's among them —
  with no schedule at all. Service days now resolve through both files.
- **The timezone is configurable**, rather than `America/Los_Angeles` inline in
  the arrivals query.
- **Rows referencing undefined ids are dropped and counted** rather than
  aborting the load, because published feeds routinely contain a few.
- **Kafka, Redis and InfluxDB are no longer required.** InfluxDB was passed
  around and never used; Redis only held a timestamp, which now lives in
  Postgres; Kafka is optional as described above.
- **MapLibre replaces Mapbox GL**, so no access token is needed to see a map.
- **The build works on current toolchains**: Go modules instead of `dep`, and
  webpack 5 / Babel 7 instead of webpack 3 / Babel 6 with a hand-built fork of
  `react-map-gl`.
- The TriMet-only `/api/v1/trimet/arrivals` proxy is gone; `/api/v1/arrivals`
  is computed from GTFS and remains.
- **Vehicle bearing is stored as a float.** The column was a `smallint`, so
  every vehicle insert failed with a type error against any feed sending a
  fractional bearing, which GTFS-realtime allows and most feeds do.
- **Shape ids are read as strings.** Both shape queries scanned `shape_id`
  into an `int`, which only worked because TriMet numbers its shapes. This
  broke the arrivals endpoint, not just the map.
- **Route lines are configurable** rather than hard-coded to light rail.


## Data licence

Delhi's transit data is published by the Government of NCT of Delhi through the
Open Transit Data portal. Check the portal's terms for how you may use it. This
repository contains no OTD data; the fixtures under `logic/testdata/delhi/` are
synthetic.

The code is MIT licensed — see [LICENSE](LICENSE).


[TriMet]: https://trimet.org
[Open Transit Data Delhi]: https://otd.delhi.gov.in
[GTFS]: https://gtfs.org
[GTFS-realtime]: https://gtfs.org/realtime/reference/
[Postgres]: https://www.postgresql.org/
[MapLibre]: https://maplibre.org/
[deck.gl]: https://deck.gl/
[Docker]: https://www.docker.com/
