package service

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/accrual"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/model"
	"go.uber.org/zap"
)

type AccrualOrderRepository interface {
	ListOrdersForAccrual(ctx context.Context, limit int) ([]model.OrderForAccrual, error)
	SetOrderStatusIfNotFinal(ctx context.Context, number string, status string) error
	SetProcessedAndCreditOnce(ctx context.Context, number string, accrual *float64) (bool, error)
}

type AccrualWorker struct {
	repo         AccrualOrderRepository
	client       accrual.Client
	fsm          *accrualFSM
	log          *zap.Logger
	pollInterval time.Duration
	batchSize    int
	timeNowFunc  func() time.Time
	blockedUntil time.Time
}

func NewAccrualWorker(repo AccrualOrderRepository, client accrual.Client, log *zap.Logger, pollInterval time.Duration, batchSize int) *AccrualWorker {
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}
	if batchSize <= 0 {
		batchSize = 20
	}

	return &AccrualWorker{
		repo:         repo,
		client:       client,
		fsm:          newAccrualFSM(repo),
		log:          log,
		pollInterval: pollInterval,
		batchSize:    batchSize,
		timeNowFunc:  time.Now,
		blockedUntil: time.Time{},
	}
}

func (aw *AccrualWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(aw.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			aw.pollOnce(ctx)
		}
	}
}

func (aw *AccrualWorker) pollOnce(ctx context.Context) {
	if aw.timeNowFunc().Before(aw.blockedUntil) {
		return
	}

	orders, err := aw.repo.ListOrdersForAccrual(ctx, aw.batchSize)
	if err != nil {
		aw.log.Error("list orders for accrual failed", zap.Error(err))
		return
	}

	for ord := range slices.Values(orders) {
		if ctx.Err() != nil {
			return
		}
		if aw.timeNowFunc().Before(aw.blockedUntil) {
			return
		}

		res, err := aw.client.GetOrder(ctx, ord.Number)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				aw.log.Warn("accrual request failed", zap.String("order", ord.Number), zap.Error(err))
			}
			continue
		}

		if res.Kind == accrual.ResultRateLimited {
			if res.RetryAfter > 0 {
				aw.blockedUntil = aw.timeNowFunc().Add(res.RetryAfter)
				aw.log.Warn("accrual was rate limited, worker is blocked",
					zap.Duration("retry_after", res.RetryAfter),
					zap.Time("blocked_until", aw.blockedUntil),
				)
			}
			return
		}

		if err = aw.fsm.Apply(ctx, ord, res); err != nil {
			aw.log.Warn("accrual fsm apply failed",
				zap.String("order", ord.Number),
				zap.String("current_status", ord.Status),
				zap.Any("result_kind", res.Kind),
				zap.Error(err),
			)
		}
	}
}
