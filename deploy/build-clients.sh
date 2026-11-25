#!/usr/bin/env bash
# DCMS Social — серверная сборка клиентов (Android APK, Linux Desktop, Windows Desktop).
# Поддерживает работу на серверах x86_64 и arm64 (aarch64).
#
# Примеры:
#   ./deploy/build-clients.sh --url https://social.example.com --name "Моя Сеть"
#   ./deploy/build-clients.sh --url https://dcms.home --insecure-tls --targets android,linux,windows
#   sudo dcms build-clients --url https://dcms.home --insecure-tls
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# Подхватываем серверные конфиги, если они есть
[ -f /etc/dcms/install.conf ] && . /etc/dcms/install.conf
[ -f /etc/dcms/client.conf ] && . /etc/dcms/client.conf

# Значения по умолчанию
SERVER_URL="${SERVER_URL:-}"
if [ -z "$SERVER_URL" ] && [ -n "${DOMAIN:-}" ]; then
  SERVER_URL="https://${DOMAIN}"
fi
APP_NAME="${APP_NAME:-DCMS Social}"
APP_ID="${APP_ID:-org.dcms.app}"
TARGETS="${TARGETS:-android,linux,windows}"
ALLOW_INSECURE_TLS="${ALLOW_INSECURE_TLS:-false}"
LINUX_ARCHS="${LINUX_ARCHS:-}"
WIN_ARCHS="${WIN_ARCHS:-x64}"
OUT_DIR="${OUT_DIR:-}"
KEYSTORE_PATH="${KEYSTORE_PATH:-/etc/dcms/android-release.keystore}"
KEYSTORE_PASS="${KEYSTORE_PASS:-dcms-release-key}"
KEY_ALIAS="${KEY_ALIAS:-dcms}"
ICON_PATH="${ICON_PATH:-}"

while [ $# -gt 0 ]; do
  case "$1" in
    --url)          SERVER_URL="$2"; shift 2 ;;
    --name)         APP_NAME="$2"; shift 2 ;;
    --app-id)       APP_ID="$2"; shift 2 ;;
    --targets)      TARGETS="$2"; shift 2 ;;
    --linux-archs)  LINUX_ARCHS="$2"; shift 2 ;;
    --win-archs)    WIN_ARCHS="$2"; shift 2 ;;
    --out)          OUT_DIR="$2"; shift 2 ;;
    --icon)         ICON_PATH="$2"; shift 2 ;;
    --insecure-tls) ALLOW_INSECURE_TLS="true"; shift ;;
    -h|--help)
      sed -n '2,10p' "$0"
      echo
      echo "Флаги:"
      echo "  --url URL            URL сервера соцсети (например https://dcms.home)"
      echo "  --name NAME          Название приложения (по умолчанию: DCMS Social)"
      echo "  --app-id ID          ID пакета (по умолчанию: org.dcms.app)"
      echo "  --targets LIST       Цели через запятую: android,linux,windows"
      echo "  --linux-archs LIST   Архитектуры Linux: x64,arm64 (по умолчанию: архитектура хоста)"
      echo "  --win-archs LIST     Архитектуры Windows: x64,arm64 (по умолчанию: x64)"
      echo "  --out DIR            Папка для артефактов (по умолчанию: /var/lib/dcms/apps или ./data/apps)"
      echo "  --icon FILE          Своя PNG-иконка 512x512"
      echo "  --insecure-tls       Разрешить самоподписанный TLS-сертификат для этого домена"
      exit 0 ;;
    *) echo "Неизвестный параметр: $1" >&2; exit 1 ;;
  esac
done

c_ok=$'\e[32m'; c_err=$'\e[31m'; c_inf=$'\e[36m'; c_wrn=$'\e[33m'; c_off=$'\e[0m'
info() { echo "${c_inf}==>${c_off} $*"; }
ok()   { echo "${c_ok}✓${c_off} $*"; }
warn() { echo "${c_wrn}!${c_off} $*" >&2; }
die()  { echo "${c_err}✗ $*${c_off}" >&2; exit 1; }

[ -n "$SERVER_URL" ] || die "Укажите --url https://ваш-домен (или SERVER_URL в /etc/dcms/client.conf)"
SERVER_URL="${SERVER_URL%/}"

if [ -z "$OUT_DIR" ]; then
  if [ -d /var/lib/dcms ] && [ -w /var/lib/dcms ]; then
    OUT_DIR="/var/lib/dcms/apps"
  else
    OUT_DIR="$REPO_DIR/data/apps"
  fi
fi
mkdir -p "$OUT_DIR"

HOST_UNAME="$(uname -m)"
case "$HOST_UNAME" in
  x86_64|amd64)  HOST_ARCH="x64" ;;
  aarch64|arm64) HOST_ARCH="arm64" ;;
  *)             HOST_ARCH="$HOST_UNAME" ;;
esac
[ -n "$LINUX_ARCHS" ] || LINUX_ARCHS="$HOST_ARCH"

GIT_REV="$(git -C "$REPO_DIR" rev-parse --short HEAD 2>/dev/null || echo 'dev')"
VERSION_NUM="$(git -C "$REPO_DIR" rev-list --count HEAD 2>/dev/null || echo '1')"
VERSION_NAME="1.0.${VERSION_NUM}"
BUILT_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

info "Сборка клиентов DCMS Social"
echo "  Сервер:      $SERVER_URL (insecure TLS: $ALLOW_INSECURE_TLS)"
echo "  Приложение:  $APP_NAME ($APP_ID) v$VERSION_NAME ($GIT_REV)"
echo "  Хост:        Linux $HOST_UNAME ($HOST_ARCH)"
echo "  Цели:        $TARGETS"
echo "  Вывод:       $OUT_DIR"

has_target() {
  echo ",$TARGETS," | grep -qi ",$1,"
}

# ─────────────────────────────── ANDROID APK
build_android() {
  info "Сборка Android APK…"
  local sdk="${ANDROID_HOME:-${ANDROID_SDK_ROOT:-}}"
  for p in /opt/android-sdk /usr/lib/android-sdk "$HOME/Android/Sdk"; do
    [ -z "$sdk" ] && [ -d "$p" ] && sdk="$p"
  done
  [ -n "$sdk" ] && [ -d "$sdk" ] || { warn "Android SDK не найден (установите ANDROID_HOME) — пропуск Android"; return 1; }
  export ANDROID_HOME="$sdk"
  export ANDROID_SDK_ROOT="$sdk"

  command -v java >/dev/null || { warn "JDK не найден — пропуск Android"; return 1; }
  local gradle_bin=""
  command -v gradle >/dev/null && gradle_bin="$(command -v gradle)"
  [ -n "$gradle_bin" ] || { warn "gradle не найден — пропуск Android"; return 1; }

  local work_dir="$REPO_DIR/clients/android"
  [ -d "$work_dir" ] || die "Не найден $work_dir"

  # Генерируем постоянный release keystore (один раз на сервере)
  local ks_args=()
  if command -v keytool >/dev/null; then
    local ks_dir; ks_dir="$(dirname "$KEYSTORE_PATH")"
    if mkdir -p "$ks_dir" 2>/dev/null && [ -w "$ks_dir" ]; then
      if [ ! -f "$KEYSTORE_PATH" ]; then
        info "Генерирую ключ подписи Android ($KEYSTORE_PATH)…"
        keytool -genkeypair -v -keystore "$KEYSTORE_PATH" -storepass "$KEYSTORE_PASS" \
          -alias "$KEY_ALIAS" -keypass "$KEYSTORE_PASS" -keyalg RSA -keysize 2048 -validity 10000 \
          -dname "CN=${APP_NAME}, O=DCMS, C=UN" >/dev/null 2>&1 || warn "Не удалось создать keystore — используется debug-подпись"
      fi
      if [ -f "$KEYSTORE_PATH" ]; then
        ks_args+=("-PkeystorePath=$KEYSTORE_PATH" "-PkeystorePass=$KEYSTORE_PASS" "-PkeyAlias=$KEY_ALIAS" "-PkeyPass=$KEYSTORE_PASS")
      fi
    fi
  fi

  local extra_args=()
  # На arm64 Linux Maven-версия aapt2 (x86_64) не запускается — используем нативный системный /usr/bin/aapt2
  if [ "$HOST_ARCH" = "arm64" ] && [ -x /usr/bin/aapt2 ]; then
    extra_args+=("-Pandroid.aapt2FromMavenOverride=/usr/bin/aapt2")
  fi

  if [ -n "$ICON_PATH" ] && [ -f "$ICON_PATH" ]; then
    cp -f "$ICON_PATH" "$work_dir/app/src/main/res/mipmap-xxxhdpi/ic_launcher.png"
    cp -f "$ICON_PATH" "$work_dir/app/src/main/res/mipmap-xxxhdpi/ic_launcher_round.png"
  fi

  (
    cd "$work_dir"
    "$gradle_bin" assembleRelease --no-daemon \
      "-PserverUrl=$SERVER_URL" \
      "-PappName=$APP_NAME" \
      "-PappId=$APP_ID" \
      "-PallowInsecureTls=$ALLOW_INSECURE_TLS" \
      "-PversionName=$VERSION_NAME" \
      "-PversionCode=$VERSION_NUM" \
      "${ks_args[@]}" \
      "${extra_args[@]}"
  )

  local apk
  apk="$(find "$work_dir/app/build/outputs/apk/release" -name '*.apk' | head -n1)"
  [ -f "$apk" ] || { warn "APK не найден после сборки"; return 1; }
  cp -f "$apk" "$OUT_DIR/dcms-android.apk"
  ok "Android APK: $OUT_DIR/dcms-android.apk ($(du -h "$OUT_DIR/dcms-android.apk" | awk '{print $1}'))"
}

# ─────────────────────────────── ELECTRON DESKTOP (Linux / Windows)
build_desktop() {
  local want_linux="$1"
  local want_win="$2"
  command -v node >/dev/null && command -v npm >/dev/null || { warn "Node.js/npm не найден — пропуск Desktop"; return 1; }

  local src_dir="$REPO_DIR/clients/desktop"
  local cache_dir="${DCMS_CLIENT_CACHE:-/var/cache/dcms-desktop-build}"
  if ! mkdir -p "$cache_dir" 2>/dev/null || [ ! -w "$cache_dir" ]; then
    cache_dir="/tmp/dcms-desktop-build"
    mkdir -p "$cache_dir"
  fi

  info "Подготовка Electron-проекта ($cache_dir)…"
  cp -f "$src_dir/package.json" "$cache_dir/package.json"
  rm -rf "$cache_dir/src" "$cache_dir/assets" "$cache_dir/dist"
  cp -r "$src_dir/src" "$src_dir/assets" "$cache_dir/"
  if [ -n "$ICON_PATH" ] && [ -f "$ICON_PATH" ]; then
    cp -f "$ICON_PATH" "$cache_dir/assets/icon.png"
  fi

  # Генерируем config.json и обновляем метаданные package.json под домен/название
  node - "$cache_dir" "$SERVER_URL" "$APP_NAME" "$APP_ID" "$ALLOW_INSECURE_TLS" "$VERSION_NAME" <<'NODEEOF'
const fs = require('fs');
const path = require('path');
const [dir, serverUrl, appName, appId, insecure, ver] = process.argv.slice(2);
fs.writeFileSync(path.join(dir, 'config.json'), JSON.stringify({
  serverUrl,
  appName,
  appId,
  allowInsecureTls: insecure === 'true' || insecure === '1',
}, null, 2));
const pkgPath = path.join(dir, 'package.json');
const pkg = JSON.parse(fs.readFileSync(pkgPath, 'utf8'));
pkg.version = ver;
pkg.productName = appName;
pkg.build = pkg.build || {};
pkg.build.appId = appId;
pkg.build.productName = appName;
fs.writeFileSync(pkgPath, JSON.stringify(pkg, null, 2));
NODEEOF

  if [ ! -d "$cache_dir/node_modules/electron-builder" ]; then
    info "Устанавливаю зависимости Electron (однократно в кэш)…"
    ( cd "$cache_dir" && npm install --no-audit --no-fund )
  fi

  if [ "$want_linux" = 1 ]; then
    IFS=',' read -ra LARCHS <<< "$LINUX_ARCHS"
    for arch in "${LARCHS[@]}"; do
      arch="$(echo "$arch" | xargs)"
      [ -n "$arch" ] || continue
      info "Сборка Linux Desktop ($arch)…"
      rm -rf "$cache_dir/dist"
      ( cd "$cache_dir" && npx electron-builder --linux tar.gz "--$arch" --publish never )
      local f
      f="$(find "$cache_dir/dist" -maxdepth 1 -name '*.tar.gz' | head -n1)"
      if [ -f "$f" ]; then
        cp -f "$f" "$OUT_DIR/dcms-linux-${arch}.tar.gz"
        ok "Linux ($arch): $OUT_DIR/dcms-linux-${arch}.tar.gz ($(du -h "$OUT_DIR/dcms-linux-${arch}.tar.gz" | awk '{print $1}'))"
      fi
    done
  fi

  if [ "$want_win" = 1 ]; then
    IFS=',' read -ra WARCHS <<< "$WIN_ARCHS"
    for arch in "${WARCHS[@]}"; do
      arch="$(echo "$arch" | xargs)"
      [ -n "$arch" ] || continue
      info "Сборка Windows Desktop ($arch)…"
      rm -rf "$cache_dir/dist"
      # zip-сборка не требует Wine и стабильно работает даже на arm64 Linux-сервере
      ( cd "$cache_dir" && npx electron-builder --win zip "--$arch" --publish never )
      local f
      f="$(find "$cache_dir/dist" -maxdepth 1 -name '*.zip' | head -n1)"
      if [ -f "$f" ]; then
        cp -f "$f" "$OUT_DIR/dcms-windows-${arch}.zip"
        ok "Windows ($arch): $OUT_DIR/dcms-windows-${arch}.zip ($(du -h "$OUT_DIR/dcms-windows-${arch}.zip" | awk '{print $1}'))"
      fi
    done
  fi
}

# ─────────────────────────────── ЗАПУСК ЦЕЛЕЙ
FAILED=0
if has_target android; then
  build_android || FAILED=1
fi
WANT_LINUX=0; has_target linux && WANT_LINUX=1
WANT_WIN=0;   has_target windows && WANT_WIN=1
if [ "$WANT_LINUX" = 1 ] || [ "$WANT_WIN" = 1 ]; then
  build_desktop "$WANT_LINUX" "$WANT_WIN" || FAILED=1
fi

# ─────────────────────────────── MANIFEST.JSON ДЛЯ /apps
node - "$OUT_DIR" "$APP_NAME" "$SERVER_URL" "$VERSION_NAME" "$GIT_REV" "$BUILT_AT" <<'NODEEOF'
const fs = require('fs');
const path = require('path');
const crypto = require('crypto');
const [outDir, appName, serverUrl, version, rev, builtAt] = process.argv.slice(2);
const known = [
  { file: 'dcms-android.apk', platform: 'android', arch: 'universal', label: 'Android (APK)', note: 'Android 7.0 и выше' },
  { file: 'dcms-windows-x64.zip', platform: 'windows', arch: 'x64', label: 'Windows (64-bit)', note: 'Распакуйте архив и запустите .exe' },
  { file: 'dcms-windows-arm64.zip', platform: 'windows', arch: 'arm64', label: 'Windows (ARM64)', note: 'Для Windows на ARM' },
  { file: 'dcms-linux-x64.tar.gz', platform: 'linux', arch: 'x64', label: 'Linux (x86_64)', note: 'Распакуйте и запустите dcms-desktop' },
  { file: 'dcms-linux-arm64.tar.gz', platform: 'linux', arch: 'arm64', label: 'Linux (ARM64)', note: 'Для ARM64 (Raspberry Pi, серверы, ноуты)' },
];
const items = [];
for (const k of known) {
  const full = path.join(outDir, k.file);
  if (!fs.existsSync(full)) continue;
  const st = fs.statSync(full);
  const sha = crypto.createHash('sha256').update(fs.readFileSync(full)).digest('hex');
  items.push({ ...k, size: st.size, sha256: sha, updated_at: Math.floor(st.mtimeMs / 1000) });
}
fs.writeFileSync(path.join(outDir, 'manifest.json'), JSON.stringify({
  app_name: appName, server_url: serverUrl, version, rev, built_at: builtAt, items
}, null, 2));
NODEEOF

# Права для пользователя сервиса dcms (если запущено от root)
if id dcms >/dev/null 2>&1; then
  chown -R dcms:dcms "$OUT_DIR" 2>/dev/null || true
fi

ok "Манифест обновлён: $OUT_DIR/manifest.json"
exit "$FAILED"
