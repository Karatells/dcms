package account

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"dcms/internal/core"
	"dcms/internal/store"
)

// MFADevice — привязанное доверенное Android-устройство для подтверждения 2FA.
// Приватный ключ ECDSA P-256 хранится исключительно в аппаратном AndroidKeyStore телефона.
type MFADevice struct {
	store.Base
	UserID     int64  `json:"user_id"`
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	PubKeyB64  string `json:"pub_key"`
	CreatedAt  int64  `json:"created_at"`
	LastUsedAt int64  `json:"last_used_at"`
}

type mfaEnrollNonce struct {
	userID int64
	exp    time.Time
}

type mfaLoginChallenge struct {
	id          string
	userID      int64
	nonce       string
	matchCode   string
	choices     []string
	ip          string
	deviceLabel string
	status      string // pending | approved | denied
	tries       int
	createdAt   time.Time
	exp         time.Time
}

var (
	mfaDevices *store.Table[MFADevice, *MFADevice]
	mfaMu      sync.Mutex
	mfaEnrolls = map[string]mfaEnrollNonce{}
	mfaLogins  = map[string]*mfaLoginChallenge{}
)

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func randIntRange(minVal, maxVal int) int {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(maxVal-minVal+1)))
	if err != nil {
		return minVal
	}
	return minVal + int(n.Int64())
}

func makeNumberChoices(correct string) []string {
	seen := map[string]bool{correct: true}
	out := []string{correct}
	for len(out) < 3 {
		v := fmt.Sprintf("%02d", randIntRange(10, 99))
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	// Криптографически перемешиваем 3 варианта
	for i := len(out) - 1; i > 0; i-- {
		j := randIntRange(0, i)
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func parseECDSAP256Pub(pubB64 string) (*ecdsa.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(pubB64))
	if err != nil {
		return nil, fmt.Errorf("некорректный base64 публичного ключа")
	}
	pubAny, err := x509.ParsePKIXPublicKey(raw)
	if err != nil {
		return nil, fmt.Errorf("некорректный формат публичного ключа")
	}
	pub, ok := pubAny.(*ecdsa.PublicKey)
	if !ok || pub.Curve != elliptic.P256() {
		return nil, fmt.Errorf("требуется ключ ECDSA P-256")
	}
	return pub, nil
}

func verifyECDSASig(pubB64, payload, sigB64 string) bool {
	pub, err := parseECDSAP256Pub(pubB64)
	if err != nil {
		return false
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(sigB64))
	if err != nil || len(sig) < 8 {
		return false
	}
	sum := sha256.Sum256([]byte(payload))
	return ecdsa.VerifyASN1(pub, sum[:], sig)
}

func describeLoginClient(ua string) string {
	u := strings.ToLower(ua)
	app := "Браузер"
	switch {
	case strings.Contains(u, "dcmsandroid"):
		app = "DCMS Android"
	case strings.Contains(u, "electron"):
		app = "DCMS Desktop"
	case strings.Contains(u, "edg/"):
		app = "Edge"
	case strings.Contains(u, "firefox"):
		app = "Firefox"
	case strings.Contains(u, "chrome") && !strings.Contains(u, "edg/"):
		app = "Chrome"
	case strings.Contains(u, "safari") && !strings.Contains(u, "chrome"):
		app = "Safari"
	}
	osName := "Устройство"
	switch {
	case strings.Contains(u, "windows"):
		osName = "Windows"
	case strings.Contains(u, "android"):
		osName = "Android"
	case strings.Contains(u, "iphone") || strings.Contains(u, "ipad"):
		osName = "iOS"
	case strings.Contains(u, "mac os") || strings.Contains(u, "macintosh"):
		osName = "macOS"
	case strings.Contains(u, "linux"):
		osName = "Linux"
	}
	return app + " · " + osName
}

func cleanupMFALocked(now time.Time) {
	for k, v := range mfaEnrolls {
		if now.After(v.exp) {
			delete(mfaEnrolls, k)
		}
	}
	for k, v := range mfaLogins {
		if now.After(v.exp) {
			delete(mfaLogins, k)
		}
	}
}

func userHasMFADevices(uid int64) bool {
	if mfaDevices == nil {
		return false
	}
	return mfaDevices.Count(func(d *MFADevice) bool { return d.UserID == uid }) > 0
}

func createMFALoginChallenge(a *core.App, uid int64, ip, ua string) *mfaLoginChallenge {
	now := time.Now()
	code := fmt.Sprintf("%02d", randIntRange(10, 99))
	ch := &mfaLoginChallenge{
		id:          randHex(20),
		userID:      uid,
		nonce:       randHex(20),
		matchCode:   code,
		choices:     makeNumberChoices(code),
		ip:          ip,
		deviceLabel: describeLoginClient(ua),
		status:      "pending",
		createdAt:   now,
		exp:         now.Add(2 * time.Minute),
	}
	mfaMu.Lock()
	cleanupMFALocked(now)
	mfaLogins[ch.id] = ch
	mfaMu.Unlock()

	if a != nil && a.Hub != nil {
		a.Hub.ToUser(uid, "mfa_request", ch.publicForApp())
	}
	return ch
}

func (ch *mfaLoginChallenge) publicForApp() map[string]any {
	return map[string]any{
		"challenge_id": ch.id,
		"nonce":        ch.nonce,
		"ip":           ch.ip,
		"device_label": ch.deviceLabel,
		"choices":      ch.choices,
		"expires_at":   ch.exp.Unix(),
	}
}

func registerAppMFA(a *core.App) {
	mfaDevices = store.NewTable[MFADevice](a.DB, "mfa_devices")

	// Список привязанных Android-устройств
	a.GET("/api/2fa/app/devices", func(c *core.Ctx) error {
		u, err := c.Need()
		if err != nil {
			return err
		}
		list := mfaDevices.Find(func(d *MFADevice) bool { return d.UserID == u.ID })
		items := make([]map[string]any, 0, len(list))
		for _, d := range list {
			items = append(items, map[string]any{
				"id":           d.ID,
				"device_id":    d.DeviceID,
				"device_name":  d.DeviceName,
				"created_at":   d.CreatedAt,
				"last_used_at": d.LastUsedAt,
			})
		}
		return c.JSON(map[string]any{"items": items})
	}, core.Auth)

	// Шаг 1 привязки: выдача одноразового nonce для подписи в AndroidKeyStore
	a.POST("/api/2fa/app/enroll/start", func(c *core.Ctx) error {
		u, err := c.Need()
		if err != nil {
			return err
		}
		if !strings.Contains(c.R.UserAgent(), "DcmsAndroid/") {
			return core.ErrForbidden("Привязка доступна только в мобильном приложении Android")
		}
		nonce := randHex(20)
		now := time.Now()
		mfaMu.Lock()
		cleanupMFALocked(now)
		mfaEnrolls[nonce] = mfaEnrollNonce{userID: u.ID, exp: now.Add(3 * time.Minute)}
		mfaMu.Unlock()
		return c.JSON(map[string]any{"nonce": nonce})
	}, core.Auth)

	// Шаг 2 привязки: проверка пароля + ECDSA-подписи AndroidKeyStore и сохранение публичного ключа
	a.POST("/api/2fa/app/enroll/finish", func(c *core.Ctx) error {
		u, err := c.Need()
		if err != nil {
			return err
		}
		if !strings.Contains(c.R.UserAgent(), "DcmsAndroid/") {
			return core.ErrForbidden("Привязка доступна только в мобильном приложении Android")
		}
		var in struct {
			Pass       string `json:"pass"`
			DeviceID   string `json:"device_id"`
			DeviceName string `json:"device_name"`
			PubKey     string `json:"pub_key"`
			Nonce      string `json:"nonce"`
			Signature  string `json:"signature"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if !core.CheckPassword(in.Pass, u.PassHash) {
			return core.ErrBad("Неверный текущий пароль")
		}
		devID := strings.TrimSpace(in.DeviceID)
		if len(devID) < 8 || len(devID) > 128 {
			return core.ErrBad("Некорректный идентификатор устройства")
		}
		devName := core.Trunc(strings.TrimSpace(in.DeviceName), 80)
		if devName == "" {
			devName = "Android-устройство"
		}
		now := time.Now()
		mfaMu.Lock()
		en, ok := mfaEnrolls[in.Nonce]
		delete(mfaEnrolls, in.Nonce)
		mfaMu.Unlock()
		if !ok || en.userID != u.ID || now.After(en.exp) {
			return core.ErrBad("Сессия привязки истекла, попробуйте снова")
		}
		if _, err := parseECDSAP256Pub(in.PubKey); err != nil {
			return core.ErrBad(err.Error())
		}
		payload := "dcms-enroll-v1:" + devID + ":" + in.Nonce
		if !verifyECDSASig(in.PubKey, payload, in.Signature) {
			return core.ErrForbidden("Недействительная криптографическая подпись устройства")
		}

		existing := mfaDevices.Find(func(d *MFADevice) bool { return d.UserID == u.ID && d.DeviceID == devID })
		nowUnix := core.Now()
		if len(existing) > 0 {
			mfaDevices.Update(existing[0].ID, func(d *MFADevice) {
				d.DeviceName = devName
				d.PubKeyB64 = strings.TrimSpace(in.PubKey)
				d.LastUsedAt = nowUnix
			})
			return c.OK("id", existing[0].ID)
		}
		id := mfaDevices.Insert(&MFADevice{
			UserID:     u.ID,
			DeviceID:   devID,
			DeviceName: devName,
			PubKeyB64:  strings.TrimSpace(in.PubKey),
			CreatedAt:  nowUnix,
			LastUsedAt: nowUnix,
		})
		return c.OK("id", id)
	}, core.Auth)

	// Удалить (отвязать) доверенное устройство (требуется пароль)
	a.POST("/api/2fa/app/devices/{id}/delete", func(c *core.Ctx) error {
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
		id := c.PathInt("id")
		dev, ok := mfaDevices.Get(id)
		if !ok || dev.UserID != u.ID {
			return core.ErrNotFound("Устройство не найдено")
		}
		mfaDevices.Delete(id)
		return c.OK()
	}, core.Auth)

	// Активные запросы на подтверждение входа (fallback к SSE для Android-приложения)
	a.GET("/api/2fa/app/pending", func(c *core.Ctx) error {
		u, err := c.Need()
		if err != nil {
			return err
		}
		now := time.Now()
		mfaMu.Lock()
		cleanupMFALocked(now)
		var out []map[string]any
		for _, ch := range mfaLogins {
			if ch.userID == u.ID && ch.status == "pending" && now.Before(ch.exp) {
				out = append(out, ch.publicForApp())
			}
		}
		mfaMu.Unlock()
		if out == nil {
			out = []map[string]any{}
		}
		return c.JSON(map[string]any{"items": out})
	}, core.Auth)

	// Подтвердить или отклонить вход (только при валидной ECDSA-подписи AndroidKeyStore!)
	a.POST("/api/2fa/app/respond", func(c *core.Ctx) error {
		u, err := c.Need()
		if err != nil {
			return err
		}
		var in struct {
			ChallengeID string `json:"challenge_id"`
			DeviceID    string `json:"device_id"`
			Action      string `json:"action"` // approve | deny
			MatchCode   string `json:"match_code"`
			Signature   string `json:"signature"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if in.Action != "approve" && in.Action != "deny" {
			return core.ErrBad("Некорректное действие")
		}
		devs := mfaDevices.Find(func(d *MFADevice) bool { return d.UserID == u.ID && d.DeviceID == strings.TrimSpace(in.DeviceID) })
		if len(devs) == 0 {
			return core.ErrForbidden("Это устройство не привязано к аккаунту для подтверждения 2FA")
		}
		dev := devs[0]

		now := time.Now()
		mfaMu.Lock()
		defer mfaMu.Unlock()
		cleanupMFALocked(now)
		ch, ok := mfaLogins[in.ChallengeID]
		if !ok || ch.userID != u.ID || ch.status != "pending" || now.After(ch.exp) {
			return core.ErrBad("Запрос на вход уже устарел или обработан")
		}

		// Криптографическая проверка подписи AndroidKeyStore
		payload := fmt.Sprintf("dcms-2fa-v1:%s:%s:%s:%s", ch.id, in.Action, strings.TrimSpace(in.MatchCode), ch.nonce)
		if !verifyECDSASig(dev.PubKeyB64, payload, in.Signature) {
			return core.ErrForbidden("Недействительная подпись мобильного устройства")
		}

		if in.Action == "deny" {
			ch.status = "denied"
			mfaDevices.Update(dev.ID, func(d *MFADevice) { d.LastUsedAt = core.Now() })
			return c.OK("status", "denied")
		}

		// Проверка Number Matching (защита от случайного подтверждения / MFA fatigue)
		if strings.TrimSpace(in.MatchCode) != ch.matchCode {
			ch.tries++
			if ch.tries >= 2 {
				ch.status = "denied"
				return core.ErrBad("Неверное число. Запрос на вход отклонён в целях безопасности")
			}
			return core.ErrBad("Выбрано неверное число подтверждения")
		}

		ch.status = "approved"
		mfaDevices.Update(dev.ID, func(d *MFADevice) { d.LastUsedAt = core.Now() })
		return c.OK("status", "approved")
	}, core.Auth)

	// Опрос статуса подтверждения со стороны экрана входа
	a.POST("/api/login/poll", func(c *core.Ctx) error {
		var in struct {
			ChallengeID string `json:"challenge_id"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		now := time.Now()
		mfaMu.Lock()
		cleanupMFALocked(now)
		ch, ok := mfaLogins[in.ChallengeID]
		if !ok || now.After(ch.exp) {
			delete(mfaLogins, in.ChallengeID)
			mfaMu.Unlock()
			return c.JSON(map[string]any{"status": "expired", "error": "Время подтверждения истекло"})
		}
		st := ch.status
		uid := ch.userID
		if st == "approved" || st == "denied" {
			delete(mfaLogins, in.ChallengeID)
		}
		mfaMu.Unlock()

		if st == "pending" {
			return c.JSON(map[string]any{"status": "pending"})
		}
		if st == "denied" {
			return c.JSON(map[string]any{"status": "denied", "error": "Вход отклонён в мобильном приложении"})
		}
		u, ok := a.T.Users.Get(uid)
		if !ok {
			return core.ErrBad("Пользователь не найден")
		}
		a.Login(c, u.ID)
		collisions(a, c, u)
		return c.JSON(map[string]any{"status": "approved", "id": u.ID})
	})
}
