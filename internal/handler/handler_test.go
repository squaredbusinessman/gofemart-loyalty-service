package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/auth"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/middleware"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/model"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/repository"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/service"
)

type stubUserRepo struct {
	createUserFn    func(ctx context.Context, login, passwordHash string) (int64, error)
	getUserByLoginFn func(ctx context.Context, login string) (model.User, error)
	createOrderIfNotExistsFn func(ctx context.Context, userID int64, number string) (created bool, ownerID int64, err error)
}

func (s stubUserRepo) CreateUser(ctx context.Context, login, passwordHash string) (int64, error) {
	return s.createUserFn(ctx, login, passwordHash)
}

func (s stubUserRepo) GetUserByLogin(ctx context.Context, login string) (model.User, error) {
	return s.getUserByLoginFn(ctx, login)
}

func (s stubUserRepo) CreateOrderIfNotExists(ctx context.Context, userID int64, number string) (created bool, ownerID int64, err error) {
	if s.createOrderIfNotExistsFn == nil {
		return false, 0, errors.New("unexpected CreateOrderIfNotExists call")
	}
	return s.createOrderIfNotExistsFn(ctx, userID, number)
}

type stubTokenGenerator struct {
	generateTokenFn func(userID int64) (string, error)
}

func (s stubTokenGenerator) GenerateToken(userID int64) (string, error) {
	return s.generateTokenFn(userID)
}

func newNoopOrderService() stubOrderService {
	return stubOrderService{
		submitOrderFn: func(ctx context.Context, userID int64, rawNumber string) (service.SubmitOrderResult, error) {
			return service.SubmitOrderAccepted, nil
		},
	}
}

type stubOrderService struct {
	submitOrderFn func(ctx context.Context, userID int64, rawNumber string) (service.SubmitOrderResult, error)
}

func (s stubOrderService) SubmitOrder(ctx context.Context, userID int64, rawNumber string) (service.SubmitOrderResult, error) {
	return s.submitOrderFn(ctx, userID, rawNumber)
}

type stubTokenParser struct {
	parseTokenFn func(token string) (int64, error)
}

func (s stubTokenParser) ParseToken(token string) (int64, error) {
	return s.parseTokenFn(token)
}

func TestRegister_StatusCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		repo       stubUserRepo
		tokenGen   stubTokenGenerator
		wantStatus int
	}{
		{
			name: "400 bad json",
			body: "{",
			repo: stubUserRepo{
				createUserFn: func(ctx context.Context, login, passwordHash string) (int64, error) {
					return 1, nil
				},
				getUserByLoginFn: func(ctx context.Context, login string) (model.User, error) {
					return model.User{}, nil
				},
			},
			tokenGen: stubTokenGenerator{
				generateTokenFn: func(userID int64) (string, error) { return "token", nil },
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "409 login exists",
			body: `{"login":"user","password":"pass"}`,
			repo: stubUserRepo{
				createUserFn: func(ctx context.Context, login, passwordHash string) (int64, error) {
					return 0, repository.ErrUserAlreadyExists
				},
				getUserByLoginFn: func(ctx context.Context, login string) (model.User, error) {
					return model.User{}, nil
				},
			},
			tokenGen: stubTokenGenerator{
				generateTokenFn: func(userID int64) (string, error) { return "token", nil },
			},
			wantStatus: http.StatusConflict,
		},
		{
			name: "500 repo failure",
			body: `{"login":"user","password":"pass"}`,
			repo: stubUserRepo{
				createUserFn: func(ctx context.Context, login, passwordHash string) (int64, error) {
					return 0, errors.New("db down")
				},
				getUserByLoginFn: func(ctx context.Context, login string) (model.User, error) {
					return model.User{}, nil
				},
			},
			tokenGen: stubTokenGenerator{
				generateTokenFn: func(userID int64) (string, error) { return "token", nil },
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := NewHandler(tt.repo, tt.tokenGen, newNoopOrderService())
			req := httptest.NewRequest(http.MethodPost, "/api/user/register", strings.NewReader(tt.body))
			res := httptest.NewRecorder()

			h.Register(res, req)

			require.Equal(t, tt.wantStatus, res.Code)
		})
	}
}

func TestLogin_StatusCodes(t *testing.T) {
	t.Parallel()

	validHash, err := auth.HashPassword("correct-password")
	require.NoError(t, err)

	tests := []struct {
		name       string
		body       string
		repo       stubUserRepo
		tokenGen   stubTokenGenerator
		wantStatus int
	}{
		{
			name: "400 bad json",
			body: "{",
			repo: stubUserRepo{
				createUserFn: func(ctx context.Context, login, passwordHash string) (int64, error) {
					return 1, nil
				},
				getUserByLoginFn: func(ctx context.Context, login string) (model.User, error) {
					return model.User{}, nil
				},
			},
			tokenGen: stubTokenGenerator{
				generateTokenFn: func(userID int64) (string, error) { return "token", nil },
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "401 invalid credentials user not found",
			body: `{"login":"missing","password":"pass"}`,
			repo: stubUserRepo{
				createUserFn: func(ctx context.Context, login, passwordHash string) (int64, error) {
					return 1, nil
				},
				getUserByLoginFn: func(ctx context.Context, login string) (model.User, error) {
					return model.User{}, repository.ErrUserNotFound
				},
			},
			tokenGen: stubTokenGenerator{
				generateTokenFn: func(userID int64) (string, error) { return "token", nil },
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "500 repo failure",
			body: `{"login":"user","password":"pass"}`,
			repo: stubUserRepo{
				createUserFn: func(ctx context.Context, login, passwordHash string) (int64, error) {
					return 1, nil
				},
				getUserByLoginFn: func(ctx context.Context, login string) (model.User, error) {
					return model.User{}, errors.New("db down")
				},
			},
			tokenGen: stubTokenGenerator{
				generateTokenFn: func(userID int64) (string, error) { return "token", nil },
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "401 invalid credentials wrong password",
			body: `{"login":"user","password":"wrong-password"}`,
			repo: stubUserRepo{
				createUserFn: func(ctx context.Context, login, passwordHash string) (int64, error) {
					return 1, nil
				},
				getUserByLoginFn: func(ctx context.Context, login string) (model.User, error) {
					return model.User{ID: 10, Login: "user", PasswordHash: validHash}, nil
				},
			},
			tokenGen: stubTokenGenerator{
				generateTokenFn: func(userID int64) (string, error) { return "token", nil },
			},
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := NewHandler(tt.repo, tt.tokenGen, newNoopOrderService())
			req := httptest.NewRequest(http.MethodPost, "/api/user/login", strings.NewReader(tt.body))
			res := httptest.NewRecorder()

			h.Login(res, req)

			require.Equal(t, tt.wantStatus, res.Code)
		})
	}
}

func TestRegister_SetsAuthCookieOnSuccess(t *testing.T) {
	t.Parallel()

	h := NewHandler(
		stubUserRepo{
			createUserFn: func(ctx context.Context, login, passwordHash string) (int64, error) {
				return 7, nil
			},
			getUserByLoginFn: func(ctx context.Context, login string) (model.User, error) {
				return model.User{}, nil
			},
		},
		stubTokenGenerator{
			generateTokenFn: func(userID int64) (string, error) {
				require.Equal(t, int64(7), userID)
				return "signed-token", nil
			},
		},
		newNoopOrderService(),
	)

	req := httptest.NewRequest(http.MethodPost, "/api/user/register", strings.NewReader(`{"login":"user","password":"pass"}`))
	res := httptest.NewRecorder()

	h.Register(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	cookies := res.Result().Cookies()
	require.NotEmpty(t, cookies)
	require.Equal(t, authCookieName, cookies[0].Name)
	require.Equal(t, "signed-token", cookies[0].Value)
}

func TestLogin_SetsAuthCookieOnSuccess(t *testing.T) {
	t.Parallel()

	validHash, err := auth.HashPassword("correct-password")
	require.NoError(t, err)

	h := NewHandler(
		stubUserRepo{
			createUserFn: func(ctx context.Context, login, passwordHash string) (int64, error) {
				return 0, nil
			},
			getUserByLoginFn: func(ctx context.Context, login string) (model.User, error) {
				return model.User{ID: 21, Login: "user", PasswordHash: validHash}, nil
			},
		},
		stubTokenGenerator{
			generateTokenFn: func(userID int64) (string, error) {
				require.Equal(t, int64(21), userID)
				return "signed-token-login", nil
			},
		},
		newNoopOrderService(),
	)

	req := httptest.NewRequest(http.MethodPost, "/api/user/login", strings.NewReader(`{"login":"user","password":"correct-password"}`))
	res := httptest.NewRecorder()

	h.Login(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	cookies := res.Result().Cookies()
	require.NotEmpty(t, cookies)
	require.Equal(t, authCookieName, cookies[0].Name)
	require.Equal(t, "signed-token-login", cookies[0].Value)
}

func TestUploadOrder_StatusCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		method        string
		contentType   string
		body          string
		withAuth      bool
		service       stubOrderService
		wantStatus    int
		wantSvcCalled bool
		wantUserID    int64
		wantBody      string
	}{
		{
			name:        "405 method not allowed",
			method:      http.MethodGet,
			contentType: "text/plain",
			body:        "79927398713",
			withAuth:    true,
			service: stubOrderService{
				submitOrderFn: func(ctx context.Context, userID int64, rawNumber string) (service.SubmitOrderResult, error) {
					return service.SubmitOrderAccepted, nil
				},
			},
			wantStatus:    http.StatusMethodNotAllowed,
			wantSvcCalled: false,
		},
		{
			name:        "401 without auth context",
			method:      http.MethodPost,
			contentType: "text/plain",
			body:        "79927398713",
			withAuth:    false,
			service: stubOrderService{
				submitOrderFn: func(ctx context.Context, userID int64, rawNumber string) (service.SubmitOrderResult, error) {
					return service.SubmitOrderAccepted, nil
				},
			},
			wantStatus:    http.StatusUnauthorized,
			wantSvcCalled: false,
		},
		{
			name:        "400 unsupported content type",
			method:      http.MethodPost,
			contentType: "application/json",
			body:        "79927398713",
			withAuth:    true,
			service: stubOrderService{
				submitOrderFn: func(ctx context.Context, userID int64, rawNumber string) (service.SubmitOrderResult, error) {
					return service.SubmitOrderAccepted, nil
				},
			},
			wantStatus:    http.StatusBadRequest,
			wantSvcCalled: false,
		},
		{
			name:        "422 invalid order number",
			method:      http.MethodPost,
			contentType: "text/plain",
			body:        "invalid-order",
			withAuth:    true,
			service: stubOrderService{
				submitOrderFn: func(ctx context.Context, userID int64, rawNumber string) (service.SubmitOrderResult, error) {
					return 0, service.ErrInvalidOrderNumber
				},
			},
			wantStatus:    http.StatusUnprocessableEntity,
			wantSvcCalled: true,
			wantUserID:    42,
			wantBody:      "invalid-order",
		},
		{
			name:        "409 uploaded by another user",
			method:      http.MethodPost,
			contentType: "text/plain",
			body:        "79927398713",
			withAuth:    true,
			service: stubOrderService{
				submitOrderFn: func(ctx context.Context, userID int64, rawNumber string) (service.SubmitOrderResult, error) {
					return 0, service.ErrOrderUploadedByAnotherUser
				},
			},
			wantStatus:    http.StatusConflict,
			wantSvcCalled: true,
			wantUserID:    42,
			wantBody:      "79927398713",
		},
		{
			name:        "500 internal service error",
			method:      http.MethodPost,
			contentType: "text/plain",
			body:        "79927398713",
			withAuth:    true,
			service: stubOrderService{
				submitOrderFn: func(ctx context.Context, userID int64, rawNumber string) (service.SubmitOrderResult, error) {
					return 0, errors.New("service unavailable")
				},
			},
			wantStatus:    http.StatusInternalServerError,
			wantSvcCalled: true,
			wantUserID:    42,
			wantBody:      "79927398713",
		},
		{
			name:        "202 accepted",
			method:      http.MethodPost,
			contentType: "text/plain",
			body:        "79927398713",
			withAuth:    true,
			service: stubOrderService{
				submitOrderFn: func(ctx context.Context, userID int64, rawNumber string) (service.SubmitOrderResult, error) {
					return service.SubmitOrderAccepted, nil
				},
			},
			wantStatus:    http.StatusAccepted,
			wantSvcCalled: true,
			wantUserID:    42,
			wantBody:      "79927398713",
		},
		{
			name:        "200 already uploaded by same user",
			method:      http.MethodPost,
			contentType: "text/plain; charset=utf-8",
			body:        "79927398713",
			withAuth:    true,
			service: stubOrderService{
				submitOrderFn: func(ctx context.Context, userID int64, rawNumber string) (service.SubmitOrderResult, error) {
					return service.SubmitOrderAlreadyUploadedByUser, nil
				},
			},
			wantStatus:    http.StatusOK,
			wantSvcCalled: true,
			wantUserID:    42,
			wantBody:      "79927398713",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svcCalled := false
			var gotUserID int64
			var gotBody string

			h := &Handler{
				orderSvc: stubOrderService{
					submitOrderFn: func(ctx context.Context, userID int64, rawNumber string) (service.SubmitOrderResult, error) {
						svcCalled = true
						gotUserID = userID
						gotBody = rawNumber
						return tt.service.SubmitOrder(ctx, userID, rawNumber)
					},
				},
			}

			req := httptest.NewRequest(tt.method, "/api/user/orders", strings.NewReader(tt.body))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			res := httptest.NewRecorder()

			if tt.withAuth {
				req.AddCookie(&http.Cookie{Name: authCookieName, Value: "valid-token"})
				wrapped := middleware.AuthMiddleware(stubTokenParser{
					parseTokenFn: func(token string) (int64, error) {
						require.Equal(t, "valid-token", token)
						return 42, nil
					},
				})(http.HandlerFunc(h.UploadOrder))

				wrapped.ServeHTTP(res, req)
			} else {
				h.UploadOrder(res, req)
			}

			require.Equal(t, tt.wantStatus, res.Code)
			require.Equal(t, tt.wantSvcCalled, svcCalled)
			if tt.wantSvcCalled {
				require.Equal(t, tt.wantUserID, gotUserID)
				require.Equal(t, tt.wantBody, gotBody)
			}
		})
	}
}
