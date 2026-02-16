package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/accrual"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/model"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type stubAccrualWorkerRepo struct {
	listOrdersFn func(ctx context.Context, limit int) ([]model.OrderForAccrual, error)

	setStatusCalls []statusCall
	setStatusErr   error

	setProcessedCalls []processedCall
	setProcessedErr   error
}

type statusCall struct {
	number string
	status string
}

type processedCall struct {
	number  string
	accrual *float64
}

func (s *stubAccrualWorkerRepo) ListOrdersForAccrual(ctx context.Context, limit int) ([]model.OrderForAccrual, error) {
	if s.listOrdersFn == nil {
		return nil, errors.New("unexpected ListOrdersForAccrual call")
	}
	return s.listOrdersFn(ctx, limit)
}

func (s *stubAccrualWorkerRepo) SetOrderStatusIfNotFinal(ctx context.Context, number string, status string) error {
	s.setStatusCalls = append(s.setStatusCalls, statusCall{number: number, status: status})
	return s.setStatusErr
}

func (s *stubAccrualWorkerRepo) SetProcessedAndCreditOnce(ctx context.Context, number string, accrualValue *float64) (bool, error) {
	s.setProcessedCalls = append(s.setProcessedCalls, processedCall{number: number, accrual: accrualValue})
	if s.setProcessedErr != nil {
		return false, s.setProcessedErr
	}
	return true, nil
}

type stubAccrualClient struct {
	getOrderFn func(ctx context.Context, number string) (accrual.Result, error)
	calls      []string
}

func (s *stubAccrualClient) GetOrder(ctx context.Context, number string) (accrual.Result, error) {
	s.calls = append(s.calls, number)
	if s.getOrderFn == nil {
		return accrual.Result{}, errors.New("unexpected GetOrder call")
	}
	return s.getOrderFn(ctx, number)
}

func TestAccrualWorker_PollOnce_StatusMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		resultKind accrual.ResultKind
		wantStatus string
	}{
		{
			name:       "REGISTERED or PROCESSING maps to local PROCESSING",
			resultKind: accrual.ResultProcessing,
			wantStatus: "PROCESSING",
		},
		{
			name:       "INVALID maps to local INVALID",
			resultKind: accrual.ResultInvalid,
			wantStatus: "INVALID",
		},
		{
			name:       "204 maps to local NEW",
			resultKind: accrual.ResultNotRegistered,
			wantStatus: "NEW",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := &stubAccrualWorkerRepo{
				listOrdersFn: func(ctx context.Context, limit int) ([]model.OrderForAccrual, error) {
					return []model.OrderForAccrual{
						{Number: "2377225624", UserID: 1, Status: "NEW"},
					}, nil
				},
			}
			client := &stubAccrualClient{
				getOrderFn: func(ctx context.Context, number string) (accrual.Result, error) {
					return accrual.Result{Kind: tt.resultKind}, nil
				},
			}

			worker := NewAccrualWorker(repo, client, zap.NewNop(), time.Second, 10)
			worker.pollOnce(context.Background())

			require.Len(t, client.calls, 1)
			require.Equal(t, "2377225624", client.calls[0])
			require.Len(t, repo.setStatusCalls, 1)
			require.Equal(t, tt.wantStatus, repo.setStatusCalls[0].status)
			require.Empty(t, repo.setProcessedCalls)
		})
	}
}

func TestAccrualWorker_PollOnce_ProcessedCallsAtomicCredit(t *testing.T) {
	t.Parallel()

	accrualValue := 500.25
	repo := &stubAccrualWorkerRepo{
		listOrdersFn: func(ctx context.Context, limit int) ([]model.OrderForAccrual, error) {
			return []model.OrderForAccrual{
				{Number: "2377225624", UserID: 1, Status: "PROCESSING"},
			}, nil
		},
	}
	client := &stubAccrualClient{
		getOrderFn: func(ctx context.Context, number string) (accrual.Result, error) {
			return accrual.Result{Kind: accrual.ResultProcessed, Accrual: &accrualValue}, nil
		},
	}

	worker := NewAccrualWorker(repo, client, zap.NewNop(), time.Second, 10)
	worker.pollOnce(context.Background())

	require.Empty(t, repo.setStatusCalls)
	require.Len(t, repo.setProcessedCalls, 1)
	require.Equal(t, "2377225624", repo.setProcessedCalls[0].number)
	require.NotNil(t, repo.setProcessedCalls[0].accrual)
	require.InDelta(t, accrualValue, *repo.setProcessedCalls[0].accrual, 0.000001)
}

func TestAccrualWorker_PollOnce_RateLimitedSetsGlobalBlock(t *testing.T) {
	t.Parallel()

	listCalls := 0
	repo := &stubAccrualWorkerRepo{
		listOrdersFn: func(ctx context.Context, limit int) ([]model.OrderForAccrual, error) {
			listCalls++
			return []model.OrderForAccrual{
				{Number: "1111111111", UserID: 1, Status: "NEW"},
				{Number: "2222222222", UserID: 2, Status: "NEW"},
			}, nil
		},
	}
	client := &stubAccrualClient{
		getOrderFn: func(ctx context.Context, number string) (accrual.Result, error) {
			return accrual.Result{
				Kind:       accrual.ResultRateLimited,
				RetryAfter: 5 * time.Second,
			}, nil
		},
	}

	now := time.Date(2026, 2, 16, 10, 0, 0, 0, time.UTC)
	worker := NewAccrualWorker(repo, client, zap.NewNop(), time.Second, 10)
	worker.timeNowFunc = func() time.Time { return now }

	worker.pollOnce(context.Background())
	require.Len(t, client.calls, 1)
	require.Equal(t, now.Add(5*time.Second), worker.blockedUntil)

	// While blocked, worker should not fetch a new batch.
	now = now.Add(2 * time.Second)
	worker.pollOnce(context.Background())
	require.Equal(t, 1, listCalls)
}

func TestAccrualWorker_PollOnce_ClientErrorContinuesBatch(t *testing.T) {
	t.Parallel()

	repo := &stubAccrualWorkerRepo{
		listOrdersFn: func(ctx context.Context, limit int) ([]model.OrderForAccrual, error) {
			return []model.OrderForAccrual{
				{Number: "1111111111", UserID: 1, Status: "NEW"},
				{Number: "2222222222", UserID: 2, Status: "NEW"},
			}, nil
		},
	}
	client := &stubAccrualClient{
		getOrderFn: func(ctx context.Context, number string) (accrual.Result, error) {
			if number == "1111111111" {
				return accrual.Result{}, errors.New("temporary network issue")
			}
			return accrual.Result{Kind: accrual.ResultProcessing}, nil
		},
	}

	worker := NewAccrualWorker(repo, client, zap.NewNop(), time.Second, 10)
	worker.pollOnce(context.Background())

	require.Equal(t, []string{"1111111111", "2222222222"}, client.calls)
	require.Len(t, repo.setStatusCalls, 1)
	require.Equal(t, "2222222222", repo.setStatusCalls[0].number)
	require.Equal(t, "PROCESSING", repo.setStatusCalls[0].status)
}

