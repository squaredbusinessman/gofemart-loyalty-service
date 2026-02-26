package migrations

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func Up(pool *pgxpool.Pool, dir string) error {
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}

	return goose.Up(db, dir)
}
