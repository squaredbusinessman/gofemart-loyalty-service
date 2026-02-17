package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/middleware"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/model"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/service"
)

type stubBalanceService struct {
	getBalanceFn     func(ctx context.Context, userID int64) (model.BalanceResponse, error)
	withdrawFn       func(ctx context.Context, userID int64, order string, sum float64) error
	getWithdrawalsFn func(ctx context.Context, userID int64) ([]model.Withdrawal, error)
}

func newNoopBalanceService() stubBalanceService {
	return stubBalanceService{
		getBalanceFn: func(ctx context.Context, userID int64) (model.BalanceResponse, error) {
			return model.BalanceResponse{}, nil
		},
		withdrawFn: func(ctx context.Context, userID int64, order string, sum float64) error {
			return nil
		},
		getWithdrawalsFn: func(ctx context.Context, userID int64) ([]model.Withdrawal, error) {
			return nil, nil
		},
	}
}

func (s stubBalanceService) GetBalance(ctx context.Context, userID int64) (model.BalanceResponse, error) {
	if s.getBalanceFn == nil {
		return model.BalanceResponse{}, errors.New("unexpected GetBalance call")
	}
	return s.getBalanceFn(ctx, userID)
}

func (s stubBalanceService) Withdraw(ctx context.Context, userID int64, order string, sum float64) error {
	if s.withdrawFn == nil {
		return errors.New("unexpected Withdraw call")
	}
	return s.withdrawFn(ctx, userID, order, sum)
}

func (s stubBalanceService) GetWithdrawals(ctx context.Context, userID int64) ([]model.Withdrawal, error) {
	if s.getWithdrawalsFn == nil {
		return nil, errors.New("unexpected GetWithdrawals call")
	}
	return s.getWithdrawalsFn(ctx, userID)
}

func TestGetBalance_StatusCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		method        string
		withAuth      bool
		service       stubBalanceService
		wantStatus    int
		wantSvcCalled bool
		wantUserID    int64
		wantBody      model.BalanceResponse
	}{
		{
			name:     "405 method not allowed",
			method:   http.MethodPost,
			withAuth: true,
			service: stubBalanceService{
				getBalanceFn: func(ctx context.Context, userID int64) (model.BalanceResponse, error) {
					return model.BalanceResponse{}, nil
				},
			},
			wantStatus:    http.StatusMethodNotAllowed,
			wantSvcCalled: false,
		},
		{
			name:     "401 without auth context",
			method:   http.MethodGet,
			withAuth: false,
			service: stubBalanceService{
				getBalanceFn: func(ctx context.Context, userID int64) (model.BalanceResponse, error) {
					return model.BalanceResponse{}, nil
				},
			},
			wantStatus:    http.StatusUnauthorized,
			wantSvcCalled: false,
		},
		{
			name:     "500 service error",
			method:   http.MethodGet,
			withAuth: true,
			service: stubBalanceService{
				getBalanceFn: func(ctx context.Context, userID int64) (model.BalanceResponse, error) {
					return model.BalanceResponse{}, errors.New("db down")
				},
			},
			wantStatus:    http.StatusInternalServerError,
			wantSvcCalled: true,
			wantUserID:    42,
		},
		{
			name:     "200 json balance",
			method:   http.MethodGet,
			withAuth: true,
			service: stubBalanceService{
				getBalanceFn: func(ctx context.Context, userID int64) (model.BalanceResponse, error) {
					return model.BalanceResponse{
						Current:   115.37,
						Withdrawn: 9.63,
					}, nil
				},
			},
			wantStatus:    http.StatusOK,
			wantSvcCalled: true,
			wantUserID:    42,
			wantBody: model.BalanceResponse{
				Current:   115.37,
				Withdrawn: 9.63,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svcCalled := false
			var gotUserID int64

			h := &Handler{
				balanceSvc: stubBalanceService{
					getBalanceFn: func(ctx context.Context, userID int64) (model.BalanceResponse, error) {
						svcCalled = true
						gotUserID = userID
						return tt.service.GetBalance(ctx, userID)
					},
				},
			}

			req := httptest.NewRequest(tt.method, "/api/user/balance", nil)
			res := httptest.NewRecorder()

			if tt.withAuth {
				req.AddCookie(&http.Cookie{Name: authCookieName, Value: "valid-token"})
				wrapped := middleware.AuthMiddleware(stubTokenParser{
					parseTokenFn: func(token string) (int64, error) {
						require.Equal(t, "valid-token", token)
						return 42, nil
					},
				})(http.HandlerFunc(h.GetBalance))
				wrapped.ServeHTTP(res, req)
			} else {
				h.GetBalance(res, req)
			}

			require.Equal(t, tt.wantStatus, res.Code)
			require.Equal(t, tt.wantSvcCalled, svcCalled)
			if tt.wantSvcCalled {
				require.Equal(t, tt.wantUserID, gotUserID)
			}

			if tt.wantStatus == http.StatusOK {
				require.Equal(t, contentAppJSON, res.Header().Get("Content-Type"))
				var got model.BalanceResponse
				err := json.Unmarshal(res.Body.Bytes(), &got)
				require.NoError(t, err)
				require.InDelta(t, tt.wantBody.Current, got.Current, 0.000001)
				require.InDelta(t, tt.wantBody.Withdrawn, got.Withdrawn, 0.000001)
			}
		})
	}
}

func TestWithdraw_StatusCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		method        string
		withAuth      bool
		body          string
		withdrawErr   error
		wantStatus    int
		wantSvcCalled bool
		wantUserID    int64
		wantOrder     string
		wantSum       float64
	}{
		{
			name:          "405 method not allowed",
			method:        http.MethodGet,
			withAuth:      true,
			body:          `{"order":"2377225624","sum":10}`,
			wantStatus:    http.StatusMethodNotAllowed,
			wantSvcCalled: false,
		},
		{
			name:          "401 without auth context",
			method:        http.MethodPost,
			withAuth:      false,
			body:          `{"order":"2377225624","sum":10}`,
			wantStatus:    http.StatusUnauthorized,
			wantSvcCalled: false,
		},
		{
			name:          "400 invalid json",
			method:        http.MethodPost,
			withAuth:      true,
			body:          `{`,
			wantStatus:    http.StatusBadRequest,
			wantSvcCalled: false,
		},
		{
			name:          "400 invalid fields",
			method:        http.MethodPost,
			withAuth:      true,
			body:          `{"order":"","sum":0}`,
			wantStatus:    http.StatusBadRequest,
			wantSvcCalled: false,
		},
		{
			name:          "422 invalid order number",
			method:        http.MethodPost,
			withAuth:      true,
			body:          `{"order":"12345678901","sum":10}`,
			withdrawErr:   service.ErrInvalidOrderNumber,
			wantStatus:    http.StatusUnprocessableEntity,
			wantSvcCalled: true,
			wantUserID:    42,
			wantOrder:     "12345678901",
			wantSum:       10,
		},
		{
			name:          "402 insufficient funds",
			method:        http.MethodPost,
			withAuth:      true,
			body:          `{"order":"2377225624","sum":1000}`,
			withdrawErr:   service.ErrInsufficientFunds,
			wantStatus:    http.StatusPaymentRequired,
			wantSvcCalled: true,
			wantUserID:    42,
			wantOrder:     "2377225624",
			wantSum:       1000,
		},
		{
			name:          "500 service error",
			method:        http.MethodPost,
			withAuth:      true,
			body:          `{"order":"2377225624","sum":10}`,
			withdrawErr:   errors.New("db down"),
			wantStatus:    http.StatusInternalServerError,
			wantSvcCalled: true,
			wantUserID:    42,
			wantOrder:     "2377225624",
			wantSum:       10,
		},
		{
			name:          "200 success",
			method:        http.MethodPost,
			withAuth:      true,
			body:          `{"order":"2377225624","sum":10.25}`,
			wantStatus:    http.StatusOK,
			wantSvcCalled: true,
			wantUserID:    42,
			wantOrder:     "2377225624",
			wantSum:       10.25,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svcCalled := false
			var gotUserID int64
			var gotOrder string
			var gotSum float64

			h := &Handler{
				balanceSvc: stubBalanceService{
					withdrawFn: func(ctx context.Context, userID int64, order string, sum float64) error {
						svcCalled = true
						gotUserID = userID
						gotOrder = order
						gotSum = sum
						return tt.withdrawErr
					},
				},
			}

			req := httptest.NewRequest(tt.method, "/api/user/balance/withdraw", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", contentAppJSON)
			res := httptest.NewRecorder()

			if tt.withAuth {
				req.AddCookie(&http.Cookie{Name: authCookieName, Value: "valid-token"})
				wrapped := middleware.AuthMiddleware(stubTokenParser{
					parseTokenFn: func(token string) (int64, error) {
						require.Equal(t, "valid-token", token)
						return 42, nil
					},
				})(http.HandlerFunc(h.Withdraw))
				wrapped.ServeHTTP(res, req)
			} else {
				h.Withdraw(res, req)
			}

			require.Equal(t, tt.wantStatus, res.Code)
			require.Equal(t, tt.wantSvcCalled, svcCalled)
			if tt.wantSvcCalled {
				require.Equal(t, tt.wantUserID, gotUserID)
				require.Equal(t, tt.wantOrder, gotOrder)
				require.InDelta(t, tt.wantSum, gotSum, 0.000001)
			}
		})
	}
}

func TestGetWithdrawals_StatusCodes(t *testing.T) {
	t.Parallel()

	firstProcessedAt := time.Date(2026, 2, 16, 9, 0, 0, 0, time.UTC)
	secondProcessedAt := time.Date(2026, 2, 16, 8, 45, 0, 0, time.UTC)

	tests := []struct {
		name          string
		method        string
		withAuth      bool
		service       stubBalanceService
		wantStatus    int
		wantSvcCalled bool
		wantUserID    int64
		wantBody      string
		wantItems     []model.Withdrawal
	}{
		{
			name:     "405 method not allowed",
			method:   http.MethodPost,
			withAuth: true,
			service: stubBalanceService{
				getWithdrawalsFn: func(ctx context.Context, userID int64) ([]model.Withdrawal, error) {
					return nil, nil
				},
			},
			wantStatus:    http.StatusMethodNotAllowed,
			wantSvcCalled: false,
		},
		{
			name:     "401 without auth context",
			method:   http.MethodGet,
			withAuth: false,
			service: stubBalanceService{
				getWithdrawalsFn: func(ctx context.Context, userID int64) ([]model.Withdrawal, error) {
					return nil, nil
				},
			},
			wantStatus:    http.StatusUnauthorized,
			wantSvcCalled: false,
		},
		{
			name:     "500 service error",
			method:   http.MethodGet,
			withAuth: true,
			service: stubBalanceService{
				getWithdrawalsFn: func(ctx context.Context, userID int64) ([]model.Withdrawal, error) {
					return nil, errors.New("db down")
				},
			},
			wantStatus:    http.StatusInternalServerError,
			wantSvcCalled: true,
			wantUserID:    42,
			wantBody:      "Internal Server Error\n",
		},
		{
			name:     "204 when empty history",
			method:   http.MethodGet,
			withAuth: true,
			service: stubBalanceService{
				getWithdrawalsFn: func(ctx context.Context, userID int64) ([]model.Withdrawal, error) {
					return []model.Withdrawal{}, nil
				},
			},
			wantStatus:    http.StatusNoContent,
			wantSvcCalled: true,
			wantUserID:    42,
		},
		{
			name:     "200 json withdrawals",
			method:   http.MethodGet,
			withAuth: true,
			service: stubBalanceService{
				getWithdrawalsFn: func(ctx context.Context, userID int64) ([]model.Withdrawal, error) {
					return []model.Withdrawal{
						{Order: "7000000003", Sum: 499.99, ProcessedAt: firstProcessedAt},
						{Order: "7000000002", Sum: 1500.00, ProcessedAt: secondProcessedAt},
					}, nil
				},
			},
			wantStatus:    http.StatusOK,
			wantSvcCalled: true,
			wantUserID:    42,
			wantItems: []model.Withdrawal{
				{Order: "7000000003", Sum: 499.99, ProcessedAt: firstProcessedAt},
				{Order: "7000000002", Sum: 1500.00, ProcessedAt: secondProcessedAt},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svcCalled := false
			var gotUserID int64

			h := &Handler{
				balanceSvc: stubBalanceService{
					getWithdrawalsFn: func(ctx context.Context, userID int64) ([]model.Withdrawal, error) {
						svcCalled = true
						gotUserID = userID
						return tt.service.GetWithdrawals(ctx, userID)
					},
				},
			}

			req := httptest.NewRequest(tt.method, "/api/user/withdrawals", nil)
			res := httptest.NewRecorder()

			if tt.withAuth {
				req.AddCookie(&http.Cookie{Name: authCookieName, Value: "valid-token"})
				wrapped := middleware.AuthMiddleware(stubTokenParser{
					parseTokenFn: func(token string) (int64, error) {
						require.Equal(t, "valid-token", token)
						return 42, nil
					},
				})(http.HandlerFunc(h.GetWithdrawals))
				wrapped.ServeHTTP(res, req)
			} else {
				h.GetWithdrawals(res, req)
			}

			require.Equal(t, tt.wantStatus, res.Code)
			require.Equal(t, tt.wantSvcCalled, svcCalled)
			if tt.wantSvcCalled {
				require.Equal(t, tt.wantUserID, gotUserID)
			}

			if tt.wantBody != "" {
				require.Equal(t, tt.wantBody, res.Body.String())
			}

			if tt.wantItems != nil {
				require.Equal(t, contentAppJSON, res.Header().Get("Content-Type"))
				var got []model.Withdrawal
				err := json.Unmarshal(res.Body.Bytes(), &got)
				require.NoError(t, err)
				require.Len(t, got, len(tt.wantItems))
				for i := range tt.wantItems {
					require.Equal(t, tt.wantItems[i].Order, got[i].Order)
					require.InDelta(t, tt.wantItems[i].Sum, got[i].Sum, 0.000001)
					require.True(t, tt.wantItems[i].ProcessedAt.Equal(got[i].ProcessedAt))
				}
			}
		})
	}
}
