package middleware

import (
	"bytes"
	"io"
	"net/http"
	"strings"
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

func HashMiddleware(key string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if key == "" {
				next.ServeHTTP(writer, request)
				return
			}

			got := request.Header.Get("HashSHA256")
			if got != "" {
				body, err := io.ReadAll(request.Body)
				if err != nil {
					http.Error(writer, "bad request", http.StatusBadRequest)
					return
				}
				_ = request.Body.Close()
				computed := sha256hex(body, key)
				if !strings.EqualFold(got, computed) {
					http.Error(writer, "bad hash", http.StatusBadRequest)
					return
				}
				request.Body = io.NopCloser(bytes.NewReader(body))
			}

			rec := NewRecorder(writer)
			next.ServeHTTP(rec, request)

			sum := sha256hex(rec.Body(), key)
			rec.Header().Set("HashSHA256", sum)
			rec.FlushTo(writer)
		})
	}
}
