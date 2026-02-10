package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/app"
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
		_, _ = fmt.Fprintln(os.Stderr, "init logger:", err)
		os.Exit(1)
	}
	defer func() { _ = logger.Log.Sync() }()

	// отменяем контекст по сигналу
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, cfg, logger.Log); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
