package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

type Config struct {
	Addr              string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
}

// Server это только жизненный цикл http сервера
type Server struct {
	log *zap.Logger
	srv *http.Server
	cfg Config
}

func New(cfg Config, h http.Handler, log *zap.Logger) (*Server, error) {
	if cfg.Addr == "" {
		return nil, fmt.Errorf("server addr is empty")
	}

	// вопрос к ревью - не нарушает ли данная конструкция YAGNI???
	if cfg.ShutdownTimeout == 0 {
		cfg.ShutdownTimeout = 10 * time.Second // если вдруг что-то пошло не так
	}

	return &Server{
		log: log,
		cfg: cfg,
		srv: &http.Server{
			Addr:              cfg.Addr,
			Handler:           h,
			ReadHeaderTimeout: cfg.ReadHeaderTimeout,
			ReadTimeout:       cfg.ReadTimeout,
			WriteTimeout:      cfg.WriteTimeout,
			IdleTimeout:       cfg.IdleTimeout,
		},
	}, nil
}

// Run живет пока не случился graceful shutdown или fatalErr
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)

	go func() {
		s.log.Info("http server starting", zap.String("addr", s.srv.Addr))
		errCh <- s.srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		s.log.Info("shutdown requested", zap.Error(ctx.Err()))

		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)
		defer cancel()

		// попытка корректно остановить сервер
		if err := s.srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}

		// ждём завершения ListenAndServe и проверяем, что там не "реальная" ошибка
		if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("listen and serve: %w", err)
		}

		s.log.Info("shutdown complete")
		return nil

	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("listen and serve: %w", err)
		}
		return nil
	}
}
