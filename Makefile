.PHONY: test coverage coverage-func coverage-html

COVERAGE_FILE ?= coverage.out
COVERAGE_PKGS := ./internal/accrual ./internal/auth ./internal/config ./internal/handler ./internal/middleware ./internal/service

test:
	go test ./...

coverage:
	go test $(COVERAGE_PKGS) -coverprofile=$(COVERAGE_FILE)

coverage-func: coverage
	go tool cover -func=$(COVERAGE_FILE)

coverage-html: coverage
	go tool cover -html=$(COVERAGE_FILE)
