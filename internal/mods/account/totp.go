package account

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"

	"dcms/internal/core"
	"dcms/internal/model"
)

func generateTOTPSecret() string {
	b := make([]byte, 20)
	_, _ = rand.Read(b)
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
}

func totpCodeAt(secret string, step int64) (string, bool) {
	sec := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(secret), " ", ""))
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(sec)
	if err != nil || len(key) == 0 {
		return "", false
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	bin := (uint32(sum[off]&0x7f) << 24) |
		(uint32(sum[off+1]) << 16) |
		(uint32(sum[off+2]) << 8) |
		uint32(sum[off+3])
	return fmt.Sprintf("%06d", bin%1000000), true
}

// verifyTOTP проверяет 6-значный код TOTP (RFC 6238, окно ±1 шаг по 30 сек).
func verifyTOTP(secret, code string) bool {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != 6 || secret == "" {
		return false
	}
	step := time.Now().Unix() / 30
	for delta := int64(-1); delta <= 1; delta++ {
		if want, ok := totpCodeAt(secret, step+delta); ok && hmac.Equal([]byte(want), []byte(code)) {
			return true
		}
	}
	return false
}

func registerTOTP(a *core.App) {
	// Состояние 2FA и генерация секрета для настройки
	a.GET("/api/settings/2fa", func(c *core.Ctx) error {
		u, err := c.Need()
		if err != nil {
			return err
		}
		if u.TOTPEnabled {
			return c.JSON(map[string]any{"enabled": true})
		}
		sec := u.TOTPSecret
		if sec == "" {
			sec = generateTOTPSecret()
			a.T.Users.Update(u.ID, func(x *model.User) { x.TOTPSecret = sec })
		}
		issuer := strings.TrimSpace(a.Cfg.Str("site_name"))
		if issuer == "" {
			issuer = "DCMS Social"
		}
		label := url.PathEscape(issuer + ":" + u.Nick)
		otpURL := "otpauth://totp/" + label + "?secret=" + sec + "&issuer=" + url.QueryEscape(issuer)
		return c.JSON(map[string]any{
			"enabled":     false,
			"secret":      sec,
			"otpauth_url": otpURL,
		})
	}, core.Auth)

	// Включить 2FA после подтверждения 6-значным кодом
	a.POST("/api/settings/2fa/enable", func(c *core.Ctx) error {
		u, err := c.Need()
		if err != nil {
			return err
		}
		if u.TOTPEnabled {
			return core.ErrBad("Двухфакторная аутентификация уже включена")
		}
		var in struct {
			Code string `json:"code"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if u.TOTPSecret == "" {
			return core.ErrBad("Сначала обновите страницу настроек для генерации ключа")
		}
		if !verifyTOTP(u.TOTPSecret, in.Code) {
			return core.ErrBad("Неверный 6-значный код. Проверьте время на устройстве")
		}
		a.T.Users.Update(u.ID, func(x *model.User) { x.TOTPEnabled = true })
		return c.OK("enabled", true)
	}, core.Auth)

	// Отключить 2FA (требуется текущий пароль)
	a.POST("/api/settings/2fa/disable", func(c *core.Ctx) error {
		u, err := c.Need()
		if err != nil {
			return err
		}
		var in struct {
			Pass string `json:"pass"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if !core.CheckPassword(in.Pass, u.PassHash) {
			return core.ErrBad("Неверный текущий пароль")
		}
		a.T.Users.Update(u.ID, func(x *model.User) {
			x.TOTPEnabled = false
			x.TOTPSecret = ""
		})
		return c.OK("enabled", false)
	}, core.Auth)
}
