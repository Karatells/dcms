#!/usr/bin/env bash
# DCMS Social — установка и обновление на чистом Linux-сервере (Debian/Ubuntu/RHEL-семейство, нужен systemd).
#
#   Установка (одной командой, от root):
#     curl -fsSL https://git.home/admin/dcms-reborn/raw/branch/main/deploy/dcms.sh | sudo bash -s install
#   Обновление (после установки доступна команда `dcms-update`):
#     sudo dcms-update            # то же, что dcms.sh update
#   Прочее:
#     dcms.sh status | logs | backup | rollback | uninstall
#
# Переменные окружения (все необязательны):
#   REPO        адрес git-репозитория (по умолчанию https://git.home/admin/dcms-reborn.git)
#   BRANCH      ветка (по умолчанию main)
#   DOMAIN      домен — включит nginx-прокси
#   EMAIL       e-mail для Let's Encrypt (вместе с DOMAIN → HTTPS)
#   PORT        порт приложения (8080)
#   GO_VERSION  версия Go, если системной нет или она старая
#   DCMS_DSN    готовый DSN внешнего MySQL/MariaDB (если не задан — ставится локальная MariaDB)
#   DB_NAME     имя БД (dcms)        DB_USER  пользователь БД (dcms)        DB_PASS  пароль БД (автогенерация)
set -euo pipefail

APP=dcms
USER_NAME=dcms
APP_DIR=/opt/dcms                 # исходники (git) и собранный бинарник
DATA_DIR=/var/lib/dcms            # загруженные файлы пользователей (uploads)
BACKUP_DIR=/var/backups/dcms
ENV_FILE=/etc/dcms/dcms.env
UNIT=/etc/systemd/system/dcms.service
GO_MIN="1.24"
GO_VERSION="${GO_VERSION:-1.24.6}"
REPO="${REPO:-https://git.home/admin/dcms-reborn.git}"
BRANCH="${BRANCH:-main}"
PORT="${PORT:-8080}"
DB_NAME="${DB_NAME:-dcms}"
DB_USER="${DB_USER:-dcms}"
DB_HOST="${DB_HOST:-127.0.0.1}"
DB_PORT="${DB_PORT:-3306}"

c_ok=$'\e[32m'; c_err=$'\e[31m'; c_inf=$'\e[36m'; c_off=$'\e[0m'
info() { echo "${c_inf}==>${c_off} $*"; }
ok()   { echo "${c_ok}✓${c_off} $*"; }
die()  { echo "${c_err}✗ $*${c_off}" >&2; exit 1; }

need_root() { [ "$(id -u)" = 0 ] || die "Запустите от root (sudo)."; }

pkg_install() {
  if command -v apt-get >/dev/null; then DEBIAN_FRONTEND=noninteractive apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq "$@"
  elif command -v dnf >/dev/null; then dnf install -y -q "$@"
  elif command -v yum >/dev/null; then yum install -y -q "$@"
  elif command -v apk >/dev/null; then die "Alpine не поддерживается (нужен systemd)"
  else die "Не найден пакетный менеджер (apt/dnf/yum)"; fi
}

ver_ge() { [ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | head -n1)" = "$2" ]; }  # ver_ge A B  → A >= B

ensure_go() {
  local cur=""
  for g in /usr/local/go/bin/go "$(command -v go || true)"; do
    [ -x "${g:-/nonexistent}" ] && cur="$($g version | awk '{print $3}' | sed 's/^go//')" && GO_BIN="$g" && break
  done
  if [ -n "$cur" ] && ver_ge "$cur" "$GO_MIN"; then ok "Go $cur"; return; fi
  info "Устанавливаю Go $GO_VERSION…"
  local arch; case "$(uname -m)" in x86_64|amd64) arch=amd64;; aarch64|arm64) arch=arm64;; armv7l) arch=armv6l;; *) die "Архитектура $(uname -m) не поддерживается";; esac
  curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${arch}.tar.gz" -o /tmp/go.tgz || die "Не удалось скачать Go"
  rm -rf /usr/local/go && tar -C /usr/local -xzf /tmp/go.tgz && rm -f /tmp/go.tgz
  GO_BIN=/usr/local/go/bin/go; ok "Go $($GO_BIN version | awk '{print $3}')"
}

ensure_mariadb() {
  if [ -n "${DCMS_DSN:-}" ]; then
    ok "Используется внешний MySQL/MariaDB (DCMS_DSN)"
    return 0
  fi
  info "Настраиваю MariaDB…"
  if ! command -v mariadbd >/dev/null && ! command -v mysqld >/dev/null; then
    if command -v apt-get >/dev/null; then
      pkg_install mariadb-server mariadb-client
    else
      pkg_install mariadb-server mariadb
    fi
  fi
  systemctl daemon-reload
  if systemctl cat mariadb.service >/dev/null 2>&1; then
    systemctl enable --now mariadb
  elif systemctl cat mysql.service >/dev/null 2>&1; then
    systemctl enable --now mysql
  else
    die "Не найдена systemd-служба mariadb/mysql"
  fi
  local db_pass="${DB_PASS:-}"
  if [ -z "$db_pass" ]; then
    db_pass="$(od -An -tx1 -N16 /dev/urandom | tr -d ' \n')"
  fi
  local sql_bin; sql_bin="$(command -v mariadb || command -v mysql)"
  "$sql_bin" <<SQLEOF
CREATE DATABASE IF NOT EXISTS \`${DB_NAME}\` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS '${DB_USER}'@'localhost' IDENTIFIED BY '${db_pass}';
CREATE USER IF NOT EXISTS '${DB_USER}'@'127.0.0.1' IDENTIFIED BY '${db_pass}';
ALTER USER '${DB_USER}'@'localhost' IDENTIFIED BY '${db_pass}';
ALTER USER '${DB_USER}'@'127.0.0.1' IDENTIFIED BY '${db_pass}';
GRANT ALL PRIVILEGES ON \`${DB_NAME}\`.* TO '${DB_USER}'@'localhost';
GRANT ALL PRIVILEGES ON \`${DB_NAME}\`.* TO '${DB_USER}'@'127.0.0.1';
FLUSH PRIVILEGES;
SQLEOF
  DB_PASS="$db_pass"
  DCMS_DSN="${DB_USER}:${DB_PASS}@tcp(${DB_HOST}:${DB_PORT})/${DB_NAME}?charset=utf8mb4"
  ok "MariaDB готова (база: $DB_NAME, пользователь: $DB_USER)"
}

as_app() { runuser -u "$USER_NAME" -- "$@"; }

build() {
  info "Сборка…"
  ( cd "$APP_DIR" && CGO_ENABLED=0 HOME=/var/lib/dcms-build GOCACHE=/var/cache/dcms-go GOPATH=/var/cache/dcms-gopath "$GO_BIN" build -trimpath -ldflags "-s -w" -o "$APP_DIR/dcms.new" . ) \
    || die "Сборка не удалась"
  if [ -d "$APP_DIR/cmd/dcms-bot" ]; then
    ( cd "$APP_DIR" && CGO_ENABLED=0 HOME=/var/lib/dcms-build GOCACHE=/var/cache/dcms-go GOPATH=/var/cache/dcms-gopath "$GO_BIN" build -trimpath -ldflags "-s -w" -o /usr/local/bin/dcms-bot ./cmd/dcms-bot ) || true
  fi
  ok "Собрано: $(git -C "$APP_DIR" rev-parse --short HEAD)"
}

wait_health() {
  for _ in $(seq 1 30); do
    curl -fsS "http://127.0.0.1:${PORT}/api/config" >/dev/null 2>&1 && return 0; sleep 1
  done; return 1
}

write_unit() {
  cat > "$UNIT" <<UNITEOF
[Unit]
Description=DCMS Social
After=network.target mariadb.service mysql.service
Wants=mariadb.service

[Service]
User=$USER_NAME
Group=$USER_NAME
EnvironmentFile=$ENV_FILE
ExecStart=$APP_DIR/dcms -addr \${DCMS_ADDR} -data \${DCMS_DATA} -dsn \${DCMS_DSN}
Restart=always
RestartSec=2
TimeoutStopSec=30
LimitNOFILE=65536
NoNewPrivileges=true
ProtectSystem=full
ProtectHome=true
PrivateTmp=true
ReadWritePaths=$DATA_DIR

[Install]
WantedBy=multi-user.target
UNITEOF
  systemctl daemon-reload
}

setup_nginx() {
  [ -n "${DOMAIN:-}" ] || return 0
  info "Настраиваю nginx для $DOMAIN…"
  command -v nginx >/dev/null || pkg_install nginx
  local conf=/etc/nginx/conf.d/dcms.conf
  cat > "$conf" <<NGX
server {
    listen 80;
    server_name $DOMAIN;
    client_max_body_size 100m;
    location / {
        proxy_pass http://127.0.0.1:$PORT;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_buffering off;           # SSE (/api/events)
        proxy_read_timeout 1h;
    }
}
NGX
  nginx -t && systemctl enable --now nginx && systemctl reload nginx
  sed -i 's/^DCMS_TRUST_PROXY=.*/DCMS_TRUST_PROXY=1/' "$ENV_FILE" 2>/dev/null || true
  if [ -n "${EMAIL:-}" ]; then
    info "Получаю сертификат Let's Encrypt…"
    command -v certbot >/dev/null || pkg_install certbot python3-certbot-nginx
    certbot --nginx -d "$DOMAIN" -m "$EMAIL" --agree-tos -n --redirect || echo "Не удалось получить сертификат — HTTPS можно включить позже: certbot --nginx -d $DOMAIN"
  fi
}

cmd_install() {
  need_root
  [ -d "$APP_DIR/.git" ] && die "Уже установлено. Для обновления: dcms-update"
  info "Устанавливаю зависимости…"
  pkg_install git curl ca-certificates tar
  command -v ffmpeg >/dev/null || pkg_install ffmpeg || echo "Предупреждение: не удалось установить ffmpeg (нужен для сжатия коротких видео)" >&2
  ensure_go
  ensure_mariadb
  id "$USER_NAME" >/dev/null 2>&1 || useradd --system --home "$DATA_DIR" --shell /usr/sbin/nologin "$USER_NAME"
  mkdir -p "$APP_DIR" "$DATA_DIR" "$BACKUP_DIR" /etc/dcms /var/lib/dcms-build /var/cache/dcms-go /var/cache/dcms-gopath
  info "Клонирую $REPO ($BRANCH)…"
  git clone --branch "$BRANCH" "$REPO" "$APP_DIR" || die "Не удалось клонировать репозиторий $REPO"
  build; mv "$APP_DIR/dcms.new" "$APP_DIR/dcms"
  chown -R "$USER_NAME:$USER_NAME" "$DATA_DIR"
  chmod 700 "$DATA_DIR"
  cat > "$ENV_FILE" <<ENVEOF
DCMS_ADDR=$( [ -n "${DOMAIN:-}" ] && echo "127.0.0.1:$PORT" || echo ":$PORT" )
DCMS_DATA=$DATA_DIR
DCMS_DSN=$DCMS_DSN
DCMS_DB_HOST=$DB_HOST
DCMS_DB_PORT=$DB_PORT
DCMS_DB_NAME=$DB_NAME
DCMS_DB_USER=$DB_USER
DCMS_DB_PASS=${DB_PASS:-}
DCMS_TRUST_PROXY=0
ENVEOF
  chmod 640 "$ENV_FILE"; chgrp "$USER_NAME" "$ENV_FILE"
  write_unit
  # установка команд управления
  { echo '#!/usr/bin/env bash'; echo "exec $APP_DIR/deploy/dcms.sh update \"\$@\""; } > /usr/local/bin/dcms-update
  chmod +x /usr/local/bin/dcms-update "$APP_DIR/deploy/dcms.sh"
  ln -sf "$APP_DIR/deploy/dcms.sh" /usr/local/bin/dcms
  cat > /etc/dcms/install.conf <<CONFEOF
REPO=$REPO
BRANCH=$BRANCH
PORT=$PORT
CONFEOF
  systemctl enable --now dcms
  wait_health && ok "Сервис запущен" || { journalctl -u dcms -n 30 --no-pager; die "Сервис не отвечает"; }
  setup_nginx
  local ip; ip="$(curl -fsS https://api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}')"
  echo
  ok "Готово! Откройте: ${DOMAIN:+http://$DOMAIN  или }http://$ip:$PORT"
  echo "  Первый зарегистрированный пользователь становится Создателем (полные права)."
  echo "  Обновление: sudo dcms-update   ·   Логи: dcms logs   ·   Резервная копия: dcms backup"
}

cmd_backup() {
  need_root; mkdir -p "$BACKUP_DIR"
  local ts; ts="$(date +%Y%m%d-%H%M%S)"
  local tmp; tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' RETURN
  if [ -f "$ENV_FILE" ]; then
    set -a; . "$ENV_FILE"; set +a
  fi
  local dump_bin; dump_bin="$(command -v mariadb-dump || command -v mysqldump || true)"
  if [ -n "$dump_bin" ] && [ -n "${DCMS_DB_NAME:-}" ]; then
    local extra=()
    [ -n "${DCMS_DB_HOST:-}" ] && extra+=(-h "$DCMS_DB_HOST")
    [ -n "${DCMS_DB_PORT:-}" ] && extra+=(-P "$DCMS_DB_PORT")
    [ -n "${DCMS_DB_USER:-}" ] && extra+=(-u "$DCMS_DB_USER")
    MYSQL_PWD="${DCMS_DB_PASS:-}" "$dump_bin" --single-transaction --routines --triggers "${extra[@]}" "$DCMS_DB_NAME" > "$tmp/database.sql" 2>/dev/null \
      || "$dump_bin" --single-transaction "$DCMS_DB_NAME" > "$tmp/database.sql" 2>/dev/null \
      || echo "Предупреждение: не удалось сделать SQL-дамп базы $DCMS_DB_NAME" >&2
  fi
  local f="$BACKUP_DIR/dcms-$ts.tar.gz"
  if [ -f "$tmp/database.sql" ]; then
    tar -czf "$f" -C "$tmp" database.sql -C "$(dirname "$DATA_DIR")" "$(basename "$DATA_DIR")" && ok "Резервная копия (БД + файлы): $f"
  else
    tar -C "$(dirname "$DATA_DIR")" -czf "$f" "$(basename "$DATA_DIR")" && ok "Резервная копия (файлы): $f"
  fi
  ls -1t "$BACKUP_DIR"/dcms-*.tar.gz 2>/dev/null | tail -n +15 | xargs -r rm -f   # храним 14 последних
  echo "$f"
}

cmd_update() {
  need_root
  [ -d "$APP_DIR/.git" ] || die "Не установлено. Сначала: dcms.sh install"
  [ -f /etc/dcms/install.conf ] && . /etc/dcms/install.conf
  mkdir -p /var/cache/dcms-gopath
  ensure_go
  command -v ffmpeg >/dev/null || pkg_install ffmpeg || true
  # При обновлении со старой версии без MySQL — автоматически настраиваем MariaDB и DCMS_DSN
  if [ -f "$ENV_FILE" ] && ! grep -q '^DCMS_DSN=' "$ENV_FILE"; then
    info "Обнаружено обновление на MySQL/MariaDB — настраиваю БД…"
    ensure_mariadb
    cat >> "$ENV_FILE" <<ENVEOF
DCMS_DSN=$DCMS_DSN
DCMS_DB_HOST=$DB_HOST
DCMS_DB_PORT=$DB_PORT
DCMS_DB_NAME=$DB_NAME
DCMS_DB_USER=$DB_USER
DCMS_DB_PASS=${DB_PASS:-}
ENVEOF
  fi
  cd "$APP_DIR"
  git remote set-url origin "$REPO" 2>/dev/null || true
  git fetch --quiet origin "$BRANCH"
  local old new; old="$(git rev-parse HEAD)"; new="$(git rev-parse "origin/$BRANCH")"
  if [ "$old" = "$new" ] && [ "${FORCE:-0}" != 1 ]; then ok "Уже последняя версия ($(git rev-parse --short HEAD))"; return; fi
  info "Обновление ${old:0:7} → ${new:0:7}"
  git log --oneline "$old..$new" | head -15 || true
  local bk; bk="$(cmd_backup | tail -n1)"
  echo "$old" > "$APP_DIR/.prev_commit"
  git reset --hard --quiet "origin/$BRANCH"
  chmod +x deploy/dcms.sh
  if ! build; then git reset --hard --quiet "$old"; die "Сборка новой версии не удалась — осталась прежняя"; fi
  cp -f "$APP_DIR/dcms" "$APP_DIR/dcms.prev" 2>/dev/null || true
  systemctl stop dcms
  mv -f "$APP_DIR/dcms.new" "$APP_DIR/dcms"
  write_unit
  systemctl start dcms
  if wait_health; then ok "Обновлено до $(git rev-parse --short HEAD)"; else
    echo "${c_err}Новая версия не запустилась — откат…${c_off}"; journalctl -u dcms -n 20 --no-pager || true
    systemctl stop dcms || true
    [ -f "$APP_DIR/dcms.prev" ] && mv -f "$APP_DIR/dcms.prev" "$APP_DIR/dcms"
    git reset --hard --quiet "$old"
    systemctl start dcms
    die "Выполнен откат на ${old:0:7}. Резервная копия: $bk"
  fi
}

cmd_rollback() {
  need_root
  [ -f "$APP_DIR/.prev_commit" ] || die "Нет сохранённой предыдущей версии"
  local prev; prev="$(cat "$APP_DIR/.prev_commit")"
  cd "$APP_DIR"; ensure_go; git reset --hard --quiet "$prev"; build
  systemctl stop dcms; mv -f dcms.new dcms; systemctl start dcms
  wait_health && ok "Откат на ${prev:0:7} выполнен (данные и БД не менялись)" || die "Сервис не отвечает"
}

cmd_uninstall() {
  need_root
  read -r -p "Удалить сервис и код? Загруженные файлы ($DATA_DIR) и база данных MariaDB останутся. [y/N] " a </dev/tty; [ "$a" = y ] || exit 0
  systemctl disable --now dcms 2>/dev/null || true
  rm -f "$UNIT" /usr/local/bin/dcms /usr/local/bin/dcms-update /etc/nginx/conf.d/dcms.conf
  rm -rf "$APP_DIR" /etc/dcms; systemctl daemon-reload; ok "Удалено. Файлы сохранены в $DATA_DIR, база данных не тронута"
}

case "${1:-}" in
  install)   cmd_install ;;
  update)    cmd_update ;;
  backup)    cmd_backup >/dev/null ;;
  rollback)  cmd_rollback ;;
  status)    systemctl status dcms --no-pager; git -C "$APP_DIR" log -1 --oneline ;;
  logs)      journalctl -u dcms -f -n 100 ;;
  uninstall) cmd_uninstall ;;
  *) sed -n 2,19p "$0"; exit 1 ;;
esac
