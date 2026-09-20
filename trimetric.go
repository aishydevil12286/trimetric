// Package trimetric wires together the GTFS ingestion pipeline and the HTTP
// API that serves it.
package trimetric

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	_ "net/http/pprof" // Import the pprof package to add the HTTP handlers
	"sync"
	"time"

	"github.com/bsdavidson/trimetric/api"
	"github.com/bsdavidson/trimetric/gtfs"
	"github.com/bsdavidson/trimetric/logic"
	"github.com/pkg/errors"
	"github.com/pressly/goose/v3"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Config describes everything Run needs to ingest a feed and serve it.
type Config struct {
	// Addr is the address the web app and API are served on.
	Addr string
	// DebugAddr serves pprof and Prometheus metrics.
	DebugAddr string
	// WebPath is the directory holding the built front end.
	WebPath string

	// APIKey authenticates realtime feed requests.
	APIKey string
	// Static says where to get the static GTFS feed.
	Static gtfs.StaticSource
	// StaticInterval is how often to reload the static feed.
	StaticInterval time.Duration

	// VehiclePositionsURL is the GTFS-realtime VehiclePositions feed.
	VehiclePositionsURL string
	// VehicleInterval is how often to poll it.
	VehicleInterval time.Duration

	// TripUpdatesURL is the GTFS-realtime TripUpdates feed. Empty disables
	// trip updates, which is the default: not every agency publishes one.
	TripUpdatesURL string
	// TripUpdateInterval is how often to poll it.
	TripUpdateInterval time.Duration

	// Timezone is the agency timezone used to resolve GTFS service days.
	Timezone string

	// KafkaBrokers routes realtime data through Kafka rather than straight
	// into Postgres. Empty, the default, keeps the whole pipeline in-process.
	KafkaBrokers []string
}

// OpenDB connects to the database.
func OpenDB(url string) (*sql.DB, error) {
	db, err := sql.Open("postgres", url)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if err := db.Ping(); err != nil {
		return nil, errors.WithStack(err)
	}
	return db, nil
}

// MigrateDB applies any updates needed to the database to bring it up to the
// latest version of the schema. If migrate is false, then it won't modify the DB
// but will instead return an error if the database is not up-to-date.
func MigrateDB(db *sql.DB, path string, migrate bool) error {
	if err := goose.SetDialect("postgres"); err != nil {
		return errors.WithStack(err)
	}

	dbVer, err := goose.EnsureDBVersion(db)
	if err != nil {
		return errors.WithStack(err)
	}
	migrations, err := goose.CollectMigrations(path, 0, goose.MaxVersion)
	if err != nil {
		return errors.WithStack(err)
	}
	lm, err := migrations.Last()
	if err != nil {
		return errors.WithStack(err)
	}
	if !migrate {
		if lm.Version > dbVer {
			return errors.Errorf("database version update required, latest version: %d, current version:%d", lm.Version, dbVer)
		}
		return nil
	}

	if err := goose.Up(db, path); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// startPipeline connects a realtime feed's producer to the consumer that
// writes it into Postgres.
//
// With no brokers configured the two are joined directly in-process. With
// brokers, the producer publishes to a Kafka topic and a consumer goroutine
// reads it back off, which is what lets ingestion and storage be scaled or
// deployed apart.
func startPipeline(ctx context.Context, cancel context.CancelFunc, wg *sync.WaitGroup, brokers []string, topic string, consume logic.ConsumerFunc) (logic.Producer, error) {
	if len(brokers) == 0 {
		return logic.NewDirectProducer(ctx, consume), nil
	}

	p, err := logic.NewKafkaProducer(topic, brokers)
	if err != nil {
		return nil, errors.Wrapf(err, "connecting to Kafka for topic %s", topic)
	}

	wg.Add(1)
	go func() {
		defer cancel()
		defer wg.Done()
		if err := logic.ConsumeKafkaTopic(ctx, consume, topic, brokers); err != nil {
			log.Printf("kafka consumer %s: %v", topic, err)
		}
	}()

	return p, nil
}

// Run starts all the processes for Trimetric.
func Run(ctx context.Context, cancel context.CancelFunc, db *sql.DB, cfg Config) error {
	vds := &logic.VehicleSQLDataset{DB: db}
	sds := &logic.StopSQLDataset{DB: db, Timezone: cfg.Timezone}
	shds := &logic.ShapeSQLDataset{DB: db}
	rds := &logic.RouteSQLDataset{DB: db}
	lds := &logic.LoaderSQLDataset{DB: db}
	tuds := &logic.TripUpdateSQLDataset{DB: db}

	wg := &sync.WaitGroup{}
	defer func() {
		cancel()
		wg.Wait()
	}()

	// spawn runs fn in the background and tears the whole process down when
	// it returns, so a dead ingestion goroutine cannot leave the app serving
	// data that has quietly stopped updating.
	spawn := func(fn func()) {
		wg.Add(1)
		go func() {
			defer cancel()
			defer wg.Done()
			fn()
		}()
	}

	if len(cfg.KafkaBrokers) > 0 {
		log.Printf("routing realtime data through Kafka at %v", cfg.KafkaBrokers)
	}

	vehicleProducer, err := startPipeline(ctx, cancel, wg, cfg.KafkaBrokers,
		logic.VehiclePositionsTopic, vds.UpsertVehiclePositionBytes)
	if err != nil {
		return err
	}
	defer func() {
		if err := vehicleProducer.Close(); err != nil {
			log.Println(err)
		}
	}()

	spawn(func() {
		log.Printf("polling vehicle positions from %s every %s",
			cfg.VehiclePositionsURL, cfg.VehicleInterval)
		if err := logic.ProduceVehiclePositions(ctx, vehicleProducer,
			cfg.VehiclePositionsURL, cfg.APIKey, cfg.VehicleInterval); err != nil {
			log.Println(err)
		}
	})

	if cfg.TripUpdatesURL != "" {
		tripProducer, err := startPipeline(ctx, cancel, wg, cfg.KafkaBrokers,
			logic.TripUpdatesTopic, tuds.UpdateTripUpdateBytes)
		if err != nil {
			return err
		}
		defer func() {
			if err := tripProducer.Close(); err != nil {
				log.Println(err)
			}
		}()

		spawn(func() {
			log.Printf("polling trip updates from %s every %s",
				cfg.TripUpdatesURL, cfg.TripUpdateInterval)
			if err := logic.ProduceTripUpdates(ctx, cfg.TripUpdatesURL, cfg.APIKey,
				tripProducer, cfg.TripUpdateInterval); err != nil {
				log.Println(err)
			}
		})
	} else {
		log.Println("no trip updates feed configured, skipping trip updates")
	}

	spawn(func() {
		logic.PollGTFSData(ctx, lds, cfg.Static, cfg.StaticInterval)
	})

	http.Handle("/metrics", promhttp.Handler())
	internalSrv := &http.Server{Addr: cfg.DebugAddr, Handler: http.DefaultServeMux}
	log.Printf("metrics and pprof listening on %s", cfg.DebugAddr)
	go func() {
		if err := internalSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Println(err)
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/vehicles", api.HandleVehiclePositions(vds))
	mux.HandleFunc("/api/v1/arrivals", api.HandleArrivals(sds, shds))
	mux.HandleFunc("/api/v1/routes", api.HandleRoutes(rds))
	mux.HandleFunc("/api/v1/routes/lines", api.HandleRouteLines(shds))
	mux.HandleFunc("/api/v1/shapes", api.HandleShapes(shds))
	mux.HandleFunc("/api/v1/stops", api.HandleStops(sds))
	mux.HandleFunc("/api/v1/trip", api.HandleTripUpdates(tuds))
	mux.HandleFunc("/ws", api.HandleWS(vds, shds, sds, rds))
	mux.Handle("/", http.FileServer(http.Dir(cfg.WebPath)))
	srv := &http.Server{Addr: cfg.Addr, Handler: mux}
	log.Printf("serving requests on %s", cfg.Addr)

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Println(err)
		}
		if err := internalSrv.Shutdown(shutdownCtx); err != nil {
			log.Println(err)
		}
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return errors.WithStack(err)
	}

	return nil
}
