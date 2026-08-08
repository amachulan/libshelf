# libshelf

Личная оболочка для коллекции Flibusta (`.inpx` + архивы fb2).

- поиск сразу по **автору + названию + серии** (`кинг сияние`)
- по умолчанию только **`lang = ru`**
- карточка книги, обложка из FB2, скачивание
- один бинарник Go, SQLite + FTS5

## Сборка (на сервере Linux)

```sh
git clone <этот-репозиторий> libshelf
cd libshelf
go mod tidy
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o libshelf ./cmd/libshelf
sudo mkdir -p /opt/libshelf/data
sudo cp libshelf /opt/libshelf/libshelf
```

## Импорт

Остановите Polka/Asket, если они слушают `:12380`.

```sh
/opt/libshelf/libshelf import \
  --inpx "/mnt/share/Книги/fb2.Flibusta.Net/flibusta_fb2_local.inpx" \
  --library-dir "/mnt/share/Книги/fb2.Flibusta.Net" \
  --data-dir /opt/libshelf/data
```

Повторный импорт:

```sh
/opt/libshelf/libshelf import ... --replace
```

## Запуск

```sh
screen -dmaS libshelf /opt/libshelf/libshelf serve \
  --addr 127.0.0.1:12380 \
  --library-dir "/mnt/share/Книги/fb2.Flibusta.Net" \
  --data-dir /opt/libshelf/data
```

Проверка:

```sh
curl -s http://127.0.0.1:12380/health
curl -s 'http://127.0.0.1:12380/api/search?q=кинг%20сияние'
```

## Nginx

Конфиг для `books.machulan.ru` менять не нужно, если уже проксирует на `127.0.0.1:12380` с заголовками:

```nginx
proxy_set_header Host $host;
proxy_set_header X-Forwarded-Host $host;
proxy_set_header X-Forwarded-Proto $scheme;
proxy_set_header Upgrade $http_upgrade;
proxy_set_header Connection "upgrade";
```

Откройте `https://books.machulan.ru` и проверьте поиск `кинг сияние`.

## Откат на Polka

```sh
pkill -f '/opt/libshelf/libshelf serve'
screen -dmaS polka /opt/polka/polka serve \
  --addr 127.0.0.1:12380 \
  --library-dir "/mnt/share/Книги/fb2.Flibusta.Net" \
  --data-dir /opt/polka/data
```

## API

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/api/search?q=...&limit=40` | поиск |
| GET | `/api/book/{id}` | карточка |
| GET | `/cover/{id}` | обложка |
| GET | `/download/{id}` | скачать FB2 |
| GET | `/api/stats` | число книг (ru) |
| GET | `/health` | health-check |
