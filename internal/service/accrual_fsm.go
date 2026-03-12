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
	table map[orderState]map[orderEvent]transitionFn
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

func isFinalState(state orderState) bool {
	return state == stInvalid || state == stProcessed
}

// State transitions:
//
// NEW
//
//	processing      -> PROCESSING
//	invalid         -> INVALID
//	processed       -> PROCESSED
//	not_registered  -> NEW
//
// PROCESSING
//
//	processing      -> PROCESSING
//	invalid         -> INVALID
//	processed       -> PROCESSED
//	not_registered  -> NEW
func newAccrualFSM(repo AccrualOrderRepository) *accrualFSM {
	setStatus := func(status orderState) transitionFn {
		return func(ctx context.Context, ord model.OrderForAccrual, _ accrual.Result) error {
			return repo.SetOrderStatusIfNotFinal(ctx, ord.Number, string(status))
		}
	}
	setProcessed := func(ctx context.Context, ord model.OrderForAccrual, res accrual.Result) error {
		_, err := repo.SetProcessedAndCreditOnce(ctx, ord.Number, res.Accrual)
		return err
	}

	return &accrualFSM{
		table: map[orderState]map[orderEvent]transitionFn{
			stNew: {
				evProcessing:    setStatus(stProcessing),
				evInvalid:       setStatus(stInvalid),
				evProcessed:     setProcessed,
				evNotRegistered: setStatus(stNew),
			},
			stProcessing: {
				evProcessing:    setStatus(stProcessing),
				evInvalid:       setStatus(stInvalid),
				evProcessed:     setProcessed,
				evNotRegistered: setStatus(stNew),
			},
		},
	}
}

func parseOrderState(raw string) (orderState, error) {
	switch raw {
	case string(stNew):
		return stNew, nil
	case string(stProcessing):
		return stProcessing, nil
	case string(stInvalid):
		return stInvalid, nil
	case string(stProcessed):
		return stProcessed, nil
	default:
		return "", fmt.Errorf("unknown order state: %q", raw)
	}
}

func (afsm *accrualFSM) Apply(ctx context.Context, ord model.OrderForAccrual, res accrual.Result) error {
	event, err := eventFromResult(res)
	if err != nil {
		return err
	}

	state, err := parseOrderState(ord.Status)
	if err != nil {
		return err
	}

	if isFinalState(state) {
		return nil
	}

	byEvent, ok := afsm.table[state]
	if !ok {
		return fmt.Errorf("fsm table has no transitions for state: %s", state)
	}

	transition, ok := byEvent[event]
	if !ok {
		return nil
	}
	return transition(ctx, ord, res)
}
