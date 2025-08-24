# L0 — Orders Service

Небольшой сервис для приёма заказов из Kafka, сохранения их в PostgreSQL и отдачи через HTTP API + минимальный web-UI. В памяти держится кэш последних заказов для быстрых чтений.

## Содержание

- [Архитектура](#архитектура)
- [Быстрый старт](#быстрый-старт)
- [Конфигурация (ENV)](#конфигурация-env)
- [HTTP API](#http-api)
- [Kafka](#kafka)
- [База данных](#база-данных)
- [UI](#ui)
- [Тесты, покрытие, линтеры](#тесты-покрытие-линтеры)
- [Makefile — шпаргалка](#makefile--шпаргалка)
- [Структура проекта](#структура-проекта)
- [Траблшутинг](#траблшутинг)

---

## Архитектура

```
Kafka (Redpanda) ---> Consumer ----> Repo (Postgres) ----> HTTP API (/order/{id})
                           |                                  |
                           v                                  v
                        In-memory Cache <------------------- Web UI (/ui/)
```

- **Consumer** (`internal/kafka`) читает сообщения, валидирует JSON, делает upsert в БД и кладёт заказ в кэш. Коммитит оффсет только после успешной записи.
- **Repo** (`internal/repo`) — доступ к PostgreSQL, upsert батчем (orders, order_payment, order_delivery, items).
- **HTTP API** (`internal/httpapi`) — выдаёт заказ по `order_uid`. Сначала смотрит в кэш, затем в БД; успешные ответы кладёт в кэш.
- **Кэш** (`internal/cache`) — потокобезопасная map для заказов. На старте кэш «прогревается» последними `N` UID’ами.
- **UI** (`web/index.html`) — простая страница для поиска заказа, переключение Card/JSON вида.
- **Конфиг/DB** (`internal/config`, `internal/db`) — загрузка ENV, создание пула, ping.

---

## Быстрый старт
### Самый быстрый способ (всё одной командой)

```bash
make reset-demo
```
Что делает эта цель:
- Останавливает и заново поднимает Postgres и Redpanda (`down` → `up`), ждёт их готовности.
- Пересоздаёт топик Kafka и **сеет демо-сообщение** (`topic-reset` + `seed`).
- Параллельно ждёт HTTP-сервис и **открывает UI в браузере**.
- Запускает приложение `go run ./cmd/api` с переменными из `.env`/`.env.example`.

Требуется: **Docker** + **Docker Compose**, **Go**, **make**, **jq**.

### Альтернатива — пошагово

1) Скопируйте пример окружения (не обязательно — `make run` сам подхватит `.env.example`):
```bash
cp .env.example .env
# при необходимости отредактируйте POSTGRES_DSN и т.п.
```

2) Поднимите инфраструктуру и запустите API:
```bash
make dev
# = docker compose up db_auth redpanda + topic create + go run ./cmd/api
```

3) Проверьте health:
```bash
curl -i http://localhost:8081/healthz
```

4) Откройте UI:
```bash
make open-ui
# или вручную: http://localhost:8081/ui/
```

5) Засейте демо-сообщение в Kafka (используется `fixtures/model.json`):
```bash
make seed
# или явно: FILE=fixtures/model.json make seed
```

6) Получите заказ по `order_uid`:
```bash
curl http://localhost:8081/order/b563feb7b2b84b6test
```

Остановить всё и почистить:
```bash
make down
```

---

## Конфигурация (ENV)

| Переменная        | По умолчанию             | Назначение                                  |
|-------------------|--------------------------|---------------------------------------------|
| `HTTP_ADDR`       | `:8081`                  | Адрес HTTP сервера                          |
| `POSTGRES_DSN`    | — (**обязателен**)       | DSN для подключения к Postgres              |
| `CACHE_WARM_LIMIT`| `100`                    | Сколько последних UID прогреть в кэш        |
| `KAFKA_BROKERS`   | `localhost:9092`         | Адрес(а) брокеров Kafka/Redpanda           |
| `KAFKA_TOPIC`     | `orders`                 | Топик                                       |
| `KAFKA_GROUP`     | `orders-consumer`        | Группа потребителей                         |

`.env.example` содержит рабочие значения для docker-окружения:
```
HTTP_ADDR=:8081
POSTGRES_DSN=postgres://orders_user:orders_pass@localhost:5432/orders_db?sslmode=disable
CACHE_WARM_LIMIT=100
KAFKA_BROKERS=localhost:9092
KAFKA_TOPIC=orders
KAFKA_GROUP=orders-consumer
```

---

## HTTP API

### `GET /healthz`
- **200 OK** — `{"status":"ok","cache_size":N,"version":"<ver>","request_id":"..."}`
- Поддерживает `HEAD`.
- Можно передать свой `X-Request-ID` — он вернётся в ответе.

### `GET /order/{order_uid}`
- Возвращает объединённый объект заказа:
  - `order`, `payment`, `delivery`, `items[]`.
- Коды ошибок:
  - **400** `{"error":"bad_request"}` — пустой/слишком длинный UID.
  - **404** `{"error":"not_found","message":"order not found"}` — нет в БД.
  - **500** `{"error":"internal"}` — прочие ошибки.
- Кэш:
  - Если заказ уже в кэше — запрос обслуживается из памяти.
  - При удачной загрузке из БД — заказ добавляется в кэш.

### Примеры

```bash
# Удача
curl -s http://localhost:8081/order/b563feb7b2b84b6test | jq .

# 404
curl -s -i http://localhost:8081/order/not_exists
```

### Редирект и статические файлы
- `/` → **307** на `/ui/`
- `/ui/` — статический интерфейс (см. раздел UI).

---

## Kafka

**Consumer** (`internal/kafka`):
- `MinBytes=1`, `MaxBytes=10MB`, `CommitInterval=0` (коммит вручную).
- Парсит JSON, валидирует поля (`order_uid`, `track_number`, `currency`, `amount>=0`).
- При **ошибке upsert** — логирует и делает backoff `~300ms + jitter` (без коммита).
- При **ошибочном JSON** или **невалидных данных** — логирует и **коммитит** (чтобы не зациклить).
- Предупреждает в логах при mismatch `key != payload.order_uid`.

Команды (используют `rpk` внутри контейнера Redpanda):
```bash
make topic         # создать топик (если нет)
make topic-list    # показать топики
make topic-reset   # пересоздать топик
make seed          # отправить JSON из fixtures/model.json (key = order_uid)
make consume N=1   # прочитать N сообщений
```

Скрипт для отправки произвольного файла:
```bash
./scripts/produce.sh fixtures/model.json           # key возьмётся из "order_uid"
./scripts/produce.sh fixtures/model.json my-key    # задать key явно
```

---

## База данных

Схема создаётся автоматически контейнером `postgres` из `db/init/` (DDL выдержка):

- Таблицы:
  - `orders(order_uid PK, ... , date_created, ...)`
  - `order_payment(order_uid PK FK->orders, ...)`
  - `order_delivery(order_uid PK FK->orders, ...)`
  - `order_items(id PK, order_uid FK->orders, ...)`
- Индексы:
  - `idx_order_items_order_uid`
  - `idx_orders_date_created (DESC)`
- Пользователь/права: создаётся роль `orders_user`, ей отдаются БД и схема.

**Upsert** выполняется батчем в транзакции: `orders` → `order_payment` → `order_delivery` → `DELETE order_items` → `INSERT items*`. При ошибках — rollback.

---

## UI

Статический клиент (`web/index.html`) доступен по `http://localhost:8081/ui/`:
- Поле ввода `order_uid`, кнопка `Find`, переключатель представлений **Card/JSON**.
- Кнопка **Copy JSON**.
- Красивые таблицы `Order / Delivery / Payment / Items`.

---

## Тесты, покрытие, линтеры

Запуск:
```bash
make test        # go test -count=1 -v ./...
make test-race   # с -race
make cover       # считает покрытие и рисует бар
make cover-html  # html-отчёт (после cover)
```
Линтеры/форматирование:
```bash
make lint-install  # установить golangci-lint
make lint          # прогнать линтер
make fmt           # gofmt -s -w .
make fmt-check     # проверить форматирование
```

---

## Makefile — шпаргалка

Базовое:
- `make help` — показать доступные цели с короткими описаниями.


Инфраструктура:
- `make up` / `make down` — поднять/остановить Postgres и Redpanda.
- `make ps`, `make logs` — статус/логи.
- `make wait-db`, `make wait-kafka`, `make wait-http` — подождать готовность сервисов.

Приложение:
- `make run` — `go run ./cmd/api` с подхватом ENV.
- `make dev` — `up + topic + run`.
- `make dbshell` — открыть psql внутри контейнера БД под `orders_user`.
- `make kafsh` — shell внутри контейнера Redpanda.
- `make open-ui` — открыть UI в браузере.
- `make reset-demo` — полный демо-цикл (пересоздать топик, засеять, открыть UI, запустить API).
- `make open-ui` — открыть UI в браузере
- `make reset-demo` — полный демо-цикл: снести/поднять инфру, дождаться сервисов, пересоздать топик, **засеять** сообщение, дождаться HTTP и **открыть UI**, затем запустить API.


Kafka:
- `make topic`, `make topic-list`, `make topic-reset`, `make seed`, `make consume`.

Инструменты:
- `make deps-install` — установить `golangci-lint` и `jq`.
- `make jq-check` — проверить наличие `jq` (подскажет, если не установлен).
- `make jq-install` — установить `jq` (подбирает пакетный менеджер).
- `make clean`, `make clean-cover` — очистка артефактов.

Переменные по умолчанию:
```
TOPIC=orders
BROKER_SERVICE=redpanda
FILE=fixtures/model.json
HTTP_BASE=http://localhost:8081
```

---

## Структура проекта

```
cmd/api/                 # main()
internal/
  cache/                 # in-memory кэш (Get/Set/Delete/Len)
  config/                # загрузка ENV
  db/                    # pgx pool, ping
  httpapi/               # маршруты, middleware (X-Request-ID), JSON-ответы
  kafka/                 # consumer, валидация, backoff, коммиты
  repo/                  # SQL, upsert батчем, выборки
  respond/               # JSON-утилиты для ответов/ошибок
db/init/                 # SQL-инициализация Postgres
fixtures/model.json      # пример заказа для Kafka
scripts/produce.sh       # отправка JSON в Kafka через rpk
web/index.html           # статический UI
docker-compose.yml       # Postgres + Redpanda
.env.example             # пример окружения
Makefile                 # команды для разработки
```

---

## Траблшутинг

- **`POSTGRES_DSN` пустой**  
  `config.Load()` вернёт ошибку. Проверьте `.env` или задайте переменную окружения.

- **API не может подключиться к БД**  
  Убедитесь, что `docker compose up -d db_auth` выполнен, и DSN указывает на `localhost:5432`.  
  `make wait-db` помогает дождаться готовности.

- **Consumer не видит Kafka / нет топика**  
  Поднимите Redpanda: `docker compose up -d redpanda`, создайте топик: `make topic`, проверьте `make topic-list`.

- **`make seed` ругается на `jq`**  
  Установите `jq`: `make jq-install` или поставьте вручную.

- **404 при запросе заказа**  
  Значит такого `order_uid` нет в БД. Отправьте пример: `make seed`, затем повторите запрос.

- **Хочется версионировать билд**  
  Переменная `version` в `main.go` задаётся через ldflags:  
  `go run -ldflags "-X main.version=1.2.3" ./cmd/api`
