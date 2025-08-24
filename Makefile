# =============================================================================
# Variables
# =============================================================================
TOPIC ?= orders
BROKER_SERVICE ?= redpanda
FILE ?= fixtures/model.json
APP_CMD = go run ./cmd/api

#Tool paths autodetect
GOBIN := $(shell go env GOBIN)
ifeq ($(GOBIN),)
  GOBIN := $(shell go env GOPATH)/bin
endif
GOLANGCI := $(shell command -v golangci-lint 2>/dev/null || echo $(GOBIN)/golangci-lint)

# Если есть .env — берём его, иначе .env.example
ENV_FILE ?= $(if $(wildcard .env),.env,.env.example)

# URLы
HTTP_BASE ?= http://localhost:8081
DEMO_ORDER ?= b563feb7b2b84b6test

# Определяем команду открыть в браузере"
UNAME_S := $(shell uname)
OPEN_CMD := xdg-open
ifeq ($(UNAME_S),Darwin)
  OPEN_CMD := open
endif

# =============================================================================
# Colors
# =============================================================================
GREEN := \033[1;32m
YELLOW := \033[1;33m
RED := \033[1;31m
CYAN := \033[1;36m
BOLD := \033[1m
NC := \033[0m

# выбрать "open" для macOS / Linux
ifeq ($(shell uname),Darwin)
	OPEN := open
else
	OPEN := xdg-open
endif

# =============================================================================
# Phony & Defaults
# =============================================================================
.PHONY: help \
        up down ps logs wait-db wait-kafka wait-http \
        topic topic-list topic-reset seed consume \
        run dev dbshell kafsh open-ui reset-demo \
        test test-race cover cover-html lint lint-install fmt fmt-check clean clean-cover \
        jq-check jq-install deps-install

.DEFAULT_GOAL := help

# =============================================================================
# Help
# =============================================================================
help:
	@echo "$(BOLD)Available targets$(NC):"
	@awk 'BEGIN {FS=":.*##"; OFS=""} \
	     /^[a-zA-Z0-9_.-]+:.*##/ {printf "  $(CYAN)%-18s$(NC) %s\n", $$1, $$2}' $(MAKEFILE_LIST)


# =============================================================================
# Docker & Infra
# =============================================================================
up: ## Поднять Postgres и Redpanda в фоне
	docker compose up -d db_auth redpanda

down: ## Остановить и удалить контейнеры и тома
	docker compose down -v || true

ps: ## Показать статус контейнеров
	docker compose ps

logs: ## Хвост логов всех сервисов
	docker compose logs -f --tail=200

wait-db:  ## Ждать готовность Postgres
	@echo "Waiting for Postgres..."
	@until docker compose exec -T db_auth pg_isready -U admin -d postgres -h localhost >/dev/null 2>&1; do sleep 0.5; done
	@echo "Postgres is ready."

wait-kafka: ## Ждать готовность Redpanda
	@echo "Waiting for Redpanda..."
	@until docker compose exec -T $(BROKER_SERVICE) rpk cluster info >/dev/null 2>&1; do sleep 0.5; done
	@echo "Redpanda is ready."

wait-http: ## Ждать готовность HTTP-сервера
	@echo "Waiting for $(HTTP_BASE)..."
	@until curl -fsS -o /dev/null "$(HTTP_BASE)/"; do sleep 0.3; done
	@echo "HTTP is ready."

# =============================================================================
# Kafka tools
# =============================================================================
topic: ## Создать топик
	- docker compose exec -T $(BROKER_SERVICE) rpk topic create $(TOPIC) -p 1 -r 1

topic-list: ## Список топиков
	docker compose exec -T $(BROKER_SERVICE) rpk topic list

topic-reset: ## Пересоздать топик
	- docker compose exec -T $(BROKER_SERVICE) rpk topic delete $(TOPIC)
	docker compose exec -T $(BROKER_SERVICE) rpk topic create $(TOPIC) -p 1 -r 1

seed: topic ## Отправить сообщение в топик
	@KEY_VAL=$${KEY:-$$(jq -r '.order_uid' $(FILE))}; \
	test -n "$$KEY_VAL" || { echo "KEY пуст. Передай KEY=... или положи order_uid в $(FILE)"; exit 1; }; \
	echo "Producing to '$(TOPIC)' with key '$$KEY_VAL' from '$(FILE)'..."; \
	jq -c . $(FILE) | docker compose exec -T $(BROKER_SERVICE) rpk topic produce $(TOPIC) -k "$$KEY_VAL"; \
	echo "Done."

consume: ## Прочитать сообщения из топика
	docker compose exec -T $(BROKER_SERVICE) rpk topic consume $(TOPIC) -n $${N:-1}

# =============================================================================
# App run
# =============================================================================
run: ## Запустить API с подхватом переменных
	set -a; source $(ENV_FILE); set +a; $(APP_CMD)

dev: up topic run ## Поднять инфраструктуру, убедиться что есть топик и запустить API
	@true

dbshell: ## Открыть psql в контейнере БД как orders_user
	docker compose exec -it db_auth psql -U orders_user -d orders_db

kafsh: ## Шелл внутри контейнера Redpanda
	docker compose exec -it $(BROKER_SERVICE) bash

open-ui: # Открыть UI в стандартном браузере
	$(OPEN_CMD) "$(HTTP_BASE)/"

# Полный демо-цикл: снести -> поднять -> дождаться -> пересоздать топик -> засеять -> открыть браузер -> запустить API
reset-demo: down up wait-db wait-kafka topic-reset seed 
	( $(MAKE) -s wait-http && $(MAKE) -s open-ui ) & \
	set -a; source $(ENV_FILE); set +a; $(APP_CMD)

# =============================================================================
# Test & coverage
# =============================================================================
PKG ?= ./...   # можно переопределять: make cover PKG=./internal/cache

test: ##Запустить тесты
	@echo "$(BOLD)Running tests$(NC) for $(CYAN)$(PKG)$(NC)…"
	@go test -count=1 -v $(PKG)

test-race: ## Запустить тесты с -race
	@echo "$(BOLD)Running tests (-race)$(NC) for $(CYAN)$(PKG)$(NC)…"
	@go test -count=1 -race -v $(PKG)

cover: ## Посчитать покрытие и распечатать прогресс-бар
	@echo "$(BOLD)Computing coverage$(NC) for $(CYAN)$(PKG)$(NC)…"
	@go test -count=1 -covermode=atomic -coverprofile=coverage.out $(PKG) >/dev/null
	@PCT_RAW=$$(go tool cover -func=coverage.out | awk '/^total:/ {print $$3}'); \
	PCT=$$(printf "%s" "$$PCT_RAW" | tr -d '%'); \
	INT=$${PCT%.*}; \
	BAR_FILL=$$((INT/5)); \
	BAR=$$(printf "%0.s#" $$(seq 1 $$BAR_FILL)); \
	SPACE=$$(printf "%0.s." $$(seq 1 $$((20-BAR_FILL)))); \
	if [ $$INT -ge 90 ]; then COLOR="$(GREEN)"; MSG="excellent"; \
	elif [ $$INT -ge 60 ]; then COLOR="$(YELLOW)"; MSG="okay"; \
	else COLOR="$(RED)"; MSG="low"; fi; \
	printf "$(BOLD)Coverage$(NC): $(CYAN)%s%%%s [$$COLOR%s%s$(NC)]  %s\n" "$$PCT" "" "$$BAR" "$$SPACE" "$$MSG"

cover-html: ## Сгенерировать HTML-отчёт из coverage.out и открыть в браузере
	@test -f coverage.out || (echo "Run 'make cover' first" && exit 1)
	@go tool cover -html=coverage.out -o coverage.html
	@echo "HTML report: coverage.html"
	-@$(OPEN) coverage.html >/dev/null 2>&1 || true

# =============================================================================
# Lint & Format
# =============================================================================
lint: ## Прогнать golangci-lint (нужен установленный бинарник)
	@if ! [ -x "$(GOLANGCI)" ]; then \
		echo "golangci-lint не найден. Установи: make lint-install"; exit 1; \
	fi
	@"$(GOLANGCI)" run ./...

lint-install: ## Установить golangci-lint через go install
	@echo "Installing golangci-lint…"
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "Done. Перезапусти shell, если команда не находится."

fmt: ## Отформатировать код (gofmt -s -w)
	@gofmt -s -w .

fmt-check: ## Проверить форматирование (провалится, если есть нефоматированные файлы)
	@files="$$(gofmt -s -l .)"; \
	if [ -n "$$files" ]; then \
		echo "Нужно прогнать gofmt для файлов:"; echo "$$files"; exit 1; \
	fi

clean: ## Почистить артефакты сборки/покрытия и кеш тестов
	@rm -f coverage.out coverage.html repo.cov
	@go clean -testcache

clean-cover: ## Удалить только файлы покрытия
	@rm -f coverage.out coverage.html

# =============================================================================
# Tools: jq & deps
# =============================================================================
jq-check: ## Проверить, что jq установлен (иначе подсказать команду установки)
	@if ! command -v jq >/dev/null 2>&1; then \
		echo "jq не найден. Установи: make jq-install"; \
		exit 1; \
	fi
	@echo "jq: $$((jq --version) 2>/dev/null || echo 'ok')"

jq-install: ## Установить jq (macOS: brew; Linux: apt/dnf/yum/pacman/apk/zypper)
	@if command -v jq >/dev/null 2>&1; then \
		echo "jq уже установлен: $$(jq --version)"; \
		exit 0; \
	fi; \
	OS=$$(uname); \
	if [ "$$OS" = "Darwin" ]; then \
		if command -v brew >/dev/null 2>&1; then \
			echo "Installing jq via Homebrew…"; brew install jq; \
		else \
			echo "Homebrew не найден. Установи brew (https://brew.sh) и выполни: brew install jq"; exit 1; \
		fi; \
	else \
		if   command -v apt-get >/dev/null 2>&1; then echo "Installing jq via apt-get…"; sudo apt-get update && sudo apt-get install -y jq; \
		elif command -v dnf     >/dev/null 2>&1; then echo "Installing jq via dnf…";     sudo dnf install -y jq; \
		elif command -v yum     >/dev/null 2>&1; then echo "Installing jq via yum…";     sudo yum install -y jq; \
		elif command -v pacman  >/dev/null 2>&1; then echo "Installing jq via pacman…";  sudo pacman -Sy --noconfirm jq; \
		elif command -v apk     >/dev/null 2>&1; then echo "Installing jq via apk…";     sudo apk add --no-cache jq; \
		elif command -v zypper  >/dev/null 2>&1; then echo "Installing jq via zypper…";  sudo zypper install -y jq; \
		else \
			echo "Не удалось определить пакетный менеджер. Установи jq вручную: https://jqlang.github.io/jq/"; \
			exit 1; \
		fi; \
	fi

deps-install: ## Установить базовые тулзы для разработки (golangci-lint и jq)
	@$(MAKE) -s lint-install
	@$(MAKE) -s jq-install
