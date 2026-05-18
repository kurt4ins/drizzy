# Features — drizzy

---

## 1. Алгоритм ранжирования

### Уровень 1 — первичный рейтинг

| Подпункт ТЗ                                  | Реализация                                                              | Файлы                                                                                                                                                                                                            |
| -------------------------------------------- | ----------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Возраст, пол, интересы, гео                  | колонки `profiles.age / gender / interests / city / latitude/longitude` | [migrations/001_init.sql](../migrations/001_init.sql), [pkg/models/models.go](../pkg/models/models.go)                                                                                                           |
| Полнота анкеты + кол-во фото                 | `profiles.completeness_score` пересчитывается при каждом write          | [profile-service/internal/repository/profile.go](../profile-service/internal/repository/profile.go), [profile-service/internal/handler/profile.go](../profile-service/internal/handler/profile.go)               |
| Первичные предпочтения (возраст, пол, город) | `user_preferences` + фильтрация в `TopCandidates`                       | [profile-service/internal/handler/preferences.go](../profile-service/internal/handler/preferences.go), [ranking-service/internal/repository/repository.go](../ranking-service/internal/repository/repository.go) |

### Уровень 2 — поведенческий рейтинг

| Подпункт ТЗ                      | Реализация                                                               | Файлы                                                                                                                                                                                                              |
| -------------------------------- | ------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Кол-во лайков анкеты             | `user_behavior_stats.likes_received` (инкремент в `UpdateBehaviorStats`) | [ranking-service/internal/repository/repository.go](../ranking-service/internal/repository/repository.go), [ranking-service/internal/consumer/interaction.go](../ranking-service/internal/consumer/interaction.go) |
| Соотношение лайков и пропусков   | формула `like_ratio · ln(1 + likes + skips)` в `RecalculateAllScores`    | [ranking-service/internal/repository/repository.go](../ranking-service/internal/repository/repository.go)                                                                                                          |
| Частота взаимных лайков (мэтчей) | `user_behavior_stats.matches_count` + инкремент в `CreateMatch`          | [ranking-service/internal/repository/repository.go](../ranking-service/internal/repository/repository.go)                                                                                                          |
| Частота инициирования диалогов   | колонка `user_behavior_stats.conversations_started` (схема готова)       | [migrations/001_init.sql](../migrations/001_init.sql)                                                                                                                                                              |

### Уровень 3 — комбинированный рейтинг

| Подпункт ТЗ                              | Реализация                                                                                             | Файлы                                                                                                                                                            |
| ---------------------------------------- | ------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Интеграция уровней 1+2 по весовой модели | `TopCandidates` ORDER BY `score + completeness_score + city_bonus`                                     | [ranking-service/internal/repository/repository.go](../ranking-service/internal/repository/repository.go)                                                        |
| Реферальная система                      | таблица `referrals` + бонус `+0.1 · COUNT(referrer)` в `RecalculateAllScores` (algorithm_version `v2`) | [migrations/001_init.sql](../migrations/001_init.sql), [ranking-service/internal/repository/repository.go](../ranking-service/internal/repository/repository.go) |

---

## 2. Redis

| Применение                                 | Описание                                                                     | Файлы                                                                                                                                                                              |
| ------------------------------------------ | ---------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Discovery queue                            | `LIST discovery:queue:{user_id}`, TTL 30 мин, `LPOP` ботом, `RPUSH` воркером | [bot-service/internal/discovery/queue.go](../bot-service/internal/discovery/queue.go), [ranking-service/internal/worker/ranking.go](../ranking-service/internal/worker/ranking.go) |
| Session store визарда                      | `HASH session:{user_id}`, TTL 10 мин                                         | [bot-service/internal/session/redis.go](../bot-service/internal/session/redis.go), [bot-service/internal/handler/session_keys.go](../bot-service/internal/handler/session_keys.go) |
| User store (tg_id ↔ user_id)               | кэш связи telegram ID ↔ внутренний UUID                                      | [bot-service/internal/userstore/userstore.go](../bot-service/internal/userstore/userstore.go)                                                                                      |
| Инвалидация очереди при смене предпочтений | `DEL discovery:queue:{user_id}`                                              | [profile-service/internal/handler/preferences.go](../profile-service/internal/handler/preferences.go)                                                                              |
| Asynq backend                              | переиспользует тот же Redis (как очередь задач)                              | [ranking-service/cmd/main.go](../ranking-service/cmd/main.go)                                                                                                                      |

---

## 3. Celery / asynq

| Применение                                                       | Файлы                                                                                       |
| ---------------------------------------------------------------- | ------------------------------------------------------------------------------------------- |
| Периодический пересчёт всех `user_ratings` (cron `*/15 * * * *`) | [ranking-service/internal/worker/ranking.go](../ranking-service/internal/worker/ranking.go) |
| Refill discovery-очередей по топ-10 кандидатов на юзера          | [ranking-service/internal/worker/ranking.go](../ranking-service/internal/worker/ranking.go) |
| Регистрация задачи `ranking:recalculate` + scheduler/server      | [ranking-service/cmd/main.go](../ranking-service/cmd/main.go)                               |
| Asynqmon UI на `:8888`                                           | [docker-compose.yml](../docker-compose.yml)                                                 |

---

## 4. MQ-брокер — RabbitMQ

| Применение                                                                              | Файлы                                                                                                   |
| --------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| Topic exchange `drizzy.events` + DLX `drizzy.dlx` (3 retry), хелперы Publisher/Consumer | [pkg/rabbitmq/rabbitmq.go](../pkg/rabbitmq/rabbitmq.go)                                                 |
| Envelope + типы событий (`interaction.*`, `match.created`, `like.received`)             | [pkg/events/events.go](../pkg/events/events.go)                                                         |
| Producer: бот публикует `interaction.liked` / `interaction.skipped`                     | [bot-service/internal/handler/browse.go](../bot-service/internal/handler/browse.go)                     |
| Consumer: ranking слушает `interaction.*`, детектит mutual like → `match.created`       | [ranking-service/internal/consumer/interaction.go](../ranking-service/internal/consumer/interaction.go) |
| Consumer: бот слушает `match.notify` и шлёт пуш в TG                                    | [bot-service/internal/handler/match.go](../bot-service/internal/handler/match.go)                       |
| Consumer: бот слушает `like.received` для уведомлений о лайке                           | [bot-service/internal/handler/like.go](../bot-service/internal/handler/like.go)                         |

---

## 5. Метрики и логирование

| Применение                                                                                        | Файлы                                                                                                                                                                                                                                                               |
| ------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Prometheus middleware + доменные счётчики (interactions, matches, recalc duration, queue refills) | [pkg/metrics/metrics.go](../pkg/metrics/metrics.go)                                                                                                                                                                                                                 |
| Подключение middleware + `/metrics` в profile-service                                             | [profile-service/cmd/main.go](../profile-service/cmd/main.go)                                                                                                                                                                                                       |
| `/metrics` route + observation в ranking                                                          | [ranking-service/cmd/main.go](../ranking-service/cmd/main.go), [ranking-service/internal/worker/ranking.go](../ranking-service/internal/worker/ranking.go), [ranking-service/internal/consumer/interaction.go](../ranking-service/internal/consumer/interaction.go) |
| Отдельный metrics HTTP-server `:9090` в bot-service                                               | [bot-service/cmd/main.go](../bot-service/cmd/main.go)                                                                                                                                                                                                               |
| Structured logging (chi `middleware.Logger`/`RequestID`) + AMQP `correlation_id`                  | [profile-service/cmd/main.go](../profile-service/cmd/main.go), [bot-service/cmd/main.go](../bot-service/cmd/main.go), [ranking-service/cmd/main.go](../ranking-service/cmd/main.go), [pkg/rabbitmq/rabbitmq.go](../pkg/rabbitmq/rabbitmq.go)                        |

---

## 6. S3-хранилище — MinIO

| Применение                                                                 | Файлы                                                                                     |
| -------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------- |
| MinIO-клиент, авто-создание bucket'а `drizzy-photos`, ping для healthcheck | [profile-service/internal/storage/minio.go](../profile-service/internal/storage/minio.go) |
| Presigned PUT URL для загрузки фото + GET primary photo                    | [profile-service/internal/handler/photo.go](../profile-service/internal/handler/photo.go) |
| Конфигурация контейнера + console на `:9001`                               | [docker-compose.yml](../docker-compose.yml)                                               |
| Подключение в init / DI                                                    | [profile-service/cmd/main.go](../profile-service/cmd/main.go)                             |

---

## 7. CI/CD

| Применение                                                                                                          | Файлы                                                   |
| ------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------- |
| GitHub Actions: `go vet`, `go test -race`, build, golangci-lint, Docker Buildx по 3 Dockerfile; на push/PR в `main` | [.github/workflows/ci.yml](../.github/workflows/ci.yml) |

---

## 8. match-notification сервис

| Применение                                                                                        | Файлы                                                                                                                                                                    |
| ------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Consumer `match.notify` в bot-service: подтягивает обе анкеты, шлёт пуш в TG с кнопкой «Написать» | [bot-service/internal/handler/match.go](../bot-service/internal/handler/match.go)                                                                                        |
| Параллельный consumer `like.received` для нотификаций о входящих лайках                           | [bot-service/internal/handler/like.go](../bot-service/internal/handler/like.go)                                                                                          |
| HTTP-клиенты к profile/ranking для подгрузки данных в нотификацию                                 | [bot-service/internal/client/profile.go](../bot-service/internal/client/profile.go), [bot-service/internal/client/ranking.go](../bot-service/internal/client/ranking.go) |

Полностью асинхронная цепочка: bot → RabbitMQ → ranking (detect match) → RabbitMQ → bot (notify).
