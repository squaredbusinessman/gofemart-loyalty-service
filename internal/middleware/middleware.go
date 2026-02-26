package middleware

import (
	"net/http"
	"time"

	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/logger"
	"go.uber.org/zap"
)

type Middleware func(handler http.Handler) http.Handler

func Conveyor(h http.Handler, middlewares ...Middleware) http.Handler {
	for _, middleware := range middlewares {
		h = middleware(h)
	}
	return h
}

func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		start := time.Now()
		lw := &logger.LoggingWriter{ResponseWriter: writer}

		next.ServeHTTP(lw, request)

		logger.Log.Info("HTTP request",
			zap.String("method", request.Method),
			zap.String("path", request.URL.Path),
			zap.Int("status", lw.Status),
			zap.Int("bytes", lw.Bytes),
			zap.Duration("latency", time.Since(start)),
		)
	})
}
