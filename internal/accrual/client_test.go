package accrual

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseRetryAfter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		header  string
		want    time.Duration
		wantErr bool
	}{
		{
			name:   "valid seconds",
			header: "60",
			want:   60 * time.Second,
		},
		{
			name:   "valid seconds with spaces",
			header: " 5 ",
			want:   5 * time.Second,
		},
		{
			name:    "invalid not a number",
			header:  "abc",
			wantErr: true,
		},
		{
			name:    "invalid zero",
			header:  "0",
			wantErr: true,
		},
		{
			name:    "invalid negative",
			header:  "-10",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseRetryAfter(tt.header)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestHTTPClient_GetOrder_200_204_429(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		statusCode      int
		body            string
		retryAfter      string
		wantKind        ResultKind
		wantAccrual     *float64
		wantRetryAfter  time.Duration
		wantErrContains string
	}{
		{
			name:       "200 processed with accrual",
			statusCode: http.StatusOK,
			body:       `{"order":"123","status":"PROCESSED","accrual":500}`,
			wantKind:   ResultProcessed,
			wantAccrual: func() *float64 {
				v := 500.0
				return &v
			}(),
		},
		{
			name:       "200 processing from registered",
			statusCode: http.StatusOK,
			body:       `{"order":"123","status":"REGISTERED"}`,
			wantKind:   ResultProcessing,
		},
		{
			name:       "204 not registered",
			statusCode: http.StatusNoContent,
			wantKind:   ResultNotRegistered,
		},
		{
			name:           "429 rate limited",
			statusCode:     http.StatusTooManyRequests,
			retryAfter:     "3",
			wantKind:       ResultRateLimited,
			wantRetryAfter: 3 * time.Second,
		},
		{
			name:            "429 invalid retry after",
			statusCode:      http.StatusTooManyRequests,
			retryAfter:      "oops",
			wantErrContains: "invalid retry after",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodGet, r.Method)
				require.Contains(t, r.URL.Path, "/api/orders/")

				if tt.retryAfter != "" {
					w.Header().Set("Retry-After", tt.retryAfter)
				}
				w.WriteHeader(tt.statusCode)
				if tt.body != "" {
					_, _ = w.Write([]byte(tt.body))
				}
			}))
			defer srv.Close()

			cli, err := NewClient(srv.URL, 2*time.Second, 0)
			require.NoError(t, err)

			got, err := cli.GetOrder(context.Background(), "123456")
			if tt.wantErrContains != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErrContains)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantKind, got.Kind)
			require.Equal(t, tt.wantRetryAfter, got.RetryAfter)

			if tt.wantAccrual == nil {
				require.Nil(t, got.Accrual)
			} else {
				require.NotNil(t, got.Accrual)
				require.InDelta(t, *tt.wantAccrual, *got.Accrual, 0.000001)
			}
		})
	}
}

