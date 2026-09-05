package server

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/plugin-service/internal/config"
	"github.com/Wei-Shaw/sub2api/plugin-service/internal/handler"
	hostprincipal "github.com/Wei-Shaw/sub2api/plugin-service/internal/host/principal"
	"github.com/Wei-Shaw/sub2api/plugin-service/internal/media"
	"github.com/Wei-Shaw/sub2api/plugin-service/internal/pluginregistry"
	"github.com/Wei-Shaw/sub2api/plugin-service/internal/repository"
	"github.com/Wei-Shaw/sub2api/plugin-service/internal/service"
	"github.com/Wei-Shaw/sub2api/plugin-service/plugins"

	_ "github.com/lib/pq"
)

func NewRouter(cfg config.Config) http.Handler {
	history := newHistoryService(cfg)
	storage := newMediaStorage(cfg)
	registry := pluginregistry.New()
	if err := plugins.RegisterAll(registry); err != nil {
		panic(err)
	}

	app := handler.NewApp(handler.AppDeps{
		Config: cfg,
	})
	principalMiddleware := hostprincipal.NewMiddleware()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", app.Health)
	registry.RegisterRoutes(mux, pluginregistry.RouteDeps{
		Config:  cfg,
		Auth:    principalMiddleware,
		History: history,
		Media:   storage,
	})
	return app.WithCommonHeaders(mux)
}

func newMediaStorage(cfg config.Config) media.Storage {
	if !cfg.MinIO.Enabled {
		log.Print("[plugin-service] MinIO is not configured; image media remains inline")
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	storage, err := media.NewMinIOStorage(ctx, cfg.MinIO)
	if err != nil {
		panic(err)
	}
	log.Printf("[plugin-service] using MinIO bucket %s for image media", cfg.MinIO.Bucket)
	return storage
}

func newHistoryService(cfg config.Config) *service.HistoryService {
	if !cfg.Database.Enabled {
		log.Print("[plugin-service] shared database config not found; using in-memory plugin history")
		return service.NewHistoryService(repository.NewHistoryRepository())
	}

	db, err := sql.Open("postgres", cfg.Database.DSN())
	if err != nil {
		panic(err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		panic(err)
	}
	if err := repository.EnsureHistorySchema(ctx, db); err != nil {
		_ = db.Close()
		panic(err)
	}

	log.Print("[plugin-service] using shared database for plugin history")
	return service.NewHistoryService(repository.NewSQLHistoryRepository(db))
}
