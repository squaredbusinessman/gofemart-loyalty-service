package service

import (
	"context"
	"fmt"

	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/accrual"
	"github.com/squaredbusinessman/gofemart-loyalty-service/internal/model"
)

// FSM в этом файле отвечает только за бизнес-переходы статусов заказа.
// Важные границы ответственности:
//  1. Здесь нет HTTP/Retry/429-логики — это зона worker/client.
//  2. Здесь нет логирования — вызывающий слой логирует контекст ошибки (номер заказа и т.д.).
//  3. Переход в PROCESSED делегирует в репозиторий атомарную операцию SetProcessedAndCreditOnce,
//     чтобы начисление происходило ровно один раз.
//
// Это keeps-it-simple FSM: таблица переходов + функции-экшены.
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
	// table[state][event] => transition function
	table map[orderState]map[orderEvent]transitionFn
}

// eventFromResult переводит ответ accrual-клиента в внутреннее событие FSM.
// Если ResultKind не относится к переходам состояния (например rate-limit), возвращаем ошибку.
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

// Финальные состояния не должны изменяться повторным polling.
// Это защищает от повторной обработки уже закрытых заказов.
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
//
// Почему not_registered -> NEW:
// accrual вернул 204, значит заказ еще не зарегистрирован во внешней системе.
// Локально оставляем заказ в ожидании.
func newAccrualFSM(repo AccrualOrderRepository) *accrualFSM {
	setStatus := func(status orderState) transitionFn {
		return func(ctx context.Context, ord model.OrderForAccrual, _ accrual.Result) error {
			return repo.SetOrderStatusIfNotFinal(ctx, ord.Number, string(status))
		}
	}

	// setProcessed не обновляет статус напрямую, а вызывает атомарный метод репозитория:
	// смена статуса + начисление баланса в одной транзакции.
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

// parseOrderState валидирует сырой статус из БД и переводит его в типизированное состояние FSM.
// Явный парсинг вместо type-cast нужен, чтобы не проглатывать неизвестные значения.
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

// Apply выполняет один шаг FSM для конкретного заказа:
// status(from DB) + result(from accrual) -> transition action.
// Возвращает ошибку только для действительно аномальных ситуаций:
//   - неизвестный статус в БД;
//   - неподдерживаемый result kind;
//   - поломанная таблица переходов;
//   - ошибка репозитория при выполнении transition action.
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
