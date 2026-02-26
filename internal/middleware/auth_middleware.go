package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/auth"
)

type ctxKey string

const userIDKey ctxKey = "user_id"

type TokenParser interface {
	ParseToken(token string) (int64, error)
}

func AuthMiddleware(tm TokenParser) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			cookie, err := request.Cookie("auth_token")
			if err != nil || strings.TrimSpace(cookie.Value) == "" {
				http.Error(writer, "unauthorized", http.StatusUnauthorized)
				return
			}

			userID, err := tm.ParseToken(cookie.Value)
			if err != nil && (errors.Is(err, auth.ErrInvalidToken) || errors.Is(err, auth.ErrExpiredToken)) {
				http.Error(writer, "unauthorized", http.StatusUnauthorized)
				return
			}
			if err != nil {
				http.Error(writer, "internal server error", http.StatusInternalServerError)
				return
			}

			ctx := context.WithValue(request.Context(), userIDKey, userID)
			next.ServeHTTP(writer, request.WithContext(ctx))
		})
	}
}

func UserIDFromContext(ctx context.Context) (int64, bool) {
	v, ok := ctx.Value(userIDKey).(int64)
	return v, ok
}
