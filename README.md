# Drizzy — dating bot

Telegram dating bot на микросервисной архитектуре с использованием Go. Ранжированная выдача профилей, PostgreSQL, Redis prefetch-очереди, RabbitMQ для event streaming и межсервисного взаимодействия, MinIO (S3) для фотографий, asynq для периодического пересчёта рейтингов.

## Документация

| Документ                                              | Описание                                                         |
| ----------------------------------------------------- | ---------------------------------------------------------------- |
| [stage-4-report.md](stage-4-report.md)                | Что добавлено на этапе 4 (метрики, CI/CD, JMeter, реф. бонус)    |
| [docs/services.md](docs/ru/services.md)               | Границы сервисов, зоны ответственности, паттерны коммуникации    |
| [docs/architecture.md](docs/ru/architecture.md)       | Диаграмма системы, RabbitMQ routing, Redis keys, каталог событий |
| [docs/database-schema.md](docs/ru/database-schema.md) | Таблицы PostgreSQL, индексы, ER-диаграмма                        |

## Роадмап

1. **Этап 1 — Планирование и проектирование**: сервисы, архитектура, схема БД, настройка репозитория.
2. **Этап 2 — Базовая функциональность**: Telegram bot service, регистрация по `/start`, REST Profile API.
3. **Этап 3 — Профили и ранжирование**: CRUD, алгоритм ранжирования (уровни 1–3), Redis prefetch, интеграция с RabbitMQ.
4. **Этап 4 — Доработка**: asynq-расписания, Prometheus-метрики, CI/CD (GitHub Actions), JMeter нагрузочное тестирование, реферальный бонус в ранжировании.

## Стек

- **Go** — все сервисы
- **Telegram Bot API** — `go-telegram-bot-api` или `telebot`
- **chi** — легковесный HTTP-роутер для Profile API (REST/JSON)
- **PostgreSQL** — основное хранилище данных
- **Redis** — кэш ранжированных списков кандидатов, состояние сессий
- **RabbitMQ** — event streaming, асинхронное межсервисное взаимодействие
- **MinIO** — S3-совместимое объектное хранилище для фотографий профилей
- **asynq** — распределённая очередь задач для Go (периодический пересчёт рейтингов)
- **Prometheus client_golang** — метрики HTTP-запросов и доменных событий, эндпоинты `/metrics` на каждом сервисе
- **GitHub Actions** — CI: `go vet`, `go test -race`, golangci-lint, Docker Buildx
- **Apache JMeter** — нагрузочное тестирование profile-service ([loadtest/](loadtest))

## Структура проекта

```
drizzy/
├── docker-compose.yml
├── Makefile
├── .github/workflows/ci.yml        # CI: build/test/lint + docker build
├── loadtest/                       # JMeter план + README
├── pkg/                            # общие Go-пакеты
│   ├── models/                     # общие доменные типы (JSON request/response structs)
│   ├── config/                     # парсинг переменных окружения
│   ├── metrics/                    # Prometheus middleware + доменные счётчики
│   ├── events/                     # envelope и типы AMQP-событий
│   └── rabbitmq/                   # хелперы publisher/consumer
├── bot-service/
│   ├── cmd/main.go
│   ├── internal/
│   │   ├── handler/                # обработчики Telegram-команд
│   │   ├── keyboard/               # построение inline-клавиатур
│   │   └── client/                 # HTTP-клиент к profile-service
│   └── Dockerfile
├── profile-service/
│   ├── cmd/main.go
│   ├── internal/
│   │   ├── handler/                # chi HTTP-хендлеры (REST endpoints)
│   │   ├── repository/             # запросы к PostgreSQL
│   │   └── storage/                # загрузка в MinIO / presigned URL
│   └── Dockerfile
├── ranking-service/
│   ├── cmd/main.go
│   ├── internal/
│   │   ├── consumer/               # RabbitMQ event consumer
│   │   ├── scorer/                 # реализация скоринга L1/L2/L3
│   │   ├── cache/                  # управление Redis sorted set / list
│   │   └── worker/                 # определения задач asynq
│   └── Dockerfile
├── migrations/                     # SQL-миграции (goose)
└── docs/
    ├── ru/                         # документация на русском
    ├── services.md
    ├── architecture.md
    └── database-schema.md
```

## Быстрый старт

```bash
cp .env.example .env   # выставить TELEGRAM_TOKEN
docker compose up --build
```

После запуска доступны:

| URL                             | Что                                       |
| ------------------------------- | ----------------------------------------- |
| `http://localhost:8080/healthz` | profile-service health                    |
| `http://localhost:8080/metrics` | profile-service Prometheus metrics        |
| `http://localhost:8081/healthz` | ranking-service health                    |
| `http://localhost:8081/metrics` | ranking-service Prometheus metrics        |
| `http://localhost:9090/metrics` | bot-service Prometheus metrics            |
| `http://localhost:9001`         | MinIO console                             |
| `http://localhost:15672`        | RabbitMQ management UI                    |
| `http://localhost:8888`         | asynqmon — мониторинг периодических задач |

## Тестирование

```bash
# Unit + race-детектор (то же, что делает CI)
go test ./... -race -count=1

# Нагрузочное (требует Apache JMeter ≥ 5.6)
jmeter -n -t loadtest/profile-service.jmx \
  -Jthreads=50 -Jrampup=30 -Jduration=120 \
  -l loadtest/results/run.jtl \
  -e -o loadtest/results/html
```
