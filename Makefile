GO := go
BINARY := bin/app
MAIN := ./cmd/main.go
COVER_FILE := coverage.out
COVER_HTML := coverage.html

.PHONY: run build test testv cover cover-func cover-html generate fmt vet tidy check clean

run:
	$(GO) run $(MAIN)

build:
	@mkdir -p bin
	$(GO) build -o $(BINARY) $(MAIN)

# Юнит-тесты всего проекта
test:
	$(GO) test ./...

testv:
	$(GO) test ./... -v -count=1

# Краткое покрытие по всем пакетам
cover:
	$(GO) test ./... -cover -count=1

# Подробный отчёт покрытия по сервису (функции)
cover-func:
	$(GO) test ./internal/services/archive_service -coverprofile=$(COVER_FILE) -count=1
	$(GO) tool cover -func=$(COVER_FILE)

# HTML-отчёт покрытия по сервису
cover-html:
	$(GO) test ./internal/services/archive_service -coverprofile=$(COVER_FILE) -count=1
	$(GO) tool cover -html=$(COVER_FILE) -o $(COVER_HTML)
	open $(COVER_HTML)

# Генерация моков (go:generate в интерфейсах)
generate:
	$(GO) generate ./...

fmt:
	gofmt -s -w .

vet:
	$(GO) vet ./...

tidy:
	$(GO) mod tidy

# Быстрая проверка локально
check: fmt vet test

clean:
	rm -rf bin $(COVER_FILE) $(COVER_HTML)
