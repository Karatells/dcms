# DCMS Social (Go)

Полный перенос социальной сети DCMS-Social 2.1.0 с PHP на Go. Один бинарник: REST API + SPA-фронтенд (вшит в бинарник),
только стандартная библиотека Go. Данные хранятся на диске (журнал + снимки) в каталоге данных и переживают перезапуск.

## Установка на чистый Linux-сервер (Debian/Ubuntu/RHEL, systemd)

```bash
curl -fsSL https://raw.githubusercontent.com/<user>/<repo>/main/deploy/dcms.sh \
  | sudo REPO=https://github.com/<user>/<repo>.git bash -s install
```

С доменом, nginx и HTTPS (Let's Encrypt):

```bash
curl -fsSL .../deploy/dcms.sh | sudo REPO=... DOMAIN=example.com EMAIL=me@example.com bash -s install
```

Скрипт ставит git/Go, создаёт пользователя `dcms`, собирает проект в `/opt/dcms`, кладёт данные в `/var/lib/dcms`,
создаёт systemd-службу `dcms` и (если задан `DOMAIN`) nginx-прокси. Первый зарегистрированный пользователь становится «Создателем».

## Обновление

```bash
sudo dcms-update
```

Забирает свежий коммит из репозитория, делает резервную копию данных (`/var/backups/dcms`, хранится 14 последних), собирает новую версию,
перезапускает службу и проверяет, что она отвечает. Если сборка или запуск не удались — автоматически откатывается на прежнюю версию.

| Команда | Действие |
|---|---|
| `dcms-update` | обновить до последней версии ветки |
| `dcms rollback` | вернуться на предыдущую версию |
| `dcms backup` | резервная копия данных |
| `dcms status` / `dcms logs` | состояние / журнал службы |
| `dcms uninstall` | удалить службу и код (данные остаются) |

Переменные: `REPO`, `BRANCH` (main), `DOMAIN`, `EMAIL`, `PORT` (8080), `GO_VERSION`.

## Разработка

```bash
DCMS_DEV=1 go run . -addr :8080 -data ./data -web web   # -web — отдавать фронтенд с диска без пересборки
```
