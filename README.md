# libshelf

Self-hosted веб-каталог личной библиотеки: индекс **`.inpx`** + архивы **FB2**.

Один статический бинарник на Go, SQLite + FTS5, встроенный UI. Подходит для домашнего сервера, NAS или VPS: импортируете каталог MyHomeLib/INPX, указываете папку с архивами и поднимаете HTTP.

## Возможности

- поиск по **автору, названию и серии** сразу; расширенный — автор, **год издания** и **год добавления** в каталог; **ё/е** и порядок имени не мешают; выдача по релевантности, с **пагинацией**
- одинаковые книги с разными файлами схлопываются в одно **произведение**, на карточке — выбор **издания / перевода**
- языки каталога задаёт **админ** (по умолчанию `ru`; можно несколько или все)
- карточка книги, обложка из FB2, скачивание FB2
- **читалка** FB2 в браузере: страницы или лента, выравнивание, полноэкран, прогресс
- личные списки **Читаю / Прочитано / Хочу**
- **каталог**: авторы и серии по буквам, жанры
- несколько корней архивов, **dedupe** и дозагрузка нового `.inpx` без копирования всей коллекции
- запуск на **Windows** мастером настройки (двойной щелчок)
- **OPDS** для ридеров (`/opds`)
- вход с ролями **читатель** / **админ** (можно отключить)

## Что нужно

1. Файл каталога `.inpx` (формат MyHomeLib / совместимые сборки).
2. Каталог с архивами книг, на которые ссылается этот `.inpx` (`--library-dir`).
3. Каталог данных для SQLite и кэша обложек (`--data-dir`).

Пути ниже — примеры; подставьте свои.

## Windows (удобный запуск)

1. Скачайте [`libshelf-windows-amd64.exe`](https://github.com/amachulan/libshelf/releases/download/latest/libshelf-windows-amd64.exe).
2. Положите exe куда удобно (можно рядом с книгами) и **запустите двойным щелчком**.
3. Откроется браузер с мастером настройки:
   - файл каталога `.inpx`
   - папка с архивами FB2
   - папка данных (по умолчанию `data` рядом с exe)
4. Дождитесь импорта. Если включён вход, на экране покажут логин/пароль админа — сохраните их.
5. Пока пользуетесь библиотекой, **не закрывайте** чёрное окно консоли (это и есть LibShelf).

Повторный запуск — снова двойной щелчок: настройки читаются из `libshelf.json` рядом с exe, браузер откроется сам.

Остановить: закрыть окно консоли или Ctrl+C.

## Быстрый старт (Linux / сервер / из исходников)

### 1. Бинарник

С [Releases / latest](https://github.com/amachulan/libshelf/releases/tag/latest):

| Файл | Назначение |
|------|------------|
| [`libshelf-linux-amd64`](https://github.com/amachulan/libshelf/releases/download/latest/libshelf-linux-amd64) | сервер, NAS, VPS |
| [`libshelf-windows-amd64.exe`](https://github.com/amachulan/libshelf/releases/download/latest/libshelf-windows-amd64.exe) | ПК Windows (см. выше) |

Или сборка из исходников:

```sh
git clone https://github.com/amachulan/libshelf.git
cd libshelf
CGO_ENABLED=0 go build -o libshelf ./cmd/libshelf
```

Кросс-сборка:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o libshelf-linux-amd64 ./cmd/libshelf
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o libshelf-windows-amd64.exe ./cmd/libshelf
```

### 2. Режим start (мастер в браузере)

На любой ОС можно так же, как на Windows:

```sh
./libshelf start
# или просто: ./libshelf
```

При пустой базе откроется `/setup.html`. Конфиг пишется в `libshelf.json` рядом с бинарником.

### 3. Импорт из CLI

```sh
./libshelf import \
  --inpx /path/to/catalog.inpx \
  --library-dir /path/to/fb2/archives \
  --data-dir /path/to/libshelf-data
```

Повторный полный импорт (пересоздать индекс книг): добавьте `--replace`.  
База пользователей (`users.db`: аккаунты, полки, прогресс) при `--replace` **не** трогается.

### 4. Запуск сервера (без мастера)

```sh
./libshelf serve \
  --addr 127.0.0.1:12380 \
  --library-dir /path/to/fb2/archives \
  --data-dir /path/to/libshelf-data \
  --auth users \
  --open
```

Откройте в браузере `http://127.0.0.1:12380` (или доверьте `--open`).

Проверка:

```sh
curl -s http://127.0.0.1:12380/health
# ok <git-sha>
```

## Два каталога `.inpx` (старый + новый)

Типичный случай: в старом дампе есть книги, которых нет в свежем (или наоборот). Старый дамп **не трогаем**; новый после получения чистим от дублей и дописываем в базу.

1. Положить свежий слепок в отдельную папку (например `/data/books-new/`).
2. Убрать из нового `.inpx` всё, что уже есть в вашей базе:

```sh
./libshelf dedupe \
  --base-db /opt/libshelf/data/libshelf.db \
  --incoming /data/books-new/catalog.inpx \
  --out /data/books-new/catalog.unique.inpx \
  --library-dir /data/books-new \
  --prune-empty-archives
```

Сначала можно добавить `--dry-run`, чтобы только увидеть список архивов на удаление.

3. Дописать каталог без очистки старой базы (zip нового дампа **можно не переносить**):

```sh
./libshelf import --append \
  --inpx /data/books-new/catalog.unique.inpx \
  --library-dir /opt/libshelf/library \
  --data-dir /opt/libshelf/data
```

4. Запускать `serve` с **двумя** корнями архивов:

```sh
./libshelf serve \
  --library-dir /opt/libshelf/library \
  --library-dir /data/books-new \
  --data-dir /opt/libshelf/data \
  --auth users
```

Или в `deploy.sh`: `LIBSHELF_LIB_EXTRA=/data/books-new` (несколько путей через `:`).

Вместо `--base-db` можно указать эталонный старый `.inpx`: `--base /path/to/old.inpx`.  
При конфликте LIBID побеждает уже существующая запись (старая база / первый `--inpx`).

## Авторизация

По умолчанию `--auth=users`: без логина UI и API недоступны (кроме `/health`, страницы входа и статики логина).

При первом запуске, если пользователей ещё нет, создаётся админ:
- из переменных окружения `LIBSHELF_ADMIN_USER` / `LIBSHELF_ADMIN_PASS`, или
- логин `admin` и случайный пароль (смотрите лог процесса при первом старте)

Роли:

| Роль | Доступ |
|------|--------|
| **reader** | поиск, карточки, скачивание, читалка, свои списки |
| **admin** | всё то же + управление пользователями в UI |

Добавить пользователя из CLI:

```sh
./libshelf user add \
  --data-dir /path/to/libshelf-data \
  --username alice \
  --password 'secret' \
  --role reader
```

Открытый режим без логина: `--auth=none`.

## OPDS

Корень каталога для приложений вроде KOReader / Moon+ / Aldiko:

```
http://HOST:PORT/opds
```

При `--auth=users` клиент должен уметь **HTTP Basic** (тот же логин/пароль, что и в веб-UI).  
Basic Auth намеренно работает только для `/opds`, `/download/` и `/cover/`, чтобы браузер не подставлял сохранённые OPDS-учётные данные на весь сайт.

Поиск: OpenSearch из корневого фида или `/opds/search?q=...`.

## Обратный прокси (nginx)

Типичная схема: libshelf слушает localhost, снаружи — HTTPS через nginx/Caddy.

```nginx
server {
    listen 443 ssl http2;
    server_name books.example.com;

    # ssl_certificate …;
    # ssl_certificate_key …;

    location / {
        proxy_pass http://127.0.0.1:12380;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Host $host;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

Важно передать `X-Forwarded-Proto`, иначе cookie сессии могут выставиться без флага `Secure` за HTTPS.

## Обновление с GitHub Releases

Каждый push в `master` собирает **linux amd64** и **windows amd64** в Actions и обновляет pre-release [`latest`](https://github.com/amachulan/libshelf/releases/tag/latest).

Вспомогательный скрипт `scripts/deploy.sh`:

- ждёт публикацию **текущего** commit в `latest` (не ставит предыдущую сборку);
- скачивает бинарник, сверяет `libshelf version` с SHA;
- подменяет бинарник и перезапускает процесс в `screen` (пути настраиваются env).

Пример установки скрипта:

```sh
sudo mkdir -p /opt/libshelf
sudo curl -fsSL -o /opt/libshelf/deploy.sh \
  https://raw.githubusercontent.com/amachulan/libshelf/master/scripts/deploy.sh
sudo chmod +x /opt/libshelf/deploy.sh

# Локальные пути — в deploy.env (не затирается при обновлении deploy.sh):
sudo tee /opt/libshelf/deploy.env <<'EOF'
LIBSHELF_LIB="/mnt/share/Книги/fb2.Flibusta.Net"
LIBSHELF_LIB_EXTRA="/mnt/share/Книги/fb2.Flibusta.Net.2"
EOF

sudo /opt/libshelf/deploy.sh
```

Скрипт сам подхватывает `/opt/libshelf/deploy.env` (или путь из `LIBSHELF_ENV`). Переменные окружения по-прежнему можно задать и вручную: `LIBSHELF_REPO`, `LIBSHELF_BIN`, `LIBSHELF_DATA`, `LIBSHELF_LIB`, `LIBSHELF_LIB_EXTRA` (доп. каталоги через `:`), `LIBSHELF_ADDR`, `LIBSHELF_AUTH` (`users`|`none`), `LIBSHELF_LANGUAGES`, `LIBSHELF_DEPLOY_WAIT`.

Импорт и базу скрипт **не** трогает — только бинарник и перезапуск `serve`.

## CLI

```text
libshelf                 (= start)
libshelf start           [--config FILE] [--addr HOST:PORT] [--no-browser]
libshelf import          --inpx FILE [--inpx FILE ...] --library-dir DIR --data-dir DIR [--replace|--append]
libshelf dedupe          --incoming FILE --out FILE (--base FILE | --base-db PATH) [--library-dir DIR] [--prune-empty-archives] [--dry-run]
libshelf serve           --library-dir DIR [--library-dir DIR ...] --data-dir DIR [--addr HOST:PORT] [--auth users|none] [--lang CODE] [--open]
libshelf user add        --data-dir DIR --username NAME --password PASS [--role admin|reader]
libshelf version
```

Языки каталога: в мастере setup, в `libshelf.json` (`"languages": ["ru"]` / `["ru","en"]` / `["*"]`), флаг `--lang` (повторяемый) или env `LIBSHELF_LANGUAGES=ru,en`. После смены языков при старте пересоберётся поисковый индекс.

`libshelf.json` (рядом с exe при `start`):

```json
{
  "addr": "127.0.0.1:12380",
  "library_dir": "C:\\\\Books\\\\archives",
  "data_dir": "C:\\\\LibShelf\\\\data",
  "inpx": "C:\\\\Books\\\\catalog.inpx",
  "auth": "users",
  "languages": ["ru"],
  "open_browser": true
}
```

## API (кратко)

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/api/search?q=&title=&author=&year=&added=` | поиск (`year`/`added` или `*_from`+`*_to`) |
| GET | `/api/book/{id}` | карточка (+ shelfStatus/progress) |
| GET | `/api/book/{id}/read` | HTML читалки + оглавление |
| PUT | `/api/book/{id}/progress` | `{position:0..1}` |
| GET | `/api/shelf?status=reading\|read\|want` | список полки |
| GET | `/api/shelf/continue` | продолжить чтение |
| PUT | `/api/shelf/{id}` | `{status}` или `{status:null}` |
| GET | `/api/catalog/authors` | буквы / авторы |
| GET | `/api/catalog/series` | серии |
| GET | `/api/catalog/genres` | жанры |
| GET | `/api/catalog/genres/{code}` | книги жанра |
| GET | `/opds` | OPDS root |
| GET | `/cover/{id}` | обложка |
| GET | `/download/{id}` | скачать FB2 |
| GET | `/api/stats` | число видимых книг |
| GET | `/health` | health-check (`ok <sha>`) |

При `--auth=users` большинство `/api/*` требуют сессию (cookie после `/api/login`).
