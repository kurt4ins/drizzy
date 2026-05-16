# Этап 4 Отчет

## 1. Prometheus-метрики во всех 3 сервисах (+1 → пункт «Метрики и логирование»)

**Файлы:**

- `pkg/metrics/metrics.go` — **новый пакет**. Содержит:
  - `Middleware(service string)` — chi/net/http middleware, считает
    `drizzy_http_requests_total{service,method,path,status}` и гистограмму
    `drizzy_http_request_duration_seconds`;
  - доменные счётчики: `drizzy_interactions_total{action}`,
    `drizzy_matches_total`, `drizzy_discovery_queue_refills_total`,
    `drizzy_ranking_recalc_duration_seconds`;
  - `Handler()` — `promhttp.Handler()` для `/metrics`.
- `profile-service/cmd/main.go` — подключён `metrics.Middleware("profile-service")`,
  добавлен маршрут `r.Handle("/metrics", metrics.Handler())`.
- `ranking-service/cmd/main.go` — добавлен `muxHTTP.Handle("/metrics", ...)`.
- `ranking-service/internal/worker/ranking.go` — фиксирует
  `RankingRecalcDuration` и `DiscoveryQueueRefills` в `RefillForUser`.
- `ranking-service/internal/consumer/interaction.go` — инкремент
  `InteractionsTotal` и `MatchesTotal` в обработчике события.
- `bot-service/cmd/main.go` — поднят отдельный HTTP-сервер на `:9090` с
  `/metrics` и `/healthz` (бот не имеет своего REST API, поэтому метрики
  выставлены через отдельный mux).
- `docker-compose.yml` — проброшен порт `9090:9090` для `bot-service` и
  переменная `METRICS_PORT=9090`.
- `go.mod` / `go.sum` — добавлена зависимость
  `github.com/prometheus/client_golang v1.23.x`.

После запуска `docker compose up` метрики доступны на:

- `http://localhost:8080/metrics` — profile-service
- `http://localhost:8081/metrics` — ranking-service
- `http://localhost:9090/metrics` — bot-service

## 2. Реферальный бонус в формуле рейтинга (закрывает уровень 3 → +1 к пункту «Рейтинг»)

**Файл:** `ranking-service/internal/repository/repository.go`,
функция `RecalculateAllScores`.

Алгоритм теперь покрывает все три уровня из ТЗ:

- **Уровень 1** (первичный): `completeness_score`, фильтры по
  возрасту/полу/городу — применяются в `TopCandidates` при формировании
  очереди.
- **Уровень 2** (поведенческий): `like_ratio · ln(1 + likes + skips)`.
- **Уровень 3** (комбинированный): `+ 0.1 · COUNT(referrals.referrer_id)` —
  пользователь получает бонус за каждого приглашённого.

`algorithm_version` поднят `v1 → v2`.

## 3. GitHub Actions CI/CD (+1 → пункт «CI/CD»)

**Файл:** `.github/workflows/ci.yml` — **новый**.

Три job-а:

- `build-test` — `go mod tidy` (проверка чистоты), `go vet`,
  `go build ./...`, `go test ./... -race`;
- `lint` — `golangci-lint` через официальный action;
- `docker-build` — собирает 3 образа (profile/ranking/bot) через Buildx
  без пуша. Гарантирует, что Dockerfile'ы остаются рабочими.

Срабатывает на `push` и `pull_request` в `main`.

## 4. JMeter — нагрузочное тестирование (+1 → пункт 9.6)

**Файлы (новые):**

- `loadtest/profile-service.jmx` — план для read-heavy эндпоинтов
  `profile-service` (`/healthz`, `/api/v1/users/{id}`, `/api/v1/profiles/{id}`).
  Параметризован через JMeter properties: `host`, `port`, `threads`, `rampup`,
  `duration`, `user_id`. Профили — smoke / baseline / stress.
- `loadtest/README.md` — инструкция запуска (`jmeter -n -t ...`) и связка с
  метриками: пороговое значение — p95 `drizzy_http_request_duration_seconds`
  > 250ms.

## 5. Минорные правки

- `go.mod` — версия `go 1.25.5`, добавлены `prometheus/client_golang` и
  транзитивные зависимости.

---

## Изменённые / новые файлы

```
.github/workflows/ci.yml                                 NEW
pkg/metrics/metrics.go                                   NEW
loadtest/profile-service.jmx                             NEW
loadtest/README.md                                       NEW
stage-4-report.md                                        NEW (этот файл)

profile-service/cmd/main.go                              MODIFIED (metrics middleware + /metrics)
ranking-service/cmd/main.go                              MODIFIED (/metrics route)
ranking-service/internal/worker/ranking.go               MODIFIED (recalc duration, refill counter)
ranking-service/internal/consumer/interaction.go         MODIFIED (interactions/matches counters)
ranking-service/internal/repository/repository.go        MODIFIED (referral bonus → score v2)
bot-service/cmd/main.go                                  MODIFIED (metrics HTTP server :9090)
docker-compose.yml                                       MODIFIED (bot-service port 9090, METRICS_PORT)
go.mod, go.sum                                           MODIFIED (prometheus/client_golang)
```
