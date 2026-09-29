# FeedFlow

## Backend RSS-агрегатора на Go

FeedFlow — backend для автоматического сбора публикаций из RSS-лент. Пользователи добавляют источники, подписываются на них, получают персональную ленту публикаций и уведомления о новых записях через Telegram или email.

## Сервисы

| Приложение | Назначение |
| --- | --- |
| `cmd/gateway` | Единая точка входа на порту `8080`: маршрутизация запросов к двум API |
| `cmd/api` | Монолит на внутреннем порту `8082`: регистрация и JWT, каталог лент, подписки, публикации, RSS-воркер и кеш Redis |
| `cmd/notification-api` | HTTP API настроек уведомлений на внутреннем порту `8081`: JWT и отдельная БД |
| `cmd/outbox-relay` | Публикация основного и retry/DLQ outbox в Kafka |
| `cmd/notifier` | Kafka consumer, inbox и доставка через email/Telegram из отдельной БД |

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
cmd/gateway/        общий HTTP gateway
cmd/notification-api/ HTTP API настроек уведомлений
cmd/outbox-relay/   relay для обоих outbox
cmd/notifier/       приём событий и доставка
internal/           бизнес-логика и внутренние пакеты
contracts/          межсервисные контракты
deploy/             скрипты настройки инфраструктуры
.github/workflows/  CI/CD
docker-compose.yml  инфраструктура, миграции и профиль app
Dockerfile          сборка каждого HTTP-приложения
Taskfile.yml        команды разработки
```

## Запуск

Нужны Docker с Docker Compose и OpenSSL. Создайте `.env` с настройками PostgreSQL и JWT:

```dotenv
DB_USER=feedflow
DB_PASSWORD=feedflow_dev
DB_NAME=feedflow
JWT_KEY_ID=local-1
JWT_ISSUER=feedflow-auth
```

Один раз создайте ключи, затем запустите все сервисы:

```sh
sh deploy/auth/generate-dev-keys.sh
docker compose --profile app up --build -d
```

API доступен через gateway на `http://localhost:8080`. Для отправки уведомлений добавьте в `.env` `TELEGRAM_BOT_TOKEN` или `SMTP_HOST` и `SMTP_FROM_ADDRESS`.

### Каталог источников

`GET /v1/feeds` возвращает первую страницу источников как JSON-массив. Размер страницы по умолчанию — 50, параметр `limit` принимает значения от 1 до 100. Если есть следующая страница, ответ содержит заголовок `X-Next-Cursor`. Передайте его значение как `after` в следующем запросе, например `GET /v1/feeds?limit=50&after=<X-Next-Cursor>`. Когда заголовка нет, список закончился. Источники упорядочены по `id`; при параллельном добавлении новых источников список не является снимком базы на один момент времени.

Команды разработки из `Taskfile.yml`:

| Команда | Что делает |
| --- | --- |
| `task` | Показывает доступные команды |
| `task fmt` | Форматирует Go-код |
| `task fmt:check` | Проверяет форматирование без изменений |
| `task lint` | Запускает статический анализ |
| `task test` | Запускает все тесты |
| `task test:race` | Запускает тесты с race detector |
| `task build` | Собирает монолит в `bin/api` |
| `task run` | Запускает монолит локально |
| `task run:notification-api` | Запускает API уведомлений локально |
| `task run:gateway` | Запускает gateway локально |
| `task tidy` | Обновляет зависимости Go-модуля |
| `task check` | Проверяет форматирование, линтинг и тесты |

Для управления контейнерами:

| Команда | Что делает |
| --- | --- |
| `docker compose --profile app logs -f` | Показывает логи сервисов |
| `docker compose --profile app down` | Останавливает сервисы |

Отдельный пакет можно проверить напрямую:

```sh
go test ./internal/notification/worker
```
