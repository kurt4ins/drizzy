# Архитектура

Полный дизайн dating bot: Telegram-клиент, три Go-микросервиса (Bot, Profile, Ranking), PostgreSQL, Redis prefetch, RabbitMQ event streaming, MinIO для медиа, asynq для периодического пересчёта рейтингов.

## Диаграмма верхнего уровня

```mermaid
flowchart LR
  subgraph clients [Клиенты]
    TG[Telegram]
  end
  subgraph services [Go микросервисы]
    Bot[Bot_service]
    Prof[Profile_service_REST]
    Rank[Ranking_service]
  end
  subgraph data [Data plane]
    PG[(PostgreSQL)]
    RD[(Redis)]
    S3[(MinIO)]
  end
  subgraph async [Async plane]
    MQ[RabbitMQ]
    AQ[asynq_workers]
  end
  TG <-->|Bot API| Bot
  Bot -->|HTTP/JSON| Prof
  Bot -->|LPOP| RD
  Bot -->|publish events| MQ
  Prof --> PG
  Prof --> S3
  Prof -->|read| RD
  MQ -->|consume| Rank
  Rank --> PG
  Rank -->|RPUSH candidates| RD
  AQ -->|scheduled recompute| PG
  AQ --> RD
  MQ -->|match.created| Bot
```

## Паттерны коммуникации

| Откуда → Куда          | Протокол  | Паттерн      | Назначение                                    |
| ----------------------- | --------- | ------------ | --------------------------------------------- |
| Bot → Profile           | HTTP/JSON | Синхронный   | Чтение/запись профилей, регистрация, фото      |
| Bot → RabbitMQ          | AMQP      | Fire-and-forget | Публикация событий взаимодействия           |
| RabbitMQ → Ranking      | AMQP      | Consumer     | Обновление поведенческой статистики            |
| RabbitMQ → Bot          | AMQP      | Consumer     | Доставка уведомлений о мэтчах                 |
| Ranking → Redis         | Redis     | Write        | Запись предварительно ранжированных списков     |
| Bot → Redis             | Redis     | Read         | Извлечение следующего кандидата из prefetch-очереди |
| Profile → Redis         | Redis     | Read         | Fallback при cache miss                        |
| asynq → PostgreSQL      | SQL       | По расписанию| Периодический пересчёт рейтингов               |

## RabbitMQ routing

- **Producer:** Bot service — публикует события после нажатия пользователем like/skip.
- **Exchange:** `drizzy.events` — тип **topic**, durable.
- **Routing keys:** `interaction.liked`, `interaction.skipped`, `match.created`.
- **Очереди:**

| Очередь                  | Binding key            | Consumer        | Назначение                          |
| ------------------------ | ---------------------- | --------------- | ----------------------------------- |
| `behavior.aggregate`     | `interaction.*`        | Ranking service | Обновление `user_behavior_stats`    |
| `match.notify`           | `match.created`        | Bot service     | Отправка уведомлений о мэтче в TG  |
| `behavior.aggregate.dlq` | —                      | Ручная проверка | Dead-letter после макс. ретраев    |
| `match.notify.dlq`       | —                      | Ручная проверка | Dead-letter после макс. ретраев    |

- **Durability:** durable exchange, durable queues, persistent messages для всех событий взаимодействия.
- **Обработка ошибок:** у каждой очереди есть dead-letter exchange (`drizzy.dlx`), направляющий упавшие сообщения в соответствующую `.dlq`-очередь после 3 ретраев. Poison messages проверяются вручную через RabbitMQ management UI.

```mermaid
flowchart LR
  subgraph producers [Producers]
    Bot[Bot_service]
  end
  subgraph rmq [RabbitMQ]
    EX["drizzy.events (topic)"]
    Q1[behavior.aggregate]
    Q2[match.notify]
    DLQ1[behavior.aggregate.dlq]
    DLQ2[match.notify.dlq]
  end
  subgraph consumers [Consumers]
    Rank[Ranking_service]
    BotC[Bot_service]
  end
  Bot --> EX
  EX -->|"interaction.*"| Q1
  EX -->|"match.created"| Q2
  Q1 --> Rank
  Q2 --> BotC
  Q1 -.->|failed| DLQ1
  Q2 -.->|failed| DLQ2
```

## Логика определения мэтча

Определение мэтча происходит в consumer'е Ranking service. При обработке события `interaction.liked`:

1. Проверяется наличие взаимного лайка: `SELECT 1 FROM interactions WHERE actor_user_id = target AND target_user_id = actor AND action_type = 'like'`.
2. Если да → вставка в таблицу `matches` (каноничный порядок: `user_a_id = LEAST(a, b)`, `user_b_id = GREATEST(a, b)`).
3. Публикация события `match.created` в RabbitMQ → consumer `match.notify` в Bot service подхватывает его и отправляет Telegram-уведомления обоим пользователям.

## Discovery и Redis prefetch

Bot service делает `LPOP` следующего candidate ID из per-viewer Redis **list**; если список пуст или содержит ≤ 2 записи, Ranking service получает триггер (через asynq task или синхронный fallback через Profile service) для вычисления и `RPUSH` следующих ~10 candidate ID.

```mermaid
sequenceDiagram
  participant U as Пользователь
  participant Bot as Bot_service
  participant R as Redis
  participant Prof as Profile_service
  participant Rank as Ranking_service
  U->>Bot: свайп / открытие сессии
  Bot->>R: LPOP discovery:queue:{user_id}
  alt очередь пуста или мало записей
    Bot->>Prof: GET /api/v1/discovery/{user_id}/next
    Prof->>Rank: trigger refill (asynq task)
    Rank->>R: RPUSH ~10 candidate ID
    Rank-->>Prof: первый candidate ID
    Prof-->>Bot: JSON profile payload
  else в очереди есть записи
    Bot->>Prof: GET /api/v1/profiles/{candidate_id}
    Prof-->>Bot: JSON profile payload
  end
  Bot-->>U: показ карточки профиля
```

### Конвенции Redis-ключей

| Паттерн ключа                        | Тип  | TTL     | Назначение                                                       |
| ------------------------------------ | ---- | ------- | ---------------------------------------------------------------- |
| `discovery:queue:{viewer_user_id}`   | LIST | 15–30м  | FIFO следующих `profile_id`-кандидатов. DEL при смене предпочтений. |
| `session:{viewer_user_id}`           | HASH | 10м     | Краткоживущее состояние wizard-регистрации. DEL при завершении/отмене. |

Discovery queue = следующие карточки к показу. Session = состояние state machine бота. Смена предпочтений → инвалидация только discovery queue (DEL ключа, следующий запрос запускает refill).

## Каталог событий (RabbitMQ payloads)

Формат конверта (JSON, UTF-8):

```json
{
  "event_id": "uuid-v4",
  "type": "interaction.liked",
  "occurred_at": "2026-03-23T12:00:00Z",
  "schema_version": 1,
  "payload": {}
}
```

| Тип                   | Поля payload                                      |
| --------------------- | ------------------------------------------------- |
| `interaction.liked`   | `actor_user_id`, `target_user_id`, `interaction_id` |
| `interaction.skipped` | `actor_user_id`, `target_user_id`, `interaction_id` |
| `match.created`       | `match_id`, `user_a_id`, `user_b_id`              |

Все ID — `bigint` (PostgreSQL serial). Timestamps в формате RFC 3339 UTC.

## Фоновые задачи (asynq)

asynq заменяет Celery в экосистеме Go. Использует Redis как брокер (тот же инстанс Redis, другой DB number или key prefix).

| Задача                   | Расписание     | Описание                                              |
| ------------------------ | -------------- | ----------------------------------------------------- |
| `rating:recompute_all`   | Каждые 15 мин  | Пересчёт `user_ratings` для всех активных пользователей |
| `discovery:refill`       | По запросу     | Генерация и push списка кандидатов для конкретного пользователя |
| `stats:reconcile`        | Каждый 1 час   | Сверка `user_behavior_stats` с сырыми interactions     |

## Docker Compose 

```yaml
services:
  postgres:
    image: postgres:16-alpine
    ports: ["5432:5432"]
    environment:
      POSTGRES_DB: drizzy
      POSTGRES_USER: drizzy
      POSTGRES_PASSWORD: drizzy

  redis:
    image: redis:7-alpine
    ports: ["6379:6379"]

  rabbitmq:
    image: rabbitmq:3-management-alpine
    ports: ["5672:5672", "15672:15672"]
    environment:
      RABBITMQ_DEFAULT_USER: drizzy
      RABBITMQ_DEFAULT_PASS: drizzy

  minio:
    image: minio/minio:latest
    ports: ["9000:9000", "9001:9001"]
    command: server /data --console-address ":9001"
    environment:
      MINIO_ROOT_USER: minioadmin
      MINIO_ROOT_PASSWORD: minioadmin

  bot-service:
    build: ./bot-service
    depends_on: [rabbitmq, redis]
    environment:
      TELEGRAM_TOKEN: ${TELEGRAM_TOKEN}
      PROFILE_API_URL: http://profile-service:8080
      REDIS_ADDR: redis:6379
      RABBITMQ_URL: amqp://drizzy:drizzy@rabbitmq:5672/

  profile-service:
    build: ./profile-service
    depends_on: [postgres, redis, minio]
    ports: ["8080:8080"]
    environment:
      POSTGRES_DSN: postgres://drizzy:drizzy@postgres:5432/drizzy?sslmode=disable
      REDIS_ADDR: redis:6379
      MINIO_ENDPOINT: minio:9000
      MINIO_ACCESS_KEY: minioadmin
      MINIO_SECRET_KEY: minioadmin

  ranking-service:
    build: ./ranking-service
    depends_on: [postgres, redis, rabbitmq]
    environment:
      POSTGRES_DSN: postgres://drizzy:drizzy@postgres:5432/drizzy?sslmode=disable
      REDIS_ADDR: redis:6379
      RABBITMQ_URL: amqp://drizzy:drizzy@rabbitmq:5672/
```

## Observability

- **HTTP (chi)**: middleware логирования запросов, гистограммы задержек, процент ошибок.
- **RabbitMQ**: глубина очередей, утилизация consumers, процент DLQ (management UI на :15672).
- **asynq**: счётчики успехов/ошибок задач, задержки, количество ретраев (дашборд asynqmon).
- **Redis**: потребление памяти, evictions, hit ratio для discovery-ключей.
- **Все сервисы**: структурированное JSON-логирование с пробросом `request_id` через HTTP-заголовок `X-Request-ID` и RabbitMQ message header `correlation_id`.

## Связанные документы

- [services.md](services.md) — зоны ответственности сервисов.
- [database-schema.md](database-schema.md) — таблицы PostgreSQL и индексы.
