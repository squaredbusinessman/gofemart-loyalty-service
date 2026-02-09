package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoad_PriorityFlagsOverEnv(t *testing.T) {
	t.Setenv("RUN_ADDRESS", "env:1")
	cfg, err := Load([]string{"-a", "flag:2"})
	require.NoError(t, err)
	require.Equal(t, "flag:2", cfg.RunAddress)
}
