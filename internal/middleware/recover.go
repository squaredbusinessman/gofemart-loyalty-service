package middleware

import (
	"net/http"

	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/logger"
	"go.uber.org/zap"
)

func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Log.Error("panic recovered",
					zap.String("method", request.Method),
					zap.String("path", request.URL.Path),
					zap.Any(" panic", rec),
				)
				http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(writer, request)
	})
}
