package accrual

import (
	"time"

	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/option"
)

type clientConfig struct {
	timeout    time.Duration
	maxRetries int
}

func defaultClientConfig() clientConfig {
	return clientConfig{
		timeout:    3 * time.Second,
		maxRetries: 0,
	}
}

func WithTimeout(d time.Duration) option.Option[clientConfig] {
	return func(c *clientConfig) {
		if d > 0 {
			c.timeout = d
		}
	}
}

func WithMaxRetries(n int) option.Option[clientConfig] {
	return func(c *clientConfig) {
		if n >= 0 {
			c.maxRetries = n
		}
	}
}
