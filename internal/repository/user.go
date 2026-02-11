package repository

import (
	"context"

	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/model"
)

type UserRepository interface {
	CreateUser(ctx context.Context, login string, passwordHash string) (int64, error)
	GetUserByLogin(ctx context.Context, login string) (model.User, error)
}
