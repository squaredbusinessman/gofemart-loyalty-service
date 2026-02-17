package service

import (
	"context"
	"errors"

	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/model"
)

var (
	ErrInvalidWithdrawOrder = errors.New("invalid withdraw order")
	ErrInsufficientFunds    = errors.New("insufficient funds")
	ErrInvalidWithdrawSum   = errors.New("invalid withdraw sum")
)

type BalanceService interface {
	GetBalance(ctx context.Context, userID int64) (model.BalanceResponse, error)
	Withdraw(ctx context.Context, userID int64, order string, sum float64) error
	GetWithdrawals(ctx context.Context, userID int64) ([]model.Withdrawal, error)
}
