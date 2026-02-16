package accrual

import (
	"context"
	"time"
)

// ResultKind Типизированный enum для результата запроса к accrual
// Убираем строки из воркера, общаемся кодами
// Простой switch бизнес-логики
type ResultKind uint8

const (
	ResultProcessing    ResultKind = iota + 1 // REGISTERED | PROCESSING
	ResultInvalid                             // INVALID
	ResultProcessed                           // PROCESSED
	ResultNotRegistered                       // 204
	ResultRateLimited                         // 429
)

type Result struct {
	Kind       ResultKind
	Accrual    *float64
	RetryAfter time.Duration
}

type Client interface {
	GetOrder(ctx context.Context, number string) (Result, error)
}
