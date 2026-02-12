package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/auth"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/model"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/repository"
)

type TokenGenerator interface {
	GenerateToken(userID int64) (string, error)
}

const authCookieName = "auth_token"

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
	ur repository.UserRepository
	tg TokenGenerator
}

func NewHandler(userRepo repository.UserRepository, tokenGen TokenGenerator) *Handler {
	if userRepo == nil {
		panic("nil user repository")
	}
	if tokenGen == nil {
		panic("nil token generator")
	}
	return &Handler{
		ur: userRepo,
		tg: tokenGen,
	}
}

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
