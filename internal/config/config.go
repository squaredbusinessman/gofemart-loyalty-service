package config

import (
	"flag"
	"fmt"
	"github.com/ilyakaznacheev/cleanenv"
)

// Config рантайм конфиг для сервиса "Гофемарт"
//
// Конфигурирование сервиса накопительной системы лояльности:
// - RUN_ADDRESS или flag -a
// - DATABASE_URI или flag -d
// - ACCRUAL_SYSTEM_ADDRESS или flag -r
type Config struct {
	RunAddress           string `env:"RUN_ADDRESS" env-default:"localhost:8080"`
	DatabaseURI          string `env:"DATABASE_URI"`
	AccrualSystemAddress string `env:"ACCURAL_SYSTEM_ADDRESS"`
}

// Validate проверка конфигурации на старте запуска сервиса, чтобы отделить ошибки логики конфига от серверных
func (c Config) Validate() error {
	if c.RunAddress == "" {
		return fmt.Errorf("RUN_ADDRESS/-a is empty")
	}
	if c.DatabaseURI == "" {
		return fmt.Errorf("DATABASE_URI/-d is empty")
	}
	if c.AccrualSystemAddress == "" {
		return fmt.Errorf("ACCRUAL_SYSTEM_ADDRESS/-r is empty")
	}
	return nil
}

// Load читает env через cleanenv^ затем поверх применяет флаги.
// Приоритет работы: flags > env > default
func Load(args []string) (Config, error) {
	var cfg Config

	// смотрим env и env-default
	if err := cleanenv.ReadEnv(&cfg); err != nil {
		return Config{}, fmt.Errorf("read env: %w", err)
	}

	// flags поверх env
	fs := flag.NewFlagSet("gophermart", flag.ContinueOnError)

	fs.StringVar(
		&cfg.RunAddress, "a",
		cfg.RunAddress, "gophermart listen address (RUN_ADDRESS)")
	fs.StringVar(
		&cfg.DatabaseURI, "d",
		cfg.DatabaseURI, "postgres connection URI (DATABASE_URI)")
	fs.StringVar(
		&cfg.AccrualSystemAddress, "r",
		cfg.AccrualSystemAddress, "accrual system address (ACCRUAL_SYSTEM_ADDRESS)")

	if err := fs.Parse(args); err != nil {
		return Config{}, fmt.Errorf("parse flags: %w", err)
	}

	return cfg, nil
}
