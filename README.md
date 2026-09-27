# FeedFlow

## Backend RSS-агрегатора на Go

FeedFlow — backend для сбора публикаций из RSS-лент. Пользователи добавляют источники, подписываются на них, получают персональную ленту публикаций и уведомления о новых записях через Telegram или email.

Проект находится в активной разработке. Сейчас он состоит из монолита и отдельного сервиса доставки уведомлений с общей PostgreSQL. Подготовлены инфраструктура и контракты для перехода на Kafka; публикация и потребление Kafka-событий ещё не реализованы.

## Бизнес-логика

- Регистрация выдаёт пользователю API key. Он обменивается на короткоживущий JWT для доступа к защищённым маршрутам.
- Каталог RSS-лент общий для всех пользователей. Пользователь добавляет источник и отдельно оформляет подписку на него.
- RSS-воркер сразу после запуска, затем каждую минуту выбирает до трёх давно не обновлявшихся источников и загружает их параллельно. Публикации сохраняются с защитой от дублей по URL.
- Персональная лента содержит публикации из подписок, отсортированные от новых к старым, с пагинацией через `limit` и `offset`. Redis кеширует публикации на минуту, каталог источников — на час.
- Пользователь создаёт, включает, отключает и удаляет свои каналы уведомлений: Telegram chat ID или email.
- Сохранение новой публикации атомарно создаёт событие `post.created` в transactional outbox. Сервис нотификаций читает его из PostgreSQL и создаёт задания доставки по активным каналам подписчиков. Временные ошибки повторяются с растущей задержкой; зависшие задания возвращаются в очередь. Уникальность задания по пользователю, публикации и каналу защищает от повторного создания доставки.

## Сервисы

| Приложение | Назначение |
| --- | --- |
| `cmd/api` | Монолит: HTTP API на порту `8080`, регистрация и JWT, каталог лент, подписки, публикации, RSS-воркер и кеш Redis |
| `cmd/notifier` | Сервис нотификаций: обработка outbox и заданий доставки в PostgreSQL, отправка через Telegram Bot API и SMTP, повторные попытки |

HTTP API каналов выделен в `internal/notification/httpapi`, но пока подключён к серверу монолита. `cmd/notifier` работает как фоновый воркер без HTTP-сервера. Собственная БД нотификаций и общий API Gateway — следующие этапы разделения.

## Стек

- Go
- PostgreSQL, Redis
- Kafka
- JWT
- Docker, Docker Compose
- GitHub Actions

## Структура

```text
cmd/api/            монолит
cmd/notifier/       сервис нотификаций
internal/           бизнес-логика и внутренние пакеты
contracts/          межсервисные контракты
deploy/             скрипты настройки инфраструктуры
.github/workflows/  CI/CD
docker-compose.yml  инфраструктура и миграции
Dockerfile          сборка монолита
Taskfile.yml        команды разработки
```

## Запуск

Для локального запуска нужны Go 1.25, Docker с Docker Compose и OpenSSL. Для команд `task` дополнительно нужен Task; для форматирования и линтинга — `golangci-lint` v2. Все команды ниже выполняются из корня проекта.

### 1. Настроить окружение

Создайте `.env` или дополните существующий файл настройками для локальной разработки:

```dotenv
DB_USER=feedflow
DB_PASSWORD=feedflow_dev
DB_NAME=feedflow
DB_HOST=localhost
DB_PORT=5432

REDIS_NAME=feedflow_redis
REDIS_HOST=localhost
REDIS_PORT=6379

JWT_PRIVATE_KEY_PATH=.secrets/jwt/private.pem
JWT_PUBLIC_KEY_PATH=.secrets/jwt/public.pem
JWT_KEY_ID=local-1
JWT_ISSUER=feedflow-auth
JWT_ACCESS_TOKEN_TTL=15m
```

Один раз сгенерируйте пару JWT-ключей:

```sh
sh deploy/auth/generate-dev-keys.sh
```

### 2. Поднять инфраструктуру и применить миграции

```sh
docker compose up -d db redis
docker compose run --rm migrations
```

### 3. Запустить монолит

```sh
go run ./cmd/api
```

API доступен по адресу `http://localhost:8080`. Проверить публичный каталог источников можно командой:

```sh
curl http://localhost:8080/v1/feeds
```

Для защищённых запросов зарегистрируйтесь через `POST /v1/users` с JSON `{"name":"Alice"}`, затем обменяйте полученный `api_key` на JWT:

```sh
curl -X POST http://localhost:8080/v1/auth/token \
  -H 'Authorization: ApiKey <api_key>'
```

Полученный `access_token` передавайте как `Authorization: Bearer <access_token>`. Основные маршруты: `POST /v1/feeds`, `POST /v1/feed_follows` с JSON `{"feedId":"<uuid>"}`, `GET /v1/posts?limit=10&offset=0` и `/v1/notification-channel` для настройки уведомлений.

### 4. Запустить сервис нотификаций

Добавьте в `.env` настройки хотя бы одного отправителя:

| Канал | Настройки |
| --- | --- |
| Telegram | `TELEGRAM_BOT_TOKEN`; при создании пользовательского канала в `destination` укажите chat ID |
| Email | `SMTP_HOST`, `SMTP_FROM_ADDRESS`; при необходимости авторизации — одновременно `SMTP_USERNAME` и `SMTP_PASSWORD` |

Дополнительные SMTP-параметры: `SMTP_PORT` (по умолчанию `587`), `SMTP_TLS_MODE` (`starttls` по умолчанию, также `implicit` или `none`), `SMTP_TIMEOUT` (по умолчанию `10s`) и `SMTP_FROM_NAME`.

В отдельном терминале выполните:

```sh
go run ./cmd/notifier
```

### Команды разработки

Основные команды вынесены в `Taskfile.yml`:

| Команда | Что делает |
| --- | --- |
| `task` | Показывает доступные команды |
| `task run` | Запускает монолит локально |
| `task build` | Собирает монолит в `bin/api` |
| `task test` | Запускает тесты всех пакетов |
| `task test:race` | Запускает тесты с race detector |
| `task fmt` | Форматирует Go-код |
| `task lint` | Запускает статический анализ |
| `task check` | Проверяет форматирование, линтинг и тесты |
| `docker compose logs -f` | Показывает логи инфраструктуры |
| `docker compose down` | Останавливает инфраструктуру с сохранением данных в volumes |

Без Task тесты запускаются напрямую, в том числе для отдельного пакета:

```sh
go test ./...
go test ./internal/notification/worker
```

### Опциональная Kafka

```sh
docker compose --profile kafka up -d kafka kafka-init
```

Брокер доступен на `localhost:9092`; `kafka-init` создаёт основной топик, три retry-топика и DLQ. Монолит уже сохраняет `notification.requested` в outbox для каждого подписчика новой публикации, но relay и consumer ещё не реализованы. Текущая доставка продолжает работать через PostgreSQL и `post.created`; Kafka для неё не нужна.

