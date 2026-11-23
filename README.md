# DCMS Social (Go + MySQL / MariaDB)

Полный перенос социальной сети DCMS-Social 2.1.0 с PHP на Go. Один бинарник: REST API + SPA-фронтенд (вшит в бинарник),
хранение данных в **MySQL / MariaDB** (InnoDB, `utf8mb4`, автоматическое создание и миграция таблиц при запуске).
Загруженные файлы пользователей (фото, файлы обменника и т.д.) хранятся в каталоге данных (`-data`).

## Установка на чистый Linux-сервер (Debian/Ubuntu/RHEL, systemd)

```bash
curl -fsSL https://git.home/admin/dcms-reborn/raw/branch/main/deploy/dcms.sh \
  | sudo REPO=https://git.home/admin/dcms-reborn.git bash -s install
```

С доменом, nginx и HTTPS (Let's Encrypt):

```bash
curl -fsSL https://git.home/admin/dcms-reborn/raw/branch/main/deploy/dcms.sh \
  | sudo REPO=https://git.home/admin/dcms-reborn.git DOMAIN=example.com EMAIL=me@example.com bash -s install
```

С внешним сервером MySQL / MariaDB (без установки локальной MariaDB):

```bash
curl -fsSL https://git.home/admin/dcms-reborn/raw/branch/main/deploy/dcms.sh \
  | sudo REPO=https://git.home/admin/dcms-reborn.git DCMS_DSN="user:pass@tcp(db.host:3306)/dcms?charset=utf8mb4" bash -s install
```

Скрипт ставит `git`, `Go`, **MariaDB** (если не задан внешний `DCMS_DSN`), создаёт базу данных `dcms` и пользователя,
создаёт системного пользователя `dcms`, собирает проект в `/opt/dcms`, кладёт загруженные файлы в `/var/lib/dcms`,
сохраняет настройки в `/etc/dcms/dcms.env`, создаёт systemd-службу `dcms` и (если задан `DOMAIN`) nginx-прокси.
Первый зарегистрированный пользователь становится «Создателем».

## Обновление

```bash
sudo dcms-update
```

Забирает свежий коммит из `https://git.home/admin/dcms-reborn.git`, делает резервную копию базы данных (SQL-дамп) и загруженных файлов
(`/var/backups/dcms`, хранится 14 последних), собирает новую версию, перезапускает службу и проверяет, что она отвечает.
Если сборка или запуск не удались — автоматически откатывается на прежнюю версию.

| Команда | Действие |
|---|---|
| `dcms-update` | обновить до последней версии ветки |
| `dcms rollback` | вернуться на предыдущую версию |
| `dcms backup` | резервная копия (SQL-дамп БД + файлы) |
| `dcms status` / `dcms logs` | состояние / журнал службы |
| `dcms uninstall` | удалить службу и код (файлы и БД остаются) |

Переменные: `REPO` (`https://git.home/admin/dcms-reborn.git`), `BRANCH` (`main`), `DOMAIN`, `EMAIL`, `PORT` (`8080`), `GO_VERSION`, `DCMS_DSN`, `DB_NAME` (`dcms`), `DB_USER` (`dcms`), `DB_PASS`.

## Разработка

Создайте базу данных в MySQL / MariaDB:

```sql
CREATE DATABASE dcms CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER 'dcms'@'127.0.0.1' IDENTIFIED BY 'dcms';
GRANT ALL PRIVILEGES ON dcms.* TO 'dcms'@'127.0.0.1';
FLUSH PRIVILEGES;
```

Запуск:

```bash
DCMS_DEV=1 DCMS_DSN="dcms:dcms@tcp(127.0.0.1:3306)/dcms?charset=utf8mb4" \
  go run . -addr :8080 -data ./data -web web   # -web — отдавать фронтенд с диска без пересборки
```

Также поддерживаются отдельные переменные окружения: `DCMS_DB_HOST`, `DCMS_DB_PORT`, `DCMS_DB_USER`, `DCMS_DB_PASS`, `DCMS_DB_NAME` (используются, если не задан `DCMS_DSN`).
