package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/squaredbusinessman/gofemart-loyalty-service/docs"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/accrual"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/auth"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/config"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/handler"
	myMiddleware "github.com/squaredbusinessman/gofemart-loyalty-service/internal/middleware"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/repository"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/server"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/service"
	"github.com/squaredbusinessman/gofemart-loyalty-service/migrations"
	httpSwagger "github.com/swaggo/http-swagger/v2"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

func Run(ctx context.Context, cfg config.Config, log *zap.Logger) error {
	// локальные переменные, взамен магическим числам
	startTimeout := 5 * time.Second

	accrualHTTPTimeout := 3 * time.Second
	accrualMaxRetries := 2
	accrualPollInterval := 2 * time.Second
	accrualBatchSize := 20

	readHeaderTimeout := 5 * time.Second
	readTimeout := 15 * time.Second
	writeTimeout := 15 * time.Second
	idleTimeout := 60 * time.Second
	shutdownTimeout := 10 * time.Second
	// контекст-таймаут для старта БД, чтобы избежать зависаний при запуске сервиса
	startCtx, cancel := context.WithTimeout(ctx, startTimeout)
	defer cancel()

	log.Info("app starting",
		zap.String("run_address", cfg.RunAddress),
		zap.String("accrual_system_address", cfg.AccrualSystemAddress),
		zap.String("log_level", cfg.LogLevel),
	)

	log.Info("connecting to database")
	pool, err := pgxpool.New(startCtx, cfg.DatabaseURI)
	if err != nil {
		log.Error("init pgxpool failed", zap.Error(err))
		return fmt.Errorf("init pgxpool: %w", err)
	}
	defer pool.Close()

	// пингуем БД чтобы явно понять что соединение установлено
	if err = pool.Ping(startCtx); err != nil {
		log.Error("db ping failed", zap.Error(err))
		return fmt.Errorf("db ping: %w", err)
	}
	log.Info("database connected")

	// проверяем запуск схемы миграций
	if err = migrations.Up(pool, "migrations"); err != nil {
		log.Error("migrations up failed", zap.Error(err))
		return fmt.Errorf("migrations up: %w", err)
	}
	log.Info("migrations applied")

	// инициализируем хранилище, но пока без ручек
	store := repository.NewDBStorage(pool)
	// создаем accrual клиент
	accrualClient, err := accrual.NewClientWithOptions(
		cfg.AccrualSystemAddress,
		accrual.WithTimeout(accrualHTTPTimeout),
		accrual.WithMaxRetries(accrualMaxRetries),
	)
	if err != nil {
		log.Error("accrual client start failed", zap.Error(err))
		return fmt.Errorf("init accrual client: %w", err)
	}
	// инициализируем accrual worker
	accrualWorker := service.NewAccrualWorker(store, accrualClient, log, accrualPollInterval, accrualBatchSize)
	// менеджер токена
	tm, err := auth.NewTokenManager(cfg.AuthSecret, cfg.AuthTokenTTL)
	if err != nil {
		log.Error("token manager start failed", zap.Error(err))
		return fmt.Errorf("init token manager: %w", err)
	}
	// сервис заказов
	orderService := service.NewOrderService(store)
	// сервис баланса
	balanceService := service.NewBalanceService(store)
	// хэндлеры
	h := handler.NewHandler(store, tm, orderService, balanceService)
	// собираем ручки и миддлвары
	resultHandlers := buildHandlers(log, h, tm)
	// запуск http server из одноименного пакета сервиса
	// таймауты пока что хардкодим
	srv, err := server.New(server.Config{
		Addr:              cfg.RunAddress,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		ShutdownTimeout:   shutdownTimeout,
	}, resultHandlers, log)
	if err != nil {
		log.Error("init http server failed", zap.Error(err))
		return fmt.Errorf("init http server: %w", err)
	}

	// runtime-группа: HTTP сервер и фоновые воркеры живут на одном контексте
	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		log.Info("accrual worker started",
			zap.Duration("poll_interval", accrualPollInterval),
			zap.Int("batch_size", accrualBatchSize),
		)
		// запуск воркера
		accrualWorker.Run(gctx)
		log.Info("accrual worker stopped")
		return nil
	})

	g.Go(func() error {
		return srv.Run(gctx)
	})

	if err = g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}

	return nil
}

func buildHandlers(_ *zap.Logger, h *handler.Handler, tp myMiddleware.TokenParser) http.Handler {
	r := chi.NewRouter()
	r.Use(chiMiddleware.StripSlashes)

	// подключаем доку сваггера
	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	// открытые маршруты
	r.Post("/api/user/register", h.Register)
	r.Post("/api/user/login", h.Login)

	// закрытые маршруты
	r.Group(func(protectedRoutes chi.Router) {
		protectedRoutes.Use(myMiddleware.AuthMiddleware(tp))
		protectedRoutes.Post("/api/user/orders", h.UploadOrder)
		protectedRoutes.Get("/api/user/orders", h.GetOrders)
		protectedRoutes.Get("/api/user/balance", h.GetBalance)
		protectedRoutes.Post("/api/user/balance/withdraw", h.Withdraw)
		protectedRoutes.Get("/api/user/withdrawals", h.GetWithdrawals)
	})

	return myMiddleware.Conveyor(r, myMiddleware.Recoverer, myMiddleware.Gzip, myMiddleware.RequestLogger)
}
