package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/model"
)

type DBStorage struct {
	pool *pgxpool.Pool
}

func NewDBStorage(pool *pgxpool.Pool) *DBStorage {
	return &DBStorage{
		pool,
	}
}

var psql = squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)

func isRetryablePGErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return strings.HasPrefix(pgErr.Code, "08")
	}
	return false
}

func (s *DBStorage) CreateUser(ctx context.Context, login string, passwordHash string) (int64, error) {
	q, args, err := psql.
		Insert("users").
		Columns("login", "password_hash").
		Values(login, passwordHash).
		Suffix("RETURNING id").
		ToSql()

	if err != nil {
		return 0, fmt.Errorf("build create user query: %w", err)
	}

	var id int64

	err = s.pool.QueryRow(ctx, q, args...).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return 0, fmt.Errorf("create user: %w", ErrUserAlreadyExists)
		}
		return 0, fmt.Errorf("create user: %w", err)
	}
	return id, nil
}

func (s *DBStorage) GetUserByLogin(ctx context.Context, login string) (model.User, error) {
	q, args, err := psql.
		Select("id", "login", "password_hash").
		From("users").
		Where(squirrel.Eq{"login": login}).
		Limit(1).
		ToSql()
	if err != nil {
		return model.User{}, fmt.Errorf("build get user: %w", err)
	}

	var user model.User

	err = s.pool.QueryRow(ctx, q, args...).Scan(&user.ID, &user.Login, &user.PasswordHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.User{}, fmt.Errorf("get user by login: %w", ErrUserNotFound)
		}
		return model.User{}, fmt.Errorf("get user by login: %w", err)
	}
	return user, nil
}
