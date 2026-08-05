.PHONY: help docker-up docker-down docker-restart docker-logs docker-ps run build tidy fmt vet test

BINARY := build/app/whereabouts

help: ## Показать список команд
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-10s\033[0m %s\n", $$1, $$2}'

docker-up: ## Поднять докер (MariaDB) в фоне
	docker compose up -d

docker-down: ## Остановить и удалить контейнеры докера
	docker compose down

docker-restart: docker-down docker-up ## Перезапустить докер

docker-logs: ## Логи докер-контейнеров
	docker compose logs -f

docker-ps: ## Статус контейнеров
	docker compose ps

run: ## Запустить приложение (go run .)
	go run .

build: ## Собрать бинарник для Linux (amd64) в build/app/whereabouts
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $(BINARY) .

tidy: ## Подтянуть/почистить зависимости
	go mod tidy

fmt: ## Форматировать код
	go fmt ./...

vet: ## Статический анализ
	go vet ./...

test:
	@go test ./...
