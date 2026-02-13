package app

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/auth"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/config"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/handler"
	myMiddleware "github.com/squaredbusinessman/gofemart-loyalty-service/internal/middleware"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/repository"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/server"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/service"
	"github.com/squaredbusinessman/gofemart-loyalty-service/migrations"
	"go.uber.org/zap"
)

func Run(ctx context.Context, cfg config.Config, log *zap.Logger) error {
	// контекст-таймаут для старта БД, чтобы избежать зависаний при запуске сервиса
	startCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(startCtx, cfg.DatabaseURI)
	if err != nil {
		return fmt.Errorf("init pgxpool: %w", err)
	}
	defer pool.Close()

	// пингуем БД чтобы явно понять что соединение установлено
	if err = pool.Ping(startCtx); err != nil {
		return fmt.Errorf("db ping: %w", err)
	}

	// проверяем запуск схемы миграций
	if err = migrations.Up(pool, "migrations"); err != nil {
		return fmt.Errorf("migrations up: %w", err)
	}

	// инициализируем хранилище, но пока без ручек
	store := repository.NewDBStorage(pool)
	// менеджер токена
	tm, err := auth.NewTokenManager(cfg.AuthSecret, cfg.AuthTokenTTL)
	if err != nil {
		return fmt.Errorf("init token manager: %w", err)
	}
	// сервис заказов
	orderService := service.NewOrderService(store)
	// хэндлеры
	h := handler.NewHandler(store, tm, orderService)
	// собираем ручки и миддлвары
	resultHandlers := buildHandlers(log, h, tm)

	// запуск http server из одноименного пакета сервиса
	// таймауты пока что хардкодим
	srv, err := server.New(server.Config{
		Addr:              cfg.RunAddress,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		ShutdownTimeout:   10 * time.Second,
	}, resultHandlers, log)
	if err != nil {
		return fmt.Errorf("init http server: %w", err)
	}

	return srv.Run(ctx)
}

func buildHandlers(_ *zap.Logger, h *handler.Handler, tp myMiddleware.TokenParser) http.Handler {
	r := chi.NewRouter()
	r.Use(chiMiddleware.StripSlashes)

	// открытые маршруты
	r.Post("/api/user/register", h.Register)
	r.Post("/api/user/login", h.Login)

	// закрытые маршруты
	r.Group(func(protectedRoutes chi.Router) {
		protectedRoutes.Use(myMiddleware.AuthMiddleware(tp))
		protectedRoutes.Post("/api/user/orders", h.UploadOrder)
		protectedRoutes.Get("/api/user/orders", h.GetOrders)
	})

	return myMiddleware.Conveyor(r)
}
