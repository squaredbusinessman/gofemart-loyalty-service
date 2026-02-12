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
		cookie     *http.Cookie
		parser     stubTokenParser
		wantStatus int
	}{
		{
			name:       "401 without token",
			cookie:     nil,
			parser: stubTokenParser{
				parseTokenFn: func(token string) (int64, error) {
					return 0, nil
				},
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "401 invalid token",
			cookie:     &http.Cookie{Name: "auth_token", Value: "bad-token"},
			parser: stubTokenParser{
				parseTokenFn: func(token string) (int64, error) {
					return 0, auth.ErrInvalidToken
				},
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "401 expired token",
			cookie:     &http.Cookie{Name: "auth_token", Value: "expired-token"},
			parser: stubTokenParser{
				parseTokenFn: func(token string) (int64, error) {
					return 0, auth.ErrExpiredToken
				},
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "500 parser internal error",
			cookie:     &http.Cookie{Name: "auth_token", Value: "any"},
			parser: stubTokenParser{
				parseTokenFn: func(token string) (int64, error) {
					return 0, errors.New("parser failure")
				},
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "200 valid token",
			cookie:     &http.Cookie{Name: "auth_token", Value: "good-token"},
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
			if tt.cookie != nil {
				req.AddCookie(tt.cookie)
			}
			res := httptest.NewRecorder()

			h.ServeHTTP(res, req)

			require.Equal(t, tt.wantStatus, res.Code)
		})
	}
}
