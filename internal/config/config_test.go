package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoad_ReadsEnv(t *testing.T) {
	// t.Setenv меняет env только на время теста.
	t.Setenv("RUN_ADDRESS", "env-run:8080")
	t.Setenv("DATABASE_URI", "env-db")
	t.Setenv("ACCRUAL_SYSTEM_ADDRESS", "env-accrual:8081")

	cfg, err := Load(nil)
	require.NoError(t, err)

	require.Equal(t, "env-run:8080", cfg.RunAddress)
	require.Equal(t, "env-db", cfg.DatabaseURI)
	require.Equal(t, "env-accrual:8081", cfg.AccrualSystemAddress)
}

func TestLoad_PriorityFlagsOverEnv_AllKeys(t *testing.T) {
	t.Setenv("RUN_ADDRESS", "env-run:1")
	t.Setenv("DATABASE_URI", "env-db:1")
	t.Setenv("ACCRUAL_SYSTEM_ADDRESS", "env-accrual:1")

	cfg, err := Load([]string{
		"-a", "flag-run:2",
		"-d", "flag-db:2",
		"-r", "flag-accrual:2",
	})
	require.NoError(t, err)

	// Проверяем, что каждый флаг переопределил env
	require.Equal(t, "flag-run:2", cfg.RunAddress)
	require.Equal(t, "flag-db:2", cfg.DatabaseURI)
	require.Equal(t, "flag-accrual:2", cfg.AccrualSystemAddress)
}

func TestConfigValidate_RequiresDBAndAccrual(t *testing.T) {
	// Явно проверяем обязательные поля
	// RunAddress дефолт, не проверяем.
	cfg := Config{
		RunAddress:           "localhost:8080",
		DatabaseURI:          "",
		AccrualSystemAddress: "",
	}

	err := cfg.Validate()
	require.Error(t, err)
}
