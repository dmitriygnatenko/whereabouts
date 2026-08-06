# Где·Что — бэкенд (Go + MySQL/PostgreSQL/SQLite)

Бэкенд на чистой стандартной библиотеке Go (`net/http`, Go 1.26+) плюс
драйверы БД. Поддерживает три СУБД, выбираются переменной окружения
`DB_DRIVER`:

- `mysql` (по умолчанию) — через `github.com/go-sql-driver/mysql`, работает
  и с MySQL, и с MariaDB;
- `postgres` — через `github.com/jackc/pgx/v5/stdlib`;
- `sqlite` — через `modernc.org/sqlite` (чистый Go, без cgo), файл БД на
  диске, сервер не нужен.

Фронтенд (тот же Vue-файл из `/mnt/user-data/outputs`) вшит в бинарник и
отдаётся тем же сервером — открываете один URL, и всё работает без
CORS-плясок.

## Структура

Бэкенд собран по гексагональной архитектуре (порты и адаптеры) с
явными юзкейсами — бизнес-логика ничего не знает про HTTP или конкретную
СУБД, только про интерфейсы (`internal/port`), которые под неё подставляют
адаптеры.

```
go.mod
webassets.go              — go:embed фронтенда (лежит в корне: go:embed не
                             умеет смотреть за пределы своей директории)
cmd/whereabouts/
  main.go                 — composition root: выбирает адаптер БД по
                             DB_DRIVER, собирает остальные адаптеры,
                             юзкейсы, HTTP-роуты и запускает сервер
  db.go, middleware.go, seed.go
internal/
  domain/
    entity/                 — сущности (Item, Location, User, Session)
    error/                  — типизированные ошибки (Validation/NotFound/…)
    usecase/
      item/, location/, auth/, user/
                             — юзкейсы: один интерактор (Execute) на
                             сценарий
    service/
      passwordhasher/, tokengenerator/, imageprocessor/
                             — доменные сервисы: bcrypt, случайные токены
                             сессий, сжатие фото (чистая логика, без
                             внешнего состояния кроме stdlib/crypto)
  port/                    — интерфейсы, от которых зависят юзкейсы:
                             репозитории, PasswordHasher, ImageStore и т.д.
  repository/              — реализации портов-репозиториев, по пакету на
                             сущность; SQL здесь нет, только вызовы storage,
                             конвертация моделей в сущности и перевод
                             sql.ErrNoRows в NotFoundError (а нарушения
                             UNIQUE — в ConflictError). Каждый пакет сам
                             объявляет интерфейс Storage — ровно те операции
                             БД, которые вызывает именно он; сходятся они
                             только в композиционном корне (app/db.go)
    item/, location/, session/, user/
  storage/
    error/                   — sentinel UniqueViolationError: у нарушения
                             UNIQUE нет портируемого представления, поэтому
                             каждый драйвер приводит своё к нему, а
                             репозиторий переводит его в ConflictError
                             (у sql.ErrNoRows аналог не нужен — он
                             стандартный)
    model/                   — DB-модели (форма строк таблиц), отдельные от
                             domain/entity; колонки с разным представлением
                             у драйверов (settings JSON, TIMESTAMP) умеют
                             сканироваться сами
  adapter/
    httpapi/                 — driving-адаптер: net/http хендлеры и роуты
    sqlite/, mysql/, postgres/
                             — по одному driven-адаптеру на драйвер БД:
                             Config, Migrate и Storage — вся работа с БД,
                             каждый на своём диалекте ("?" против "$1",
                             LastInsertId против RETURNING, JSON_SET против
                             jsonb_set)
    filesystem/              — файлы фото на диске
web/
  index.html                — фронтенд (встраивается в бинарник через go:embed)
```

## Требования

- Go 1.26 или новее
- Для `DB_DRIVER=mysql` — запущенный сервер MySQL/MariaDB,
  пользователь с правами `CREATE DATABASE` (или база, созданная заранее)
- Для `DB_DRIVER=postgres` — запущенный сервер PostgreSQL, пользователь с
  правами на создание базы (или база, созданная заранее)
- Для `DB_DRIVER=sqlite` — ничего, кроме прав на запись в каталог: файл базы
  создаётся приложением само

## Переменные окружения

У конфигурации БД нет значений по умолчанию: `DB_DRIVER` и всё, что нужно
выбранному драйверу, обязательны и проверяются при старте — при пропуске
сервер сразу завершится с понятной ошибкой, а не молча подключится не туда.

| Переменная       | Обязательна          | Описание                          |
|------------------|------------------------|------------------------------------|
| `DB_DRIVER`      | да                     | `mysql`, `postgres` или `sqlite`   |
| `DB_HOST`        | для mysql/postgres     | Хост БД                            |
| `DB_PORT`        | для mysql/postgres     | Порт БД (обычно `3306` для MySQL, `5432` для Postgres) |
| `DB_USER`        | для mysql/postgres     | Пользователь                       |
| `DB_PASSWORD`    | нет (пусто)            | Пароль (mysql/postgres)            |
| `DB_NAME`        | для mysql/postgres     | Имя базы, создастся сама, если её нет |
| `DB_SQLITE_PATH` | для sqlite             | Путь к файлу БД                    |
| `DB_MAX_OPEN_CONNS` | нет (`10`)          | Макс. открытых соединений (игнорируется для sqlite — всегда 1) |
| `DB_MAX_IDLE_CONNS` | нет (`5`)           | Макс. простаивающих соединений (игнорируется для sqlite) |
| `DB_CONN_MAX_LIFETIME` | нет (`5m`)       | Макс. время жизни соединения (формат `time.ParseDuration`, напр. `30s`) |
| `PORT`           | нет (`8080`)           | Порт, на котором слушает сам сервер |
| `COOKIE_SECURE`  | нет (`false`)          | `true` — кука сессии только по HTTPS (включите в проде) |

## Запуск локально

```bash
cd backend

# Подтянуть зависимости (нужен интернет один раз, дальше кешируется)
go mod tidy

# Вариант 1 — MySQL/MariaDB:
export DB_DRIVER=mysql
export DB_HOST=127.0.0.1
export DB_PORT=3306
export DB_USER=root
export DB_PASSWORD=secret
export DB_NAME=wherewhat

# Вариант 2 — PostgreSQL:
export DB_DRIVER=postgres
export DB_HOST=127.0.0.1
export DB_PORT=5432
export DB_USER=postgres
export DB_PASSWORD=secret
export DB_NAME=wherewhat

# Вариант 3 — SQLite (никакого сервера БД не нужно):
export DB_DRIVER=sqlite
export DB_SQLITE_PATH=./data/wherewhat.db

go run ./cmd/whereabouts
```

При первом запуске сервер сам создаст базу (если у пользователя есть права
и это mysql/postgres — для sqlite файл создаётся всегда), создаст таблицы
и наполнит их демо-данными. Откройте `http://localhost:8080` — там сразу
открывается интерфейс приложения.

Если прав на создание базы нет — создайте её заранее вручную:

```sql
-- MySQL/MariaDB:
CREATE DATABASE wherewhat CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- PostgreSQL:
CREATE DATABASE wherewhat;
```

Локальный контейнер MariaDB для разработки — через `docker-compose.yml`:

```bash
docker-compose up -d
```

## Сборка бинарника

```bash
go build -o wherewhat ./cmd/whereabouts
./wherewhat
```

Готовый бинарник уже содержит фронтенд внутри — переносить `web/` отдельно
не нужно.

## API

Все ответы — JSON, ошибки — `{"error": "..."}`.

| Метод  | Путь                  | Описание                              |
|--------|-----------------------|----------------------------------------|
| GET    | `/api/health`         | Проверка живости                       |
| GET    | `/api/items`          | Список вещей (с фото и updatedAt)      |
| POST   | `/api/items`          | Создать вещь                           |
| PUT    | `/api/items/{id}`     | Обновить вещь                          |
| DELETE | `/api/items/{id}`     | Удалить вещь                           |
| GET    | `/api/locations`      | Список мест (плоский, с parentId)      |
| POST   | `/api/locations`      | Создать место                          |
| DELETE | `/api/locations/{id}` | Удалить место (если пусто и без вложенных) |

Тело `POST/PUT /api/items`:
```json
{ "name": "Паспорт", "locationId": "loc-hall", "notes": "...", "images": ["data:image/jpeg;base64,..."] }
```

Тело `POST /api/locations`:
```json
{ "name": "Полка 2", "color": "#4A6FA5", "parentId": "loc-garage" }
```

## Авторизация

Настоящая, не мок: пароли хешируются bcrypt'ом, сессии — случайный токен
в таблице `sessions`, привязанный к httpOnly-куке `session_token`
(`SameSite=Lax`, срок жизни 30 дней). `/api/items*` и `/api/locations*`
защищены middleware `requireAuth` — без валидной сессии вернут `401`.

| Метод | Путь                | Описание                                    |
|-------|---------------------|-----------------------------------------------|
| POST  | `/api/auth/register`| Создать пользователя (не используется в UI, но доступен) |
| POST  | `/api/auth/login`   | `{ "email": "...", "password": "..." }` → пользователь + кука |
| POST  | `/api/auth/logout`  | Удаляет сессию и куку                        |
| GET   | `/api/auth/me`      | Текущий пользователь по куке (для восстановления сессии после перезагрузки страницы) |

При первом запуске создаётся демо-пользователь: `demo@example.com` / `demo1234`.

Для продакшена за HTTPS выставьте `COOKIE_SECURE=true`, чтобы кука
сессии отправлялась только по HTTPS.

## Сжатие изображений

Фронтенд уже уменьшает фото перед отправкой (canvas, до 1000px), но бэкенд
не полагается на это и сжимает самостоятельно — на случай прямых запросов
к API или изображений, которые всё равно оказались большими. Логика в
`internal/domain/service/imageprocessor/compressor.go`:

- Если фото уже компактное (≤ 350 КБ и формат JPEG) — не трогаем.
- Иначе декодируем (JPEG/PNG/GIF из стандартной библиотеки), уменьшаем
  большую сторону до 1600px (билинейная интерполяция, без внешних
  зависимостей) и перекодируем в JPEG, подбирая качество (85 → 40) так,
  чтобы уложиться примерно в 700 КБ.
- Формат, который стандартная библиотека не умеет декодировать (например,
  HEIC/WebP), не отбрасывается — сохраняется как есть, чтобы не терять фото.
- Общий размер тела запроса `POST/PUT /api/items` ограничен 20 МБ
  (`http.MaxBytesReader`) — защита от чрезмерно больших payload'ов ещё до
  разбора JSON.

## Что дальше

- Изображения хранятся как base64 прямо в БД (колонка `MEDIUMTEXT`). Для
  больших объёмов лучше вынести их в файловое хранилище/S3 и хранить в БД
  только ссылку — тоже можно сделать отдельным шагом.
- Регистрация есть на бэкенде (`/api/auth/register`), но не выведена в UI —
  во фронтенде осталась только форма входа. При желании можно вернуть
  экран регистрации.
