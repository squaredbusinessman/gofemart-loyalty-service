package service

import (
	"context"
	"errors"
	"strings"

	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/model"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/repository"
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

type BalanceRepository interface {
	GetBalance(ctx context.Context, userID int64) (model.BalanceResponse, error)
	Withdraw(ctx context.Context, userID int64, order string, sum float64) error
	GetWithdrawals(ctx context.Context, userID int64) ([]model.Withdrawal, error)
}

type balanceService struct {
	repo BalanceRepository
}

func (bs balanceService) GetBalance(ctx context.Context, userID int64) (model.BalanceResponse, error) {
	return bs.repo.GetBalance(ctx, userID)
}

func (bs balanceService) Withdraw(ctx context.Context, userID int64, order string, sum float64) error {
	order = strings.TrimSpace(order)
	if !isDigits(order) {
		return ErrInvalidWithdrawOrder
	}

	if !isValidLuhn(order) {
		return ErrInvalidOrderNumber
	}

	if sum <= 0 {
		return ErrInvalidWithdrawSum
	}

	if err := bs.repo.Withdraw(ctx, userID, order, sum); err != nil {
		if errors.Is(err, repository.ErrInsufficientFunds) {
			return ErrInsufficientFunds
		}
		return err
	}

	return nil
}

func (bs balanceService) GetWithdrawals(ctx context.Context, userID int64) ([]model.Withdrawal, error) {
	return bs.repo.GetWithdrawals(ctx, userID)
}

func NewBalanceService(repo BalanceRepository) BalanceService {
	if repo == nil {
		panic("nil balance repository")
	}
	return &balanceService{repo: repo}
}
