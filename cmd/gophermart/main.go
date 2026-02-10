package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/config"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/logger"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/middleware"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/migrations"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/repository"
	"go.uber.org/zap"
)

func main() {
	// грузим кофиг
	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	// валидируем загруженный конфиг
	if err = cfg.Validate(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	if err = logger.Initialize(cfg.LogLevel); err != nil {
		log.Fatalf("init logger failure: %v", err)
	}
	defer logger.Log.Sync()

	var dbPool *pgxpool.Pool

	if cfg.DatabaseURI != "" {
		pool, err := pgxpool.New(context.Background(), cfg.DatabaseURI)
		if err != nil {
			log.Fatalf("db pool init failure: %v", err)
		}
		dbPool = pool
		defer dbPool.Close()

		if err = migrations.Up(dbPool, "migrations"); err != nil {
			logger.Log.Error("migrations failure", zap.Error(err))
		}

		store := repository.NewDBStorage(dbPool)
	}

	r := chi.NewRouter()
	r.Use(chiMiddleware.StripSlashes)

	logger.Log.Info("Running server on: ", zap.String("address", cfg.RunAddress))
	err = http.ListenAndServe(cfg.RunAddress, middleware.Conveyor(r))
	if err != nil {
		log.Fatalf("could not start server: %v", err)
	}
}
