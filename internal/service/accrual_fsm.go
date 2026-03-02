package service

import (
	"context"
	"fmt"

	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/accrual"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/model"
)

type orderState string
type orderEvent string

const (
	stNew        orderState = "NEW"
	stProcessing orderState = "PROCESSING"
	stInvalid    orderState = "INVALID"
	stProcessed  orderState = "PROCESSED"

	evProcessing    orderEvent = "processing"     // REGISTERED|PROCESSING
	evInvalid       orderEvent = "invalid"        // INVALID
	evProcessed     orderEvent = "processed"      // PROCESSED
	evNotRegistered orderEvent = "not_registered" // 204
)

type transitionFn func(ctx context.Context, ord model.OrderForAccrual, res accrual.Result) error

type accrualFSM struct {
	repo  AccrualOrderRepository
	table map[orderState]map[orderEvent]transitionFn
}

func newAccrualFSM(repo AccrualOrderRepository) *accrualFSM {
	setStatus := func(status string) transitionFn {
		return func(ctx context.Context, ord model.OrderForAccrual, _ accrual.Result) error {
			return repo.SetOrderStatusIfNotFinal(ctx, ord.Number, status)
		}
	}
	setProcessed := func(ctx context.Context, ord model.OrderForAccrual, res accrual.Result) error {
		_, err := repo.SetProcessedAndCreditOnce(ctx, ord.Number, res.Accrual)
		return err
	}

	return &accrualFSM{
		repo: repo,
		table: map[orderState]map[orderEvent]transitionFn{
			stNew: {
				evProcessing:    setStatus("PROCESSING"),
				evInvalid:       setStatus("INVALID"),
				evProcessed:     setProcessed,
				evNotRegistered: setStatus("NEW"),
			},
			stProcessing: {
				evProcessing:    setStatus("PROCESSING"),
				evInvalid:       setStatus("INVALID"),
				evProcessed:     setProcessed,
				evNotRegistered: setStatus("NEW"),
			},
		},
	}
}

func eventFromResult(res accrual.Result) (orderEvent, error) {
	switch res.Kind {
	case accrual.ResultProcessing:
		return evProcessing, nil
	case accrual.ResultInvalid:
		return evInvalid, nil
	case accrual.ResultProcessed:
		return evProcessed, nil
	case accrual.ResultNotRegistered:
		return evNotRegistered, nil
	default:
		return "", fmt.Errorf("unsupported result kind: %v", res.Kind)
	}
}

func (afsm *accrualFSM) Apply(ctx context.Context, ord model.OrderForAccrual, res accrual.Result) error {
	event, err := eventFromResult(res)
	if err != nil {
		return err
	}
	state := orderState(ord.Status)

	if !isKnownState(state) {
		return fmt.Errorf("unknown order state: %q", ord.Status)
	}

	if isFinalState(state) {
		return nil
	}

	byEvent := afsm.table[state]
	transition, ok := byEvent[event]
	if !ok {
		return nil // недопустимый переход
	}

	return transition(ctx, ord, res)
}

func isFinalState(state orderState) bool {
	return state == stInvalid || state == stProcessed
}

func isKnownState(state orderState) bool {
	return state == stNew || state == stProcessing || state == stInvalid || state == stProcessed
}
