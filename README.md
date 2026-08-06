# Где·Что — бэкенд (Go + MariaDB/PostgreSQL/SQLite)

Бэкенд на чистой стандартной библиотеке Go (`net/http`, Go 1.26+) плюс
драйверы БД. Поддерживает три СУБД, выбираются переменной окружения
`DB_DRIVER`:

- `mariadb` (по умолчанию) — через `github.com/go-sql-driver/mysql`, работает
  и с MySQL, и с MariaDB;
- `postgres` — через `github.com/jackc/pgx/v5/stdlib`;
- `sqlite` — через `modernc.org/sqlite` (чистый Go, без cgo), файл БД на
  диске, сервер не нужен.

Фронтенд (тот же Vue-файл из `/mnt/user-data/outputs`) вшит в бинарник и
отдаётся тем же сервером — открываете один URL, и всё работает без
CORS-плясок.

## Структура

```
backend/
  go.mod
  main.go       — точка входа: конфиг, роуты, встраивание фронтенда
  db.go         — подключение к БД (mariadb/postgres/sqlite), миграции, демо-сид
  models.go     — структуры Item / Location
  handlers.go   — HTTP-хендлеры (CRUD)
  web/
    index.html  — фронтенд (встраивается в бинарник через go:embed)
```

## Требования

- Go 1.26 или новее
- Для `DB_DRIVER=mariadb` (по умолчанию) — запущенный сервер MariaDB/MySQL,
  пользователь с правами `CREATE DATABASE` (или база, созданная заранее)
- Для `DB_DRIVER=postgres` — запущенный сервер PostgreSQL, пользователь с
  правами на создание базы (или база, созданная заранее)
- Для `DB_DRIVER=sqlite` — ничего, кроме прав на запись в каталог: файл базы
  создаётся приложением само

## Переменные окружения

| Переменная       | По умолчанию          | Описание                          |
|------------------|------------------------|------------------------------------|
| `DB_DRIVER`      | `mariadb`              | `mariadb`, `postgres` или `sqlite` |
| `DB_HOST`        | `127.0.0.1`            | Хост БД (mariadb/postgres)         |
| `DB_PORT`        | `3306` / `5432`        | Порт БД (mariadb/postgres); по умолчанию зависит от `DB_DRIVER` |
| `DB_USER`        | `root`                 | Пользователь (mariadb/postgres)    |
| `DB_PASSWORD`    | (пусто)                | Пароль (mariadb/postgres)          |
| `DB_NAME`        | `wherewhat`            | Имя базы (mariadb/postgres), создастся сама, если её нет |
| `DB_SQLITE_PATH` | `./data/wherewhat.db`  | Путь к файлу БД (sqlite)           |
| `PORT`           | `8080`                 | Порт, на котором слушает сам сервер |
| `COOKIE_SECURE`  | `false`                | `true` — кука сессии только по HTTPS (включите в проде) |

## Запуск локально

```bash
cd backend

# Подтянуть зависимости (нужен интернет один раз, дальше кешируется)
go mod tidy

# Вариант 1 — MariaDB/MySQL (по умолчанию):
export DB_DRIVER=mariadb
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

go run .
```

При первом запуске сервер сам создаст базу (если у пользователя есть права
и это mariadb/postgres — для sqlite файл создаётся всегда), создаст таблицы
и наполнит их демо-данными. Откройте `http://localhost:8080` — там сразу
открывается интерфейс приложения.

Если прав на создание базы нет — создайте её заранее вручную:

```sql
-- MariaDB/MySQL:
CREATE DATABASE wherewhat CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- PostgreSQL:
CREATE DATABASE wherewhat;
```

Локальные контейнеры БД для разработки — через `docker-compose.yml`:

```bash
# MariaDB (по умолчанию):
docker-compose up -d

# PostgreSQL (сервис под профилем, не стартует по умолчанию):
docker-compose --profile postgres up -d postgres
```

## Сборка бинарника

```bash
cd backend
go build -o wherewhat .
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
`images.go`:

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
