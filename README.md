# DCMS Social (Go + MySQL / MariaDB)

Современная социальная сеть на базе DCMS-Social, переписанная на Go. Один бинарник: REST API + SPA-фронтенд (вшит в бинарник) + realtime SSE, хранение данных в **MySQL / MariaDB** (InnoDB, `utf8mb4`, автоматическое создание и миграция таблиц при запуске).
Загруженные файлы пользователей (фото, клипы, видео-истории, музыка, голосовые, сборки приложений) хранятся в каталоге данных (`-data`).

## Установка на чистый Linux-сервер (Debian/Ubuntu/RHEL, systemd)

По умолчанию скрипт устанавливает **nginx** как внешний reverse-proxy (порт `80` / `443`), а сам Go-сервер слушает только локально на `127.0.0.1:8080` (`DCMS_TRUST_PROXY=1`).

**1. Базовая установка (доступ по IP на 80 порту, домен можно подключить позже):**

```bash
curl -fsSL https://git.home/admin/dcms-reborn/raw/branch/main/deploy/dcms.sh \
  | sudo bash -s install
```

**2. Установка сразу с доменом и автоматическим HTTPS (Let's Encrypt + автопродление):**

```bash
curl -fsSL https://git.home/admin/dcms-reborn/raw/branch/main/deploy/dcms.sh \
  | sudo DOMAIN=example.com EMAIL=admin@example.com bash -s install
```

**3. Установка с внешним сервером MySQL / MariaDB (без установки локальной MariaDB):**

```bash
curl -fsSL https://git.home/admin/dcms-reborn/raw/branch/main/deploy/dcms.sh \
  | sudo DCMS_DSN="user:pass@tcp(db.host:3306)/dcms?charset=utf8mb4" bash -s install
```

Скрипт устанавливает `git`, `ffmpeg`, `Go`, **MariaDB** (если не задан внешний `DCMS_DSN`) и **nginx**, создаёт базу данных `dcms` и пользователя, собирает проект в `/opt/dcms`, хранит данные в `/var/lib/dcms`, настройки — в `/etc/dcms/dcms.env` и `/etc/dcms/install.conf`, и поднимает systemd-службу `dcms`.
Первый зарегистрированный пользователь автоматически становится «Создателем».

---

## Подключение домена и миграция на nginx-прокси

Если сервер изначально был установлен без домена (работал по IP на 80 порту) или на старой схеме без `nginx`:

**Перевести старую установку на схему `nginx → 127.0.0.1:8080` (без потери данных):**

```bash
sudo dcms migrate-proxy
```

**Перейти с IP (80 порт) на домен с автоматическим выпуском Let's Encrypt и автопродлением:**

```bash
sudo dcms domain example.com admin@example.com
# или через migrate-proxy:
sudo dcms migrate-proxy example.com admin@example.com
```

После привязки домена скрипт:
- настраивает `nginx` с `server_name example.com` и `proxy_buffering off` (для SSE);
- добавляет `default_server`-блок с **301-редиректом** со старого IP-адреса (`http://<IP>`) на новый домен;
- выпускает сертификат **Let's Encrypt** (если не передан флаг `--no-ssl`) и включает автоматическое продление (`systemd certbot.timer` или `/etc/cron.d/dcms-certbot`).

---

## Обновление и управление сервером

```bash
sudo dcms-update
```

`dcms-update` забирает свежий коммит из ветки, делает резервную копию БД (SQL-дамп) и файлов (`/var/backups/dcms`, хранится 14 последних), собирает новую версию, сохраняет существующую SSL-конфигурацию `nginx`, перезапускает службу и проверяет health-check. При ошибке автоматически откатывается на предыдущую версию.

| Команда | Действие |
|---|---|
| `sudo dcms-update` | обновить сервер до последней версии ветки |
| `sudo dcms domain <домен> [email] [--no-ssl]` | привязать/сменить домен, выпустить Let's Encrypt и включить автопродление |
| `sudo dcms migrate-proxy [домен] [email] [--port 8080] [--no-ssl] [--force]` | мигрировать старую установку на `nginx → 127.0.0.1:PORT` или перейти с IP на домен |
| `sudo dcms build-clients [флаги]` | собрать клиентские приложения (Android APK, Linux, Windows) и опубликовать их на `/apps` |
| `sudo dcms rollback` | откатиться на предыдущую версию |
| `sudo dcms backup` | создать резервную копию (SQL-дамп БД + файлы) |
| `sudo dcms status` / `dcms logs` | состояние / логи службы в реальном времени |
| `sudo dcms uninstall` | удалить службу, nginx-конфиг и код (данные в `/var/lib/dcms` и БД остаются) |

### Переменные окружения скрипта установки/обновления
`REPO` (`https://git.home/admin/dcms-reborn.git`), `BRANCH` (`main`), `DOMAIN`, `EMAIL`, `ENABLE_SSL` (`1`), `PORT` (`8080`), `GO_VERSION`, `DCMS_DSN`, `DB_NAME` (`dcms`), `DB_USER` (`dcms`), `DB_PASS`, `BUILD_CLIENTS` (`0` — при `1` клиенты пересобираются автоматически после `dcms-update`).

---

## Сборка клиентских приложений (Android, Windows, Linux)

Скрипт `deploy/build-clients.sh` (команда `sudo dcms build-clients`) поддерживает Linux-серверы **`x86_64` и `arm64` (`aarch64`)** и **автоматически устанавливает** недостающие зависимости (Node.js 20, OpenJDK 17, Gradle 8.5, Android SDK и нативный `arm64` пакет `aapt`).

```bash
# Собрать Android APK + Linux (x64, arm64) + Windows (x64) под ваш домен:
sudo dcms build-clients --url https://example.com --name "DCMS Social" --linux-archs x64,arm64

# Для локального домена с самоподписанным сертификатом:
sudo dcms build-clients --url https://dcms.home --name "DCMS Social" --insecure-tls --targets android,linux,windows
```

Готовые сборки и `manifest.json` сохраняются в `/var/lib/dcms/apps` и сразу становятся доступны для скачивания на странице **`/apps`** сайта, а уже установленные Android и Desktop клиенты автоматически проверяют обновления через `/api/apps/check`.

Для постоянной конфигурации скопируйте `/opt/dcms/deploy/client.conf.example` в `/etc/dcms/client.conf`.

---

## Разработка

Создайте базу данных в MySQL / MariaDB:

```sql
CREATE DATABASE dcms CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER 'dcms'@'127.0.0.1' IDENTIFIED BY 'dcms';
GRANT ALL PRIVILEGES ON dcms.* TO 'dcms'@'127.0.0.1';
FLUSH PRIVILEGES;
```

Запуск в режиме разработки:

```bash
DCMS_DEV=1 DCMS_DSN="dcms:dcms@tcp(127.0.0.1:3306)/dcms?charset=utf8mb4" \
  go run . -addr :8080 -data ./data -web web   # -web — отдавать фронтенд с диска без пересборки
```

Также поддерживаются отдельные переменные окружения: `DCMS_DB_HOST`, `DCMS_DB_PORT`, `DCMS_DB_USER`, `DCMS_DB_PASS`, `DCMS_DB_NAME` (используются, если не задан `DCMS_DSN`).
