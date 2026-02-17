package handler

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/auth"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/middleware"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/model"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/repository"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/service"
)

const (
	authCookieName       = "auth_token"
	contentTypeTextPlain = "text/plain"
	contentAppJSON       = "application/json"
)

func setAuthCookie(writer http.ResponseWriter, request *http.Request, token string) {
	http.SetCookie(writer, &http.Cookie{
		Name:     authCookieName,
		Value:    token,
		Path:     "/",
		Secure:   request.TLS != nil,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

type Handler struct {
	ur         repository.UserRepository
	tg         auth.TokenGenerator
	orderSvc   service.OrderService
	balanceSvc service.BalanceService
}

func NewHandler(userRepo repository.UserRepository, tokenGen auth.TokenGenerator, orderService service.OrderService, balanceService service.BalanceService) *Handler {
	if userRepo == nil {
		panic("nil user repository")
	}
	if tokenGen == nil {
		panic("nil token generator")
	}
	return &Handler{
		ur:         userRepo,
		tg:         tokenGen,
		orderSvc:   orderService,
		balanceSvc: balanceService,
	}
}

// Register godoc
// @Summary Регистрация пользователя
// @Tags user
// @Accept json
// @Produce plain
// @Param request body model.RegisterUser true "login/password"
// @Success 200 {string} string "ok"
// @Failure 400 {string} string "bad request"
// @Failure 409 {string} string "conflict"
// @Failure 500 {string} string "internal error"
// @Router /api/user/register [post]
func (h *Handler) Register(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(writer, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	var req model.RegisterUser
	if err := json.NewDecoder(request.Body).Decode(&req); err != nil {
		http.Error(writer, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	req.Login = strings.TrimSpace(req.Login)
	if req.Login == "" || req.Password == "" {
		http.Error(writer, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	userID, err := h.ur.CreateUser(request.Context(), req.Login, passwordHash)
	if err != nil {
		if errors.Is(err, repository.ErrUserAlreadyExists) {
			http.Error(writer, "login already exists", http.StatusConflict)
			return
		}
		http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	token, err := h.tg.GenerateToken(userID)
	if err != nil {
		http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	setAuthCookie(writer, request, token)
	writer.WriteHeader(http.StatusOK)
}

// Login godoc
// @Summary Аутентификация пользователя
// @Tags user
// @Accept json
// @Produce plain
// @Param request body model.LoginRequest true "login/password"
// @Success 200 {string} string "ok"
// @Failure 400 {string} string "bad request"
// @Failure 401 {string} string "unauthorized"
// @Failure 500 {string} string "internal error"
// @Router /api/user/login [post]
func (h *Handler) Login(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(writer, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	var req model.LoginRequest
	if err := json.NewDecoder(request.Body).Decode(&req); err != nil {
		http.Error(writer, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	req.Login = strings.TrimSpace(req.Login)
	if req.Login == "" || req.Password == "" {
		http.Error(writer, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	user, err := h.ur.GetUserByLogin(request.Context(), req.Login)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			http.Error(writer, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	err = auth.CheckPassword(user.PasswordHash, req.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			http.Error(writer, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	token, err := h.tg.GenerateToken(user.ID)
	if err != nil {
		http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	setAuthCookie(writer, request, token)
	writer.WriteHeader(http.StatusOK)
}

// UploadOrder godoc
// @Summary Загрузка номера заказа
// @Tags orders
// @Accept plain
// @Produce plain
// @Security ApiKeyAuth
// @Param order body string true "Номер заказа (text/plain)"
// @Success 202 {string} string "accepted"
// @Success 200 {string} string "already uploaded by same user"
// @Failure 400 {string} string "bad request"
// @Failure 401 {string} string "unauthorized"
// @Failure 409 {string} string "conflict"
// @Failure 422 {string} string "unprocessable entity"
// @Failure 500 {string} string "internal error"
// @Router /api/user/orders [post]
func (h *Handler) UploadOrder(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(writer, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	userID, ok := middleware.UserIDFromContext(request.Context())
	if !ok {
		http.Error(writer, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	// проверка Content-Type, чтобы не ломаться на "text/plain; charset=utf-8"
	ct := request.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(ct)
	if err != nil || mediaType != contentTypeTextPlain {
		http.Error(writer, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	bodyBytes, err := io.ReadAll(request.Body)
	if err != nil {
		http.Error(writer, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	result, err := h.orderSvc.SubmitOrder(request.Context(), userID, string(bodyBytes))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrOrderNumberFormat):
			http.Error(writer, http.StatusText(http.StatusBadRequest), http.StatusBadRequest) // 400
		case errors.Is(err, service.ErrInvalidOrderNumber):
			http.Error(writer, http.StatusText(http.StatusUnprocessableEntity), http.StatusUnprocessableEntity) // 422
		case errors.Is(err, service.ErrOrderUploadedByAnotherUser):
			http.Error(writer, http.StatusText(http.StatusConflict), http.StatusConflict) // 409
		default:
			http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError) // 500
		}
		return
	}

	switch result {
	case service.SubmitOrderAccepted:
		writer.WriteHeader(http.StatusAccepted) // 202
	case service.SubmitOrderAlreadyUploadedByUser:
		writer.WriteHeader(http.StatusOK) // 200
	default:
		http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

// GetOrders godoc
// @Summary Список заказов пользователя
// @Tags orders
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {array} model.Order
// @Failure 204 {string} string "no content"
// @Failure 401 {string} string "unauthorized"
// @Failure 500 {string} string "internal error"
// @Router /api/user/orders [get]
func (h *Handler) GetOrders(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		http.Error(writer, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	userID, ok := middleware.UserIDFromContext(request.Context())
	if !ok {
		http.Error(writer, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized) // 401
		return
	}

	orders, err := h.orderSvc.GetUserOrders(request.Context(), userID)
	if err != nil {
		http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError) // 500
		return
	}

	if len(orders) == 0 {
		writer.WriteHeader(http.StatusNoContent) // 204
		return
	}

	writer.Header().Set("Content-Type", contentAppJSON)
	if err = json.NewEncoder(writer).Encode(orders); err != nil {
		http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
}

// GetBalance godoc
// @Summary Баланс пользователя
// @Tags balance
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} model.BalanceResponse
// @Failure 401 {string} string "unauthorized"
// @Failure 500 {string} string "internal error"
// @Router /api/user/balance [get]
func (h *Handler) GetBalance(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		http.Error(writer, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	userID, ok := middleware.UserIDFromContext(request.Context())
	if !ok {
		http.Error(writer, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized) // 401
		return
	}

	balance, err := h.balanceSvc.GetBalance(request.Context(), userID)
	if err != nil {
		http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	writer.Header().Set("Content-Type", contentAppJSON)
	if err = json.NewEncoder(writer).Encode(balance); err != nil {
		http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
}

// Withdraw godoc
// @Summary Списание баллов
// @Tags balance
// @Accept json
// @Produce plain
// @Security ApiKeyAuth
// @Param request body model.WithdrawRequest true "order/sum"
// @Success 200 {string} string "ok"
// @Failure 400 {string} string "bad request"
// @Failure 401 {string} string "unauthorized"
// @Failure 402 {string} string "payment required"
// @Failure 422 {string} string "unprocessable entity"
// @Failure 500 {string} string "internal error"
// @Router /api/user/balance/withdraw [post]
func (h *Handler) Withdraw(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(writer, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	userID, ok := middleware.UserIDFromContext(request.Context())
	if !ok {
		http.Error(writer, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	var req model.WithdrawRequest
	if err := json.NewDecoder(request.Body).Decode(&req); err != nil {
		http.Error(writer, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	req.Order = strings.TrimSpace(req.Order)
	if req.Order == "" || req.Sum <= 0 {
		http.Error(writer, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	err := h.balanceSvc.Withdraw(request.Context(), userID, req.Order, req.Sum)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidOrderNumber):
			http.Error(writer, http.StatusText(http.StatusUnprocessableEntity), http.StatusUnprocessableEntity)
		case errors.Is(err, service.ErrInsufficientFunds):
			http.Error(writer, http.StatusText(http.StatusPaymentRequired), http.StatusPaymentRequired)
		default:
			http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
		return
	}

	writer.WriteHeader(http.StatusOK)
}

// GetWithdrawals godoc
// @Summary История списаний пользователя
// @Tags balance
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {array} model.Withdrawal
// @Failure 204 {string} string "no content"
// @Failure 401 {string} string "unauthorized"
// @Failure 500 {string} string "internal error"
// @Router /api/user/withdrawals [get]
func (h *Handler) GetWithdrawals(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		http.Error(writer, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	userID, ok := middleware.UserIDFromContext(request.Context())
	if !ok {
		http.Error(writer, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized) // 401
		return
	}

	withdrawals, err := h.balanceSvc.GetWithdrawals(request.Context(), userID)
	if err != nil {
		http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError) // 500
		return
	}

	if len(withdrawals) == 0 {
		writer.WriteHeader(http.StatusNoContent) // 204
		return
	}

	writer.Header().Set("Content-Type", contentAppJSON)
	if err = json.NewEncoder(writer).Encode(withdrawals); err != nil {
		http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
}
