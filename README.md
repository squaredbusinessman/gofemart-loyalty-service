# Gophermart Loyalty Service

Сервис лояльности Gophermart.

## Быстрый запуск

1. Поднимите PostgreSQL:

```bash
docker run --name gophermart-pg \
  -e POSTGRES_PASSWORD=postgres \
  -e POSTGRES_DB=praktikum \
  -p 5433:5432 \
  -d postgres:16
```

2. Запустите сервис:

```bash
cd /Users/evgenijantropov/go19/go1.23.3/src/gofemart-loyalty-service

export RUN_ADDRESS=localhost:8082
export DATABASE_URI='postgresql://postgres:postgres@localhost:5433/praktikum?sslmode=disable'
export ACCRUAL_SYSTEM_ADDRESS='http://localhost:8081'

go run ./cmd/gophermart
```

## Конфигурация

Поддерживаются env переменные:

- `RUN_ADDRESS` (default: `localhost:8080`)
- `DATABASE_URI` (required)
- `ACCRUAL_SYSTEM_ADDRESS` (required)
- `LOG_LEVEL` (default: `info`)
- `AUTH_SECRET` (default: `superverymuchdifficultypasswordword`)
- `AUTH_TOKEN_TTL` (default: `15m`)

Поддерживаются флаги:

- `-a` для `RUN_ADDRESS`
- `-d` для `DATABASE_URI`
- `-r` для `ACCRUAL_SYSTEM_ADDRESS`

Приоритет конфигурации: `flags > env > defaults`.

## Миграции

Миграции применяются автоматически при старте приложения.

- Код запуска миграций: `/Users/evgenijantropov/go19/go1.23.3/src/gofemart-loyalty-service/internal/app/app.go`
- SQL файлы: `/Users/evgenijantropov/go19/go1.23.3/src/gofemart-loyalty-service/migrations`

Если миграции не применились, сервис завершится с ошибкой и не поднимет HTTP сервер.

## Swagger

После запуска сервиса UI доступен по адресу:

- `http://localhost:8082/swagger/index.html`

Перегенерация Swagger:

```bash
go generate ./cmd/gophermart
```

## Диаграмма компонентов и потоков

SVG-версии:

- [Компоненты и потоки](<docs/Gophermart Order Processing-компоненты и потоки.svg>)
- [Последовательность обработки заказа](<docs/Gophermart Order Processing-последовательность обработки заказа.svg>)

```mermaid
flowchart LR
    U["Пользователь / HTTP клиент"]
    API["gophermart: HTTP API"]
    W["gophermart: Accrual Worker (poller)"]
    DB["PostgreSQL"]
    AC["accrual API (внешний сервис)"]

    U -->|"POST/GET /api/user/*"| API
    API -->|"чтение/запись пользователей, заказов, баланса"| DB

    W -->|"выборка NEW/PROCESSING"| DB
    W -->|"GET /api/orders/{number}"| AC
    AC -->|"REGISTERED/PROCESSING/INVALID/PROCESSED/204/429"| W
    W -->|"обновление статусов и транзакционное начисление"| DB
```

Последовательность обработки заказа:

```mermaid
sequenceDiagram
    actor U as Пользователь
    participant G as gophermart API
    participant D as PostgreSQL
    participant W as gophermart Worker
    participant A as accrual API

    U->>G: POST /api/user/orders (номер)
    G->>D: INSERT order(status=NEW)
    G-->>U: 202 Accepted (или 200, если уже загружен этим пользователем)

    loop Периодический polling
        W->>D: SELECT orders WHERE status IN (NEW, PROCESSING)
        W->>A: GET /api/orders/{number}
        A-->>W: status / 204 / 429
        alt PROCESSED
            W->>D: TX: status=PROCESSED + credit once
        else INVALID
            W->>D: status=INVALID
        else REGISTERED или PROCESSING
            W->>D: status=PROCESSING
        else 204 No Content
            W->>D: оставить NEW
        else 429 Too Many Requests
            W->>W: глобальная пауза по Retry-After
        end
    end
```

## Тесты и coverage

Для воспроизводимого локального прогона используйте `make`:

```bash
make test
make coverage
make coverage-func
make coverage-html
```

- `make test` — прогоняет все тесты (`go test ./...`).
- `make coverage` — собирает покрытие в `coverage.out`.
- `make coverage-func` — показывает покрытие по функциям.
- `make coverage-html` — строит HTML-отчет через `go tool cover`.
