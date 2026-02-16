package service

import (
	"context"
	"errors"
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

	for _, ord := range orders {
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

		switch res.Kind {
		case accrual.ResultProcessing:
			if err = aw.repo.SetOrderStatusIfNotFinal(ctx, ord.Number, "PROCESSING"); err != nil {
				aw.log.Warn("set PROCESSING status was failed", zap.String("order", ord.Number), zap.Error(err))
			}
		case accrual.ResultInvalid:
			if err = aw.repo.SetOrderStatusIfNotFinal(ctx, ord.Number, "INVALID"); err != nil {
				aw.log.Warn("set INVALID status was failed", zap.String("order", ord.Number), zap.Error(err))
			}
		case accrual.ResultNotRegistered:
			// 204
			if err = aw.repo.SetOrderStatusIfNotFinal(ctx, ord.Number, "NEW"); err != nil {
				aw.log.Warn("set NEW status was failed", zap.String("order", ord.Number), zap.Error(err))
			}
		case accrual.ResultProcessed:
			if _, err = aw.repo.SetProcessedAndCreditOnce(ctx, ord.Number, res.Accrual); err != nil {
				aw.log.Warn("set PROCESSED status and credit user were failed", zap.String("order", ord.Number), zap.Error(err))
			}
		case accrual.ResultRateLimited:
			if res.RetryAfter > 0 {
				aw.blockedUntil = aw.timeNowFunc().Add(res.RetryAfter)
				aw.log.Warn("accrual was rate limited, worker is blocked", zap.Duration("retry_after", res.RetryAfter), zap.Time("blocked_until", aw.blockedUntil))
			}
			return
		}
	}
}
