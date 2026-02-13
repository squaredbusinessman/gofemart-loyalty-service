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
	"github.com/jackc/pgx/v5/pgtype"
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

// CreateUser метод создания пользователя
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

// GetUserByLogin метод авторизации пользователя
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

// CreateOrderIfNotExists метод проверки уникальности заказа, возвращает владельца заказа
func (s *DBStorage) CreateOrderIfNotExists(ctx context.Context, userID int64, number string) (created bool, ownerID int64, err error) {
	q, args, err := psql.
		Insert("orders").
		Columns("number", "user_id", "status").
		Values(number, userID, "NEW").
		Suffix("ON CONFLICT (number) DO NOTHING").
		ToSql()
	if err != nil {
		return false, 0, fmt.Errorf("buid insert order query: %w", err)
	}

	tag, err := s.pool.Exec(ctx, q, args...)
	if err != nil {
		return false, 0, fmt.Errorf("insert order: %w", err)
	}

	if tag.RowsAffected() == 1 {
		return true, userID, nil
	}

	q, args, err = psql.
		Select("user_id").
		From("orders").
		Where(squirrel.Eq{"number": number}).
		Limit(1).
		ToSql()
	if err != nil {
		return false, 0, fmt.Errorf("build select owner query: %w", err)
	}

	if err = s.pool.QueryRow(ctx, q, args...).Scan(&ownerID); err != nil {
		return false, 0, fmt.Errorf("select order owner: %w", err)
	}

	return false, ownerID, nil
}

func (s *DBStorage) ListOrdersByUser(ctx context.Context, userID int64) ([]model.Order, error) {
	q, args, err := psql.
		Select("number", "status", "accrual", "uploaded_at").
		From("orders").
		Where(squirrel.Eq{"user_id": userID}).
		OrderBy("uploaded_at DESC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build list user orders query: %w", err)
	}

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list user orders: %w", err)
	}
	defer rows.Close()

	orders := make([]model.Order, 0)

	for rows.Next() {
		var o model.Order
		var accrual pgtype.Numeric
		if err = rows.Scan(&o.Number, &o.Status, &accrual, &o.UploadedAt); err != nil {
			return nil, fmt.Errorf("scan user order: %w", err)
		}

		if accrual.Valid {
			f, convErr := accrual.Float64Value()
			if convErr != nil {
				return nil, fmt.Errorf("convert accrual: %w", convErr)
			}
			o.Accrual = &f.Float64
		}

		orders = append(orders, o)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user orders: %w", err)
	}

	return orders, nil
}
