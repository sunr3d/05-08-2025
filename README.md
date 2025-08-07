# Архивация файлов по URL (REST API)

REST API сервис для создания ZIP архивов из файлов, доступных по URL.

## Быстрый старт

```bash
go run cmd/main.go
```

Сервер поднимется на `localhost:8080`.

## Конфигурация (ENV)

- `HTTP_PORT` — порт HTTP (default: `8080`)
- `HTTP_TIMEOUT` — таймаут HTTP-клиента при скачивании (default: `30s`)
- `LOG_LEVEL` — уровень логов (`info` по умолчанию)
- `ALLOWED_EXTENSIONS` — список разрешенных MIME (default: `application/pdf,image/jpeg,image/jpg`)
- `MAX_ARCHIVES_IN_PROCESS` — лимит задач «в работе» (default: `3`)
- `MAX_FILES_PER_ARCHIVE` — лимит файлов в задаче (default: `3`)
- `ARCHIVE_TTL` — TTL задач в памяти (default: `1h`)
- `ARCHIVES_DIR` — директория с готовыми zip (default: `./data/archives`)
- `TEMP_DIR` — директория временных файлов (default: `./data/temp`)

## API

Важное: для POST методов с телом нужен заголовок `Content-Type: application/json` (строго без charset). Роуты Go 1.22 — используйте точные пути (без завершающего `/`).

### POST /archive

Создание архива по URL (1–3 шт.). Скачиваются только доступные и подходящих типов.

Request:

```json
{ "urls": ["https://...", "https://..."] }
```

Response (успех, есть хотя бы 1 файл):

```json
{
  "id": "uuid",
  "status": "ready",
  "files": ["file1.pdf", "file2.jpg"],
  "errors": [],
  "created_at": "2025-01-08T10:30:00Z",
  "archive_url": "/download?archive_id=uuid"
}
```

Если все ссылки оказались недоступны/неподдерживаемы — `status: "failed"`, `files: []`, ошибки в `errors`.

### POST /archive/empty

Создать пустую задачу.

Response:

```json
{ "id": "uuid", "status": "empty", "created_at": "2025-01-08T10:30:00Z" }
```

### POST /archive/add-file?archive_id={id}

Добавить файл в задачу. При достижении 3 файлов — собирается ZIP.

Request:

```json
{ "url": "https://example.com/file.pdf" }
```

Response:

```json
{ "success": true, "message": "Файл успешно добавлен к архиву \"uuid\"" }
```

При ошибке загрузки/валидации: `{ "success": false, "message": "..." }`.

### GET /archive/status?archive_id={id}

Вернуть статус задачи. Когда архив собран (3 файла или сборка завершена) — поле `archive_url` присутствует.

Response:

```json
{
  "id": "uuid",
  "status": "ready",
  "files": ["file1.pdf", "file2.jpg", "file3.pdf"],
  "errors": [],
  "created_at": "2025-01-08T10:30:00Z",
  "updated_at": "2025-01-08T10:35:00Z",
  "archive_url": "/download?archive_id=uuid"
}
```

### GET /download?archive_id={id}

Скачать готовый архив (`status == ready`). Отдает `application/zip`.

## Ограничения и правила

- 1–3 файла в задаче; если больше — ошибка
- В работе не больше 3 задач одновременно; при превышении — ошибка «сервер занят»
- Поддерживаемые MIME: `application/pdf`, `image/jpeg`, `image/jpg`
- Хранилище in-memory с TTL: после рестарта задачи исчезают, но zip-файлы остаются в `ARCHIVES_DIR`

## Примеры curl

- Создание архива с частичной ошибкой (2 ок, 1 битая):

```bash
curl -X POST http://localhost:8080/archive \
  -H "Content-Type: application/json" \
  -d '{
    "urls": [
      "https://www.w3.org/WAI/ER/tests/xhtml/testfiles/resources/pdf/dummy.pdf",
      "https://httpbin.org/image/jpeg",
      "https://httpbin.org/status/404"
    ]
  }'
```

- Пустая задача → добавление файлов → статус → скачивание:

```bash
# создать пустую
curl -s -X POST http://localhost:8080/archive/empty

# добавить файл
curl -X POST "http://localhost:8080/archive/add-file?archive_id=YOUR_ID" \
  -H "Content-Type: application/json" \
  -d '{"url": "https://www.w3.org/WAI/ER/tests/xhtml/testfiles/resources/pdf/dummy.pdf"}'

# статус
curl -X GET "http://localhost:8080/archive/status?archive_id=YOUR_ID"

# скачать
curl -L "http://localhost:8080/download?archive_id=YOUR_ID" -o archive.zip
```

## Примечания

- Content-Type для POST: `application/json`
- Роуты без завершающего `/`: используйте `/archive`, а не `/archive/`
- При частичных ошибках список проблемных URL в `errors`, архив формируется по доступным

## Архитектура (кратко)

- Clean Architecture: `interfaces/` — интерфейсы, `services/` — бизнес-логика, `infra/` — инфраструктура (in-memory), `api/` — HTTP хендлеры
- Зависимости прокидываются через конструкторы (DI), явная обработка ошибок, контексты, graceful shutdown
