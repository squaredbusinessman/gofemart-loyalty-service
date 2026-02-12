package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/auth"
)

type stubTokenParser struct {
	parseTokenFn func(token string) (int64, error)
}

func (s stubTokenParser) ParseToken(token string) (int64, error) {
	return s.parseTokenFn(token)
}

func TestAuthMiddleware_StatusCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		authHeader string
		parser     stubTokenParser
		wantStatus int
	}{
		{
			name:       "401 without token",
			authHeader: "",
			parser: stubTokenParser{
				parseTokenFn: func(token string) (int64, error) {
					return 0, nil
				},
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "401 invalid token",
			authHeader: "Bearer bad-token",
			parser: stubTokenParser{
				parseTokenFn: func(token string) (int64, error) {
					return 0, auth.ErrInvalidToken
				},
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "401 expired token",
			authHeader: "Bearer expired-token",
			parser: stubTokenParser{
				parseTokenFn: func(token string) (int64, error) {
					return 0, auth.ErrExpiredToken
				},
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "500 parser internal error",
			authHeader: "Bearer any",
			parser: stubTokenParser{
				parseTokenFn: func(token string) (int64, error) {
					return 0, errors.New("parser failure")
				},
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "200 valid token",
			authHeader: "Bearer good-token",
			parser: stubTokenParser{
				parseTokenFn: func(token string) (int64, error) {
					return 42, nil
				},
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			next := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				userID, ok := UserIDFromContext(request.Context())
				if tt.wantStatus == http.StatusOK {
					require.True(t, ok)
					require.Equal(t, int64(42), userID)
				}
				writer.WriteHeader(http.StatusOK)
			})

			h := AuthMiddleware(tt.parser)(next)
			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			res := httptest.NewRecorder()

			h.ServeHTTP(res, req)

			require.Equal(t, tt.wantStatus, res.Code)
		})
	}
}
