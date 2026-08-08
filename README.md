# libshelf

Личная оболочка для коллекции Flibusta (`.inpx` + архивы fb2).

- поиск сразу по **автору + названию + серии** (`кинг сияние`)
- по умолчанию только **`lang = ru`**
- карточка книги, обложка из FB2, скачивание
- **читалка** FB2 в браузере (скролл, оглавление, прогресс)
- личные списки **Читаю / Прочитано / Хочу**
- вход с ролями **читатель** / **админ**
- один бинарник Go, SQLite + FTS5

## Быстрый деплой новых версий (тест)

Каждый push в `master` собирает linux-бинарник в GitHub Actions:
- **Artifact** `libshelf-linux-amd64` (вкладка Actions)
- **Pre-release** [`latest`](https://github.com/amachulan/libshelf/releases/tag/latest) с тем же файлом

На сервере один раз:

```sh
sudo mkdir -p /opt/libshelf
sudo curl -fsSL -o /opt/libshelf/deploy.sh \
  https://raw.githubusercontent.com/amachulan/libshelf/master/scripts/deploy.sh
sudo chmod +x /opt/libshelf/deploy.sh
```

`deploy.sh` качает бинарник через **curl** (gh не обязателен).

Дальше после каждого `git push` в master:

```sh
sudo /opt/libshelf/deploy.sh
```

Скрипт скачивает `latest`, подменяет `/opt/libshelf/libshelf`, перезапускает `screen`.  
Базу и inpx **не трогает**.

Переменные окружения (опционально): `LIBSHELF_REPO`, `LIBSHELF_BIN`, `LIBSHELF_DATA`, `LIBSHELF_LIB`, `LIBSHELF_ADDR`, `LIBSHELF_AUTH` (`users`|`none`).

## Авторизация

По умолчанию `--auth=users`: без логина каталог недоступен.

При первом запуске, если пользователей ещё нет, создаётся админ:
- из env `LIBSHELF_ADMIN_USER` / `LIBSHELF_ADMIN_PASS`, или
- `admin` + случайный пароль (смотрите `screen -r libshelf` / логи)

Роли:
- **reader** — поиск, карточки, скачивание, читалка, свои списки
- **admin** — всё то же + управление пользователями в UI

Списки и прогресс чтения хранятся в `users.db` и не сбрасываются при `import --replace`.

Добавить пользователя вручную:

```sh
/opt/libshelf/libshelf user add \
  --data-dir /opt/libshelf/data \
  --username alice \
  --password 'secret' \
  --role reader
```

Открытый режим (без логина): `--auth=none` или `LIBSHELF_AUTH=none` в deploy.

## Первая установка

```sh
sudo mkdir -p /opt/libshelf/data
# либо собрать локально / скачать с release latest:
gh release download latest -R amachulan/libshelf -p libshelf-linux-amd64 -D /tmp
sudo install -m 755 /tmp/libshelf-linux-amd64 /opt/libshelf/libshelf
```

### Импорт (один раз)

```sh
/opt/libshelf/libshelf import \
  --inpx "/mnt/share/Книги/fb2.Flibusta.Net/flibusta_fb2_local.inpx" \
  --library-dir "/mnt/share/Книги/fb2.Flibusta.Net" \
  --data-dir /opt/libshelf/data
```

Повторный импорт каталога: добавить `--replace`.

### Запуск

```sh
screen -dmaS libshelf /opt/libshelf/libshelf serve \
  --addr 127.0.0.1:12380 \
  --library-dir "/mnt/share/Книги/fb2.Flibusta.Net" \
  --data-dir /opt/libshelf/data \
  --auth users
```

Или сразу `sudo /opt/libshelf/deploy.sh` после появления release `latest`.

Проверка:

```sh
curl -s http://127.0.0.1:12380/health
# пароль bootstrap смотрите в логе screen при первом старте
```

## Сборка вручную

```sh
git clone https://github.com/amachulan/libshelf.git
cd libshelf
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o libshelf ./cmd/libshelf
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

## API

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/api/search?q=...&limit=40` | поиск |
| GET | `/api/book/{id}` | карточка (+ shelfStatus/progress) |
| GET | `/api/book/{id}/read` | HTML читалки + оглавление |
| PUT | `/api/book/{id}/progress` | `{position:0..1}` |
| GET | `/api/shelf?status=reading\|read\|want` | список полки |
| GET | `/api/shelf/continue` | продолжить чтение |
| PUT | `/api/shelf/{id}` | `{status}` или `{status:null}` |
| GET | `/cover/{id}` | обложка |
| GET | `/download/{id}` | скачать FB2 |
| GET | `/api/stats` | число книг (ru) |
| GET | `/health` | health-check |
