-- +goose Up
-- +goose StatementBegin

CREATE TABLE users (
                       id BIGSERIAL PRIMARY KEY,
                       login TEXT NOT NULL UNIQUE,
                       password_hash TEXT NOT NULL,
                       current_balance NUMERIC(14,2) NOT NULL DEFAULT 0 CHECK (current_balance >= 0),
                       withdrawn_total NUMERIC(14,2) NOT NULL DEFAULT 0 CHECK (withdrawn_total >= 0),
                       created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE users IS 'Пользователи системы лояльности';
COMMENT ON COLUMN users.id IS 'PK пользователя';
COMMENT ON COLUMN users.login IS 'Уникальный логин пользователя';
COMMENT ON COLUMN users.password_hash IS 'Хэш пароля пользователя';
COMMENT ON COLUMN users.current_balance IS 'Текущий баланс баллов пользователя';
COMMENT ON COLUMN users.withdrawn_total IS 'Суммарно списано баллов пользователем';
COMMENT ON COLUMN users.created_at IS 'Время создания учетной записи';

CREATE TABLE orders (
                        id BIGSERIAL PRIMARY KEY,
                        number TEXT NOT NULL UNIQUE,
                        user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
                        status TEXT NOT NULL CHECK (status IN ('NEW', 'PROCESSING', 'INVALID', 'PROCESSED')),
                        accrual NUMERIC(14,2) NULL CHECK (accrual IS NULL OR accrual >= 0),
                        uploaded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE orders IS 'Заказы, загруженные пользователями для начисления баллов';
COMMENT ON COLUMN orders.id IS 'PK записи заказа';
COMMENT ON COLUMN orders.number IS 'Уникальный номер заказа';
COMMENT ON COLUMN orders.user_id IS 'ID пользователя, загрузившего заказ';
COMMENT ON COLUMN orders.status IS 'Статус обработки заказа';
COMMENT ON COLUMN orders.accrual IS 'Сумма начисления в баллах; NULL, если начисление отсутствует';
COMMENT ON COLUMN orders.uploaded_at IS 'Время загрузки заказа пользователем';

CREATE TABLE withdrawals (
                             id BIGSERIAL PRIMARY KEY,
                             user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
                             order_number TEXT NOT NULL UNIQUE,
                             sum NUMERIC(14,2) NOT NULL CHECK (sum > 0),
                             processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE withdrawals IS 'Списания баллов пользователей';
COMMENT ON COLUMN withdrawals.id IS 'PK записи списания';
COMMENT ON COLUMN withdrawals.user_id IS 'ID пользователя-владельца списания';
COMMENT ON COLUMN withdrawals.order_number IS 'Номер заказа, в счет которого выполнено списание (уникален)';
COMMENT ON COLUMN withdrawals.sum IS 'Сумма списания в баллах, строго > 0';
COMMENT ON COLUMN withdrawals.processed_at IS 'Время регистрации списания';

CREATE INDEX idx_orders_user_id ON orders(user_id);
CREATE INDEX idx_orders_uploaded_at ON orders(uploaded_at);
CREATE INDEX idx_withdrawals_user_id ON withdrawals(user_id);
CREATE INDEX idx_withdrawals_processed_at ON withdrawals(processed_at);

CREATE INDEX idx_orders_user_uploaded_desc
    ON orders(user_id, uploaded_at DESC);

CREATE INDEX idx_withdrawals_user_processed_desc
    ON withdrawals(user_id, processed_at DESC);

COMMENT ON INDEX idx_orders_user_id IS 'Индекс выборки заказов по пользователю';
COMMENT ON INDEX idx_orders_uploaded_at IS 'Индекс сортировки/фильтрации заказов по времени загрузки';
COMMENT ON INDEX idx_withdrawals_user_id IS 'Индекс выборки списаний по пользователю';
COMMENT ON INDEX idx_withdrawals_processed_at IS 'Индекс сортировки/фильтрации списаний по времени';
COMMENT ON INDEX idx_orders_user_uploaded_desc IS 'Оптимизация GET /api/user/orders';
COMMENT ON INDEX idx_withdrawals_user_processed_desc IS 'Оптимизация GET /api/user/withdrawals';

-- +goose StatementEnd


-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_withdrawals_user_processed_desc;
DROP INDEX IF EXISTS idx_orders_user_uploaded_desc;
DROP INDEX IF EXISTS idx_withdrawals_processed_at;
DROP INDEX IF EXISTS idx_withdrawals_user_id;
DROP INDEX IF EXISTS idx_orders_uploaded_at;
DROP INDEX IF EXISTS idx_orders_user_id;

DROP TABLE IF EXISTS withdrawals;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS users;

-- +goose StatementEnd
