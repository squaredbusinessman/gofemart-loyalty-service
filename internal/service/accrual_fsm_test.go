package service

import (
	"context"
	"errors"
	"testing"

	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/accrual"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/model"
	"github.com/stretchr/testify/require"
)

type fsmStatusCall struct {
	number string
	status string
}

type fsmProcessedCall struct {
	number  string
	accrual *float64
}

type fsmRepoStub struct {
	setStatusErr error
	setProcErr   error

	setStatusCalls []fsmStatusCall
	setProcCalls   []fsmProcessedCall
}

func (s *fsmRepoStub) ListOrdersForAccrual(ctx context.Context, limit int) ([]model.OrderForAccrual, error) {
	return nil, nil
}

func (s *fsmRepoStub) SetOrderStatusIfNotFinal(ctx context.Context, number string, status string) error {
	s.setStatusCalls = append(s.setStatusCalls, fsmStatusCall{
		number: number,
		status: status,
	})
	return s.setStatusErr
}

func (s *fsmRepoStub) SetProcessedAndCreditOnce(ctx context.Context, number string, accrualValue *float64) (bool, error) {
	s.setProcCalls = append(s.setProcCalls, fsmProcessedCall{
		number:  number,
		accrual: accrualValue,
	})
	if s.setProcErr != nil {
		return false, s.setProcErr
	}
	return true, nil
}

func TestEventFromResult(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		kind      accrual.ResultKind
		wantEvent orderEvent
		wantErr   bool
	}{
		{name: "processing", kind: accrual.ResultProcessing, wantEvent: evProcessing},
		{name: "invalid", kind: accrual.ResultInvalid, wantEvent: evInvalid},
		{name: "processed", kind: accrual.ResultProcessed, wantEvent: evProcessed},
		{name: "not registered", kind: accrual.ResultNotRegistered, wantEvent: evNotRegistered},
		{name: "unsupported kind", kind: accrual.ResultRateLimited, wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := eventFromResult(accrual.Result{Kind: tt.kind})
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantEvent, got)
		})
	}
}

func TestAccrualFSM_Apply_Transitions(t *testing.T) {
	t.Parallel()

	accrualValue := 133.7

	tests := []struct {
		name           string
		initialStatus  string
		res            accrual.Result
		wantStatusCall *fsmStatusCall
		wantProcCall   *fsmProcessedCall
	}{
		{
			name:          "NEW + processing -> PROCESSING",
			initialStatus: "NEW",
			res:           accrual.Result{Kind: accrual.ResultProcessing},
			wantStatusCall: &fsmStatusCall{
				number: "order-1",
				status: "PROCESSING",
			},
		},
		{
			name:          "NEW + invalid -> INVALID",
			initialStatus: "NEW",
			res:           accrual.Result{Kind: accrual.ResultInvalid},
			wantStatusCall: &fsmStatusCall{
				number: "order-1",
				status: "INVALID",
			},
		},
		{
			name:          "NEW + not registered -> NEW",
			initialStatus: "NEW",
			res:           accrual.Result{Kind: accrual.ResultNotRegistered},
			wantStatusCall: &fsmStatusCall{
				number: "order-1",
				status: "NEW",
			},
		},
		{
			name:          "NEW + processed -> processed and credit once",
			initialStatus: "NEW",
			res:           accrual.Result{Kind: accrual.ResultProcessed, Accrual: &accrualValue},
			wantProcCall: &fsmProcessedCall{
				number:  "order-1",
				accrual: &accrualValue,
			},
		},
		{
			name:          "PROCESSING + processing -> PROCESSING",
			initialStatus: "PROCESSING",
			res:           accrual.Result{Kind: accrual.ResultProcessing},
			wantStatusCall: &fsmStatusCall{
				number: "order-1",
				status: "PROCESSING",
			},
		},
		{
			name:          "PROCESSING + invalid -> INVALID",
			initialStatus: "PROCESSING",
			res:           accrual.Result{Kind: accrual.ResultInvalid},
			wantStatusCall: &fsmStatusCall{
				number: "order-1",
				status: "INVALID",
			},
		},
		{
			name:          "PROCESSING + not registered -> NEW",
			initialStatus: "PROCESSING",
			res:           accrual.Result{Kind: accrual.ResultNotRegistered},
			wantStatusCall: &fsmStatusCall{
				number: "order-1",
				status: "NEW",
			},
		},
		{
			name:          "PROCESSING + processed -> processed and credit once",
			initialStatus: "PROCESSING",
			res:           accrual.Result{Kind: accrual.ResultProcessed, Accrual: &accrualValue},
			wantProcCall: &fsmProcessedCall{
				number:  "order-1",
				accrual: &accrualValue,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := &fsmRepoStub{}
			fsm := newAccrualFSM(repo)
			order := model.OrderForAccrual{
				Number: "order-1",
				UserID: 1,
				Status: tt.initialStatus,
			}

			err := fsm.Apply(context.Background(), order, tt.res)
			require.NoError(t, err)

			if tt.wantStatusCall != nil {
				require.Len(t, repo.setStatusCalls, 1)
				require.Equal(t, *tt.wantStatusCall, repo.setStatusCalls[0])
			} else {
				require.Empty(t, repo.setStatusCalls)
			}

			if tt.wantProcCall != nil {
				require.Len(t, repo.setProcCalls, 1)
				require.Equal(t, tt.wantProcCall.number, repo.setProcCalls[0].number)
				require.Equal(t, tt.wantProcCall.accrual, repo.setProcCalls[0].accrual)
			} else {
				require.Empty(t, repo.setProcCalls)
			}
		})
	}
}

func TestAccrualFSM_Apply_FinalStatesAreNoOp(t *testing.T) {
	t.Parallel()

	tests := []string{"INVALID", "PROCESSED"}
	for _, st := range tests {
		st := st
		t.Run(st, func(t *testing.T) {
			t.Parallel()

			repo := &fsmRepoStub{}
			fsm := newAccrualFSM(repo)
			order := model.OrderForAccrual{
				Number: "order-1",
				UserID: 1,
				Status: st,
			}

			err := fsm.Apply(context.Background(), order, accrual.Result{Kind: accrual.ResultProcessing})
			require.NoError(t, err)
			require.Empty(t, repo.setStatusCalls)
			require.Empty(t, repo.setProcCalls)
		})
	}
}

func TestAccrualFSM_Apply_UnknownStateReturnsError(t *testing.T) {
	t.Parallel()

	repo := &fsmRepoStub{}
	fsm := newAccrualFSM(repo)
	order := model.OrderForAccrual{
		Number: "order-1",
		UserID: 1,
		Status: "BROKEN",
	}

	err := fsm.Apply(context.Background(), order, accrual.Result{Kind: accrual.ResultProcessing})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown order state")
	require.Empty(t, repo.setStatusCalls)
	require.Empty(t, repo.setProcCalls)
}

func TestAccrualFSM_Apply_UnsupportedResultKindReturnsError(t *testing.T) {
	t.Parallel()

	repo := &fsmRepoStub{}
	fsm := newAccrualFSM(repo)
	order := model.OrderForAccrual{
		Number: "order-1",
		UserID: 1,
		Status: "NEW",
	}

	err := fsm.Apply(context.Background(), order, accrual.Result{Kind: accrual.ResultRateLimited})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported result kind")
	require.Empty(t, repo.setStatusCalls)
	require.Empty(t, repo.setProcCalls)
}

func TestAccrualFSM_Apply_RepositoryErrors(t *testing.T) {
	t.Parallel()

	t.Run("status transition error", func(t *testing.T) {
		t.Parallel()

		wantErr := errors.New("set status failed")
		repo := &fsmRepoStub{setStatusErr: wantErr}
		fsm := newAccrualFSM(repo)
		order := model.OrderForAccrual{
			Number: "order-1",
			UserID: 1,
			Status: "NEW",
		}

		err := fsm.Apply(context.Background(), order, accrual.Result{Kind: accrual.ResultInvalid})
		require.ErrorIs(t, err, wantErr)
		require.Len(t, repo.setStatusCalls, 1)
		require.Empty(t, repo.setProcCalls)
	})

	t.Run("processed transition error", func(t *testing.T) {
		t.Parallel()

		wantErr := errors.New("processed update failed")
		accrualValue := 77.7
		repo := &fsmRepoStub{setProcErr: wantErr}
		fsm := newAccrualFSM(repo)
		order := model.OrderForAccrual{
			Number: "order-1",
			UserID: 1,
			Status: "PROCESSING",
		}

		err := fsm.Apply(context.Background(), order, accrual.Result{
			Kind:    accrual.ResultProcessed,
			Accrual: &accrualValue,
		})
		require.ErrorIs(t, err, wantErr)
		require.Empty(t, repo.setStatusCalls)
		require.Len(t, repo.setProcCalls, 1)
	})
}
