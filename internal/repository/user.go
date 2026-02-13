package repository

import (
	"context"

	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/model"
)

type UserRepository interface {
	CreateUser(ctx context.Context, login string, passwordHash string) (int64, error)
	GetUserByLogin(ctx context.Context, login string) (model.User, error)
	CreateOrderIfNotExists(ctx context.Context, userID int64, number string) (created bool, ownerID int64, err error)
	ListOrdersByUser(ctx context.Context, userID int64) ([]model.Order, error)
}
