# Сервисы

Три Go-микросервиса + общая инфраструктура. Взаимодействие через HTTP/JSON (синхронно) и RabbitMQ (асинхронно). Общий инстанс PostgreSQL.

## Bot service

- Long-polling / webhook к Telegram Bot API через `go-telegram-bot-api`.
- Обрабатывает `/start`, wizard регистрации, показ карточек профилей (inline-кнопки like/skip).
- Вызывает Profile service по **HTTP/JSON** для чтения/записи профилей.
- Вызывает Ranking service по **HTTP/JSON** (`POST /internal/queue/refill`) для мгновенного заполнения очереди кандидатов при первом `/browse`.
- Читает ранжированные списки кандидатов из **Redis** (`LPOP`) для минимальной задержки; если очередь пуста — сразу запрашивает refill у Ranking service и делает повторный LPOP.
- Публикует события (`interaction.liked`, `interaction.skipped`) в **RabbitMQ**.
- Потребляет `match.created` из RabbitMQ → отправляет персональное уведомление с именем, возрастом, городом и ID анкеты совпавшего пользователя.
- Потребляет `like.received` из RabbitMQ → отправляет уведомление «💛 Кто-то лайкнул твою анкету!» пользователю ещё до мэтча.
- **Не владеет** таблицами PostgreSQL напрямую.

## Profile service (REST — chi)

- Владелец таблиц `users`, `profiles`, `profile_photos`, `user_preferences`.
- REST API через `go-chi/chi`, JSON request/response.

| Метод  | Путь                              | Описание                                |
| ------ | --------------------------------- | --------------------------------------- |
| POST   | `/api/v1/users`                   | Регистрация (по Telegram ID)            |
| GET    | `/api/v1/users/{user_id}`         | Получение пользователя                  |
| GET    | `/api/v1/profiles/{user_id}`      | Профиль с фотографиями                  |
| PUT    | `/api/v1/profiles/{user_id}`      | Обновление профиля                      |
| PUT    | `/api/v1/preferences/{user_id}`   | Установка предпочтений                  |
| POST   | `/api/v1/photos/{user_id}/upload` | Presigned MinIO upload URL              |
| POST   | `/api/v1/photos/{user_id}/confirm`| Подтверждение загрузки фото             |
| GET    | `/api/v1/discovery/{user_id}/next`| Следующий кандидат                      |

- Discovery endpoint: сначала Redis prefetch-очередь, при miss — fallback на ranking pipeline.
- Генерирует presigned PUT URL для MinIO; сохраняет `s3_key` в БД после подтверждения.
- Вычисляет `completeness_score` при каждой записи в профиль.
- **Не публикует** в RabbitMQ — чистый read/write API.

## Ranking service

- **Consumer**: подписан на очередь `behavior.aggregate` (routing keys `interaction.*`). Обновляет `user_behavior_stats` в PostgreSQL. При обнаружении взаимного лайка создаёт запись в `matches` и публикует `match.created`. При одностороннем лайке публикует `like.received` (уведомление без раскрытия личности).
- **Scorer**: алгоритм v1 — `score = like_ratio × ln(1 + total_interactions)`. Пишется в `user_ratings` asynq-воркером.
- **Worker** (asynq): периодический пересчёт `user_ratings` каждую минуту (dev) / 15 мин (prod).
- **Cache manager**: после пересчёта пушит top ~10 candidate ID в Redis-списки (TTL 30 мин).
- **HTTP API** (порт 8081): `GET /healthz`, `POST /internal/queue/refill` — on-demand заполнение очереди кандидатов для конкретного пользователя.

## Media (MinIO)

- S3-совместимое хранилище для фотографий.
- Bucket: `profile-photos`, ключ: `{user_id}/{photo_id}.jpg`.
- Profile service генерирует presigned URL; бот загружает напрямую.

## Observability

- JSON-логирование с пробросом `request_id` (HTTP `X-Request-ID` → RabbitMQ `correlation_id`).
- Health endpoints: `GET /healthz` на каждом сервисе.
- RabbitMQ management UI на порту 15672.

## Связанные документы

- [architecture.md](architecture.md) — диаграмма системы, RabbitMQ routing, Redis keys, каталог событий.
- [database-schema.md](database-schema.md) — таблицы PostgreSQL и индексы.
