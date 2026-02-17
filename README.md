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
