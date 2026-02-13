package service

import (
	"context"
	"errors"
	"strings"

	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/model"
)

type SubmitOrderResult int

const (
	SubmitOrderAccepted              SubmitOrderResult = iota // 202
	SubmitOrderAlreadyUploadedByUser                          // 200
)

var (
	ErrInvalidOrderNumber         = errors.New("invalid order number")           // 422
	ErrOrderUploadedByAnotherUser = errors.New("order uploaded by another user") // 409
)

type OrderRepository interface {
	CreateOrderIfNotExists(ctx context.Context, userID int64, number string) (created bool, ownerID int64, err error)
	ListOrdersByUser(ctx context.Context, userID int64) ([]model.Order, error)
}

type OrderService interface {
	SubmitOrder(ctx context.Context, userID int64, rawNumber string) (SubmitOrderResult, error)
	GetUserOrders(ctx context.Context, userID int64) ([]model.Order, error)
}

type orderService struct {
	repo OrderRepository
}

func NewOrderService(repo OrderRepository) OrderService {
	if repo == nil {
		panic("nil order repository")
	}
	return &orderService{
		repo: repo,
	}
}

func (s *orderService) SubmitOrder(ctx context.Context, userID int64, rawNumber string) (SubmitOrderResult, error) {
	number := strings.TrimSpace(rawNumber)
	if !isDigits(number) || !isValidLuhn(number) {
		return 0, ErrInvalidOrderNumber
	}

	created, ownerID, err := s.repo.CreateOrderIfNotExists(ctx, userID, number)
	if err != nil {
		return 0, err
	}

	if created {
		return SubmitOrderAccepted, nil
	}
	if ownerID == userID {
		return SubmitOrderAlreadyUploadedByUser, nil
	}

	return 0, ErrOrderUploadedByAnotherUser
}

func (s *orderService) GetUserOrders(ctx context.Context, userID int64) ([]model.Order, error) {
	return s.repo.ListOrdersByUser(ctx, userID)
}
