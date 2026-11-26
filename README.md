# DCMS Social

[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![MariaDB / MySQL](https://img.shields.io/badge/MariaDB%20%2F%20MySQL-InnoDB%20utf8mb4-4479A1?logo=mariadb&logoColor=white)](https://mariadb.org/)
[![Clients](https://img.shields.io/badge/Clients-Android%20%7C%20Windows%20%7C%20Linux-6366f1)](https://github.com/Karatells/dcms)

**DCMS Social** — быстрая self-hosted социальная сеть и мессенджер в одном Go-бинарнике.
Включает REST API, встроенный SPA-фронтенд (`embed`), realtime-события через **SSE**, веб-звонки **WebRTC**, нативные клиенты для **Android / Windows / Linux** и автоматическую установку на Linux-сервер за 1 команду.

Репозиторий: **[https://github.com/Karatells/dcms](https://github.com/Karatells/dcms)**

---

## Возможности

- **Мессенджер и общение**: личные диалоги и групповые беседы в реальном времени (SSE), голосовые сообщения, стикерпаки, реакции, редактирование, пересылка, закрепление, поиск по истории, приглашение ботов сообществ, роли/мут/кик.
- **Аудио- и видеозвонки 1-на-1 (WebRTC)**: звонки прямо из диалога или профиля с сигналингом поверх SSE, поддержкой STUN/TURN и сворачиваемым мини-окном.
- **Контент и медиа**: умная лента (`/feed`), вертикальные клипы (`/shorts`) с мобильной Android-студией (камера, стикеры-эмодзи, текст, фильтры, обрезка), видео- и фото-истории на 24 часа, музыкальный раздел с постоянным мини-плеером, фотоальбомы, блоги, форум, файлообменник.
- **Сообщества и Bot API**: группы, каналы, боты с явной адресацией в беседах (`@alias`, `/cmd@alias`, ответ на сообщение).
- **Игры и геймификация**: ежедневные задания и серии, магазин рамок аватара, дуэли (Камень-Ножницы-Бумага, Крестики-нолики, Морской бой 6×6) с настройкой приватности вызовов.
- **Безопасность**:
  - интерактивная self-hosted капча («Я не робот»: пазл, поворот картинки, выбор фигуры — без сторонних сервисов);
  - двухфакторная аутентификация **TOTP** (скрытый ключ + QR-код по кнопке с автоскрытием);
  - **аппаратное подтверждение входа через Android-приложение** (`AndroidKeyStore` ECDSA P-256 + Number Matching);
  - гранулярная приватность профиля, стены, друзей, историй и дуэлей; управление активными сессиями.
- **Клиентские приложения**: Electron Desktop (**Windows**, **Linux x64/arm64**) и **Android APK** с уникальными экранами входа, автообновлением (проверка SHA-256) и разделом `/apps` прямо на сайте.

---

## Быстрая установка на Linux-сервер (Debian / Ubuntu / RHEL, systemd)

При установке автоматически поднимается **nginx** (порты `80` / `443`), который проксирует трафик на локальный Go-сервер (`127.0.0.1:8080`, `DCMS_TRUST_PROXY=1`).

### 1. Базовая установка (по IP на 80 порту, домен можно подключить позже)

```bash
curl -fsSL https://raw.githubusercontent.com/Karatells/dcms/main/deploy/dcms.sh \
  | sudo bash -s install
```

### 2. Установка сразу с доменом и автоматическим HTTPS (Let's Encrypt)

```bash
curl -fsSL https://raw.githubusercontent.com/Karatells/dcms/main/deploy/dcms.sh \
  | sudo DOMAIN=social.example.com EMAIL=admin@example.com bash -s install
```

### 3. Установка с внешней базой MySQL / MariaDB

```bash
curl -fsSL https://raw.githubusercontent.com/Karatells/dcms/main/deploy/dcms.sh \
  | sudo DCMS_DSN="user:pass@tcp(db.host:3306)/dcms?charset=utf8mb4" bash -s install
```

> Первый зарегистрированный пользователь автоматически получает права **Создателя**.

---

## Управление сервером и обновление

После установки в системе доступны команды `dcms` и `dcms-update`:

| Команда | Описание |
|---|---|
| `sudo dcms-update` | Обновить сервер из `https://github.com/Karatells/dcms` (с бэкапом БД/файлов и автооткатом при ошибке) |
| `sudo dcms domain <домен> [email] [--no-ssl]` | Подключить или сменить домен, выпустить **Let's Encrypt**, включить автопродление и 301-редирект с IP |
| `sudo dcms migrate-proxy [домен] [email] [--port 8080] [--no-ssl] [--force]` | Перевести существующую установку на схему `nginx → 127.0.0.1:PORT` или перейти с IP (80 порт) на домен |
| `sudo dcms build-clients [флаги]` | Собрать клиенты (Android APK, Windows, Linux) на сервере и опубликовать их на `/apps` |
| `sudo dcms rollback` | Откатиться на предыдущий коммит |
| `sudo dcms backup` | Создать резервную копию БД (SQL-дамп) и каталога `/var/lib/dcms` |
| `sudo dcms status` / `dcms logs` | Статус и журнал службы `dcms` |
| `sudo dcms uninstall` | Удалить службу, nginx-конфиг и исходники (БД и загруженные файлы сохраняются) |

### Переход с IP (80 порт) на домен с HTTPS

Если изначально соцсеть работала по IP-адресу, в любой момент выполните:

```bash
sudo dcms migrate-proxy social.example.com admin@example.com
```

Скрипт сделает бэкап текущего `dcms.conf`, настроит `nginx` на домен, добавит 301-редирект со старого IP на `https://social.example.com`, выпустит сертификат **Let's Encrypt** и включит автоматическое продление (`systemd certbot.timer` или `/etc/cron.d/dcms-certbot`).

### Основные переменные окружения скрипта
`REPO` (`https://github.com/Karatells/dcms.git`), `BRANCH` (`main`), `DOMAIN`, `EMAIL`, `ENABLE_SSL` (`1`), `PORT` (`8080`), `GO_VERSION`, `DCMS_DSN`, `DB_NAME` (`dcms`), `DB_USER` (`dcms`), `DB_PASS`, `BUILD_CLIENTS` (`0` — при `1` клиенты автоматически пересобираются после `dcms-update`).

---

## Сборка клиентских приложений на сервере (`x86_64` и `arm64`)

Сборочный скрипт `deploy/build-clients.sh` сам устанавливает все недостающие зависимости (**Node.js 20**, **OpenJDK 17**, **Gradle 8.5**, **Android SDK** и нативный пакет `aapt` на `arm64`):

```bash
sudo dcms build-clients \
  --url https://social.example.com \
  --name "DCMS Social" \
  --linux-archs x64,arm64
```

Собранные пакеты (`dcms-android.apk`, `dcms-windows-x64.zip`, `dcms-linux-*.tar.gz`) и `manifest.json` сохраняются в `/var/lib/dcms/apps` и сразу появляются на странице **`/apps`**. Установленные клиенты автоматически проверяют новые версии через `/api/apps/check` и проверяют `SHA-256` перед установкой.

Пример постоянной конфигурации: `/opt/dcms/deploy/client.conf.example` → `/etc/dcms/client.conf`.

---

## Локальная разработка

```bash
git clone https://github.com/Karatells/dcms.git
cd dcms
```

Создайте базу данных в MySQL / MariaDB:

```sql
CREATE DATABASE dcms CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER 'dcms'@'127.0.0.1' IDENTIFIED BY 'dcms';
GRANT ALL PRIVILEGES ON dcms.* TO 'dcms'@'127.0.0.1';
FLUSH PRIVILEGES;
```

Запустите сервер в dev-режиме (фронтенд подхватывается с диска без пересборки):

```bash
DCMS_DEV=1 DCMS_DSN="dcms:dcms@tcp(127.0.0.1:3306)/dcms?charset=utf8mb4" \
  go run . -addr :8080 -data ./data -web web
```
