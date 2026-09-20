package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/bsdavidson/trimetric"
	"github.com/bsdavidson/trimetric/gtfs"
	_ "github.com/lib/pq"
)

// defaultAPIKeyPath is where Docker mounts the key when it is supplied as a
// Compose secret. It is only read if no key is given another way.
const defaultAPIKeyPath = "/run/secrets/otd-api-key"

// env returns the named environment variable, or def when it is unset.
func env(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

// readAPIKey resolves the realtime feed key from, in order of precedence, the
// --api-key flag, OTD_API_KEY, or a secrets file.
func readAPIKey(flagValue, path string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	if v := os.Getenv("OTD_API_KEY"); v != "" {
		return strings.TrimSpace(v), nil
	}

	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	} else if err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	return strings.TrimSpace(string(b)), nil
}

func main() {
	addr := flag.String("addr", env("ADDR", ":80"), "Address to bind to")
	debugAddr := flag.String("debug-addr", env("DEBUG_ADDR", ":9876"), "Address to serve metrics and pprof on")
	webPath := flag.String("web-path", env("WEB_PATH", "./web/dist"), "Path to website assets")

	pgURL := flag.String("pg-url", os.Getenv("DATABASE_URL"), "Postgres connection URL; overrides the individual pg-* flags")
	pgUser := flag.String("pg-user", env("PGUSER", "trimetric"), "Postgres username")
	pgPassword := flag.String("pg-password", env("PGPASSWORD", "example"), "Postgres password")
	pgHost := flag.String("pg-host", env("PGHOST", "postgres"), "Postgres hostname")
	pgDatabase := flag.String("pg-database", env("PGDATABASE", "trimetric"), "Postgres database")

	migrate := flag.Bool("migrate", true, "Perform database migrations if true.")
	migratePath := flag.String("migrate-path", env("MIGRATE_PATH", "./migrations"), "Path to migration files")

	apiKey := flag.String("api-key", "", "Realtime feed API key. Falls back to $OTD_API_KEY then "+defaultAPIKeyPath)
	apiKeyPath := flag.String("api-key-path", env("OTD_API_KEY_PATH", defaultAPIKeyPath), "File to read the realtime feed API key from")

	staticURL := flag.String("gtfs-static-url", env("GTFS_STATIC_URL", gtfs.DefaultStaticURL), "URL of the static GTFS zip")
	staticFile := flag.String("gtfs-static-file", env("GTFS_STATIC_FILE", ""), "Path to a static GTFS zip on disk. Takes precedence over -gtfs-static-url")
	staticCSRF := flag.Bool("gtfs-static-csrf", env("GTFS_STATIC_CSRF", "true") == "true", "Fetch the static feed with a CSRF-protected POST, as the OTD portal requires")
	staticInterval := flag.Duration("gtfs-static-interval", 24*time.Hour, "How often to reload the static GTFS feed")

	vehicleURL := flag.String("vehicle-positions-url", env("GTFS_VEHICLE_POSITIONS_URL", gtfs.DefaultVehiclePositionsURL), "GTFS-realtime VehiclePositions feed URL")
	vehicleInterval := flag.Duration("vehicle-positions-interval", 10*time.Second, "How often to poll vehicle positions")

	tripUpdatesURL := flag.String("trip-updates-url", env("GTFS_TRIP_UPDATES_URL", gtfs.DefaultTripUpdatesURL), "GTFS-realtime TripUpdates feed URL. Empty disables trip updates")
	tripUpdateInterval := flag.Duration("trip-updates-interval", 30*time.Second, "How often to poll trip updates")

	timezone := flag.String("timezone", env("GTFS_TIMEZONE", gtfs.DefaultTimezone), "Agency timezone, used to resolve GTFS service days")
	routeLineTypes := flag.String("route-line-types", env("GTFS_ROUTE_LINE_TYPES", ""), "Comma-separated GTFS route types whose shapes are drawn as lines. Empty draws rail-like routes (0,1,2); add 3 for buses")

	kafkaBrokers := flag.String("kafka-brokers", os.Getenv("KAFKA_BROKERS"), "Comma-separated Kafka brokers. Empty keeps the realtime pipeline in-process")

	flag.Parse()

	log.SetFlags(log.Lshortfile)

	key, err := readAPIKey(*apiKey, *apiKeyPath)
	if err != nil {
		log.Fatal(err)
	}
	if key == "" && *staticFile == "" {
		log.Println("warning: no realtime API key configured; the realtime feed will be rejected. " +
			"Set -api-key, $OTD_API_KEY, or write the key to " + *apiKeyPath)
	}

	dsn := *pgURL
	if dsn == "" {
		dsn = fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable",
			url.QueryEscape(*pgUser), url.QueryEscape(*pgPassword), *pgHost, *pgDatabase)
	}

	var db *sql.DB
	for {
		db, err = trimetric.OpenDB(dsn)
		if err == nil {
			break
		}
		log.Println("error connecting to postgres:", err)
		log.Println("retrying in 1 second")
		time.Sleep(time.Second)
	}

	if err := trimetric.MigrateDB(db, *migratePath, *migrate); err != nil {
		log.Fatal(err)
	}

	var brokers []string
	if *kafkaBrokers != "" {
		brokers = strings.Split(*kafkaBrokers, ",")
	}

	var lineTypes []int
	for _, s := range strings.Split(*routeLineTypes, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			log.Fatalf("invalid -route-line-types %q: %v", *routeLineTypes, err)
		}
		lineTypes = append(lineTypes, n)
	}

	cfg := trimetric.Config{
		Addr:      *addr,
		DebugAddr: *debugAddr,
		WebPath:   *webPath,
		APIKey:    key,
		Static: gtfs.StaticSource{
			File: *staticFile,
			URL:  *staticURL,
			CSRF: *staticCSRF,
		},
		StaticInterval:      *staticInterval,
		VehiclePositionsURL: *vehicleURL,
		VehicleInterval:     *vehicleInterval,
		TripUpdatesURL:      *tripUpdatesURL,
		TripUpdateInterval:  *tripUpdateInterval,
		Timezone:            *timezone,
		RouteLineTypes:      lineTypes,
		KafkaBrokers:        brokers,
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-signals
		cancel()
	}()

	if err := trimetric.Run(ctx, cancel, db, cfg); err != nil {
		log.Fatal(err)
	}
}
