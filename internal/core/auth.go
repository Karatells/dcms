package core

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"dcms/internal/model"
)

const pbkdf2Iter = 120000

// HashPassword возвращает PBKDF2-SHA256 хеш в формате pbkdf2$iter$salt$hash.
func HashPassword(pw string) string {
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	k, _ := pbkdf2.Key(sha256.New, pw, salt, pbkdf2Iter, 32)
	return fmt.Sprintf("pbkdf2$%d$%s$%s", pbkdf2Iter, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(k))
}

// CheckPassword проверяет пароль.
func CheckPassword(pw, enc string) bool {
	p := strings.Split(enc, "$")
	if len(p) != 4 || p[0] != "pbkdf2" {
		return false
	}
	it, _ := strconv.Atoi(p[1])
	salt, err1 := base64.RawStdEncoding.DecodeString(p[2])
	want, err2 := base64.RawStdEncoding.DecodeString(p[3])
	if err1 != nil || err2 != nil || it <= 0 {
		return false
	}
	k, _ := pbkdf2.Key(sha256.New, pw, salt, it, len(want))
	return subtle.ConstantTimeCompare(k, want) == 1
}

// RandToken — случайная hex-строка из n байт.
func RandToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

const cookieName = "dcms_sid"

// authenticate определяет пользователя по cookie, обновляет активность (как sys/inc/user.php).
func (a *App) authenticate(c *Ctx) {
	ck, err := c.R.Cookie(cookieName)
	if err != nil || ck.Value == "" {
		a.trackGuest(c)
		return
	}
	s, ok := a.T.Sessions.OneBy("token", ck.Value)
	if !ok {
		a.trackGuest(c)
		return
	}
	u, ok := a.T.Users.Get(s.UserID)
	if !ok || u.Activation != "" {
		a.trackGuest(c)
		return
	}
	now := Now()
	// обновляем активность не чаще раза в 20 с, чтобы не писать журнал на каждый запрос
	if now-u.DateLast >= 20 {
		a.T.Users.Update(u.ID, func(x *model.User) {
			d := now - x.DateLast
			if d < 120 {
				x.Time += d
			}
			x.DateLast = now
			x.IP = c.IP()
			if ua := c.R.UserAgent(); ua != "" {
				if len(ua) > 200 {
					ua = ua[:200]
				}
				x.UA = ua
			}
			x.Browser = browserKind(c.R.UserAgent())
		})
		a.T.Users.Update(u.ID, func(x *model.User) {
			if x.RatingTmp > 1000 {
				col := x.RatingTmp / 1000
				x.Rating += col
				x.RatingTmp -= col * 1000
			}
		})
		u, _ = a.T.Users.Get(u.ID)
		a.dropRef(u.ID)
	}
	if lv := a.groupLevel(u.GroupAccess); lv != u.Level {
		u.Level = lv
	}
	c.user = &u
	c.Sess = &s
}

func browserKind(ua string) string {
	l := strings.ToLower(ua)
	for _, k := range []string{"mobile", "android", "iphone", "opera mini", "midp"} {
		if strings.Contains(l, k) {
			return "wap"
		}
	}
	return "web"
}

func (a *App) trackGuest(c *Ctx) {
	if strings.HasPrefix(c.R.URL.Path, "/api/") && c.R.Method == http.MethodGet && !strings.HasPrefix(c.R.URL.Path, "/api/events") {
		ip := ipToInt(c.IP())
		ua := c.R.UserAgent()
		if len(ua) > 32 {
			ua = ua[:32]
		}
		now := Now()
		if g, ok := a.T.Guests.OneBy("key", fmt.Sprintf("%d|%s", ip, ua)); ok {
			if now-g.DateLast > 15 {
				a.T.Guests.Update(g.ID, func(x *model.Guests) { x.DateLast = now; x.Pereh++; x.Url = c.R.Header.Get("X-Page") })
			}
		} else {
			a.T.Guests.Insert(&model.Guests{IP: ip, Ua: ua, DateAut: now, DateLast: now})
		}
	}
}

func ipToInt(s string) int64 {
	var a, b, c, d int64
	if n, _ := fmt.Sscanf(s, "%d.%d.%d.%d", &a, &b, &c, &d); n == 4 {
		return a<<24 | b<<16 | c<<8 | d
	}
	return 0
}

// IPToInt — IPv4 → число (для бана по диапазонам, как ip2long); 0 для IPv6/ошибок.
func IPToInt(s string) int64 { return ipToInt(s) }

// Login создаёт сессию и ставит cookie.
func (a *App) Login(c *Ctx, uid int64) {
	now := Now()
	tok := RandToken(32)
	a.T.Sessions.Insert(&model.Session{Token: tok, UserID: uid, Created: now, Last: now, IP: c.IP(), UA: c.R.UserAgent()})
	secure := c.R.TLS != nil || (a.Cfg.Bool("trust_proxy") && c.R.Header.Get("X-Forwarded-Proto") == "https")
	http.SetCookie(c.W, &http.Cookie{Name: cookieName, Value: tok, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Expires: time.Now().Add(180 * 24 * time.Hour), Secure: secure})
	a.T.Users.Update(uid, func(u *model.User) { u.DateAut = now; u.DateLast = now; u.IP = c.IP() })
	a.T.UserLog.Insert(&model.UserLog{IDUser: uid, Method: true, Time: now, IP: ipToInt(c.IP()), Ua: trunc(c.R.UserAgent(), 32)})
}

// Logout удаляет текущую сессию.
func (a *App) Logout(c *Ctx) {
	if c.Sess != nil {
		a.T.Sessions.Delete(c.Sess.ID)
	}
	secure := c.R.TLS != nil || (a.Cfg.Bool("trust_proxy") && c.R.Header.Get("X-Forwarded-Proto") == "https")
	http.SetCookie(c.W, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secure})
}

// LogoutAll закрывает все сессии пользователя (кроме keep, если >0).
func (a *App) LogoutAll(uid, keep int64) {
	for _, s := range a.T.Sessions.By("user", i64(uid)) {
		if s.ID != keep {
			a.T.Sessions.Delete(s.ID)
		}
	}
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

// Trunc — обрезка строки по рунам.
func Trunc(s string, n int) string { return trunc(s, n) }

// banGate: забаненные во всём разделе "all" могут только читать профиль бана и выходить.
func (a *App) banGate(c *Ctx) error {
	p0 := c.R.URL.Path
	if n := ipToInt(c.IP()); n != 0 && p0 != "/api/me" && p0 != "/api/config" && p0 != "/api/ban/status" && strings.HasPrefix(p0, "/api/") {
		if a.T.BanIP.Count(func(b *model.BanIp) bool { return n >= b.Min && n <= b.Max }) > 0 {
			return Err(403, "Ваш IP-адрес заблокирован")
		}
	}
	if c.user == nil && a.Cfg.Str("guest_select") == "1" && strings.HasPrefix(p0, "/api/") {
		for _, ok := range []string{"/api/me", "/api/config", "/api/login", "/api/register", "/api/captcha", "/api/apps", "/api/pass/", "/api/ban/", "/api/dev/", "/api/menu", "/api/events", "/api/counters"} {
			if strings.HasPrefix(p0, ok) {
				return nil
			}
		}
		return Err(401, "Доступ к сайту разрешен только авторизованым пользователям")
	}
	if c.user == nil || c.R.Method == http.MethodGet {
		return nil
	}
	p := c.R.URL.Path
	if p == "/api/logout" || p == "/api/login" {
		return nil
	}
	if b, ok := a.ActiveBan(c.user.ID, "all"); ok {
		msg := "Вы заблокированы"
		if b.Prich != "" {
			msg += ": " + b.Prich
		}
		return Err(403, msg)
	}
	return nil
}

// ActiveBan возвращает действующий бан пользователя в разделе (razdel: all, forum, chat, guest, obmen, foto, notes ...).
func (a *App) ActiveBan(uid int64, razdel string) (model.Ban, bool) {
	now := Now()
	for _, b := range a.T.Bans.By("user", i64(uid)) {
		if b.Razdel == razdel && (b.Navsegda == 1 || b.Time > now) {
			return b, true
		}
	}
	return model.Ban{}, false
}

// IsBanned — есть ли любой действующий бан (значок бана).
func (a *App) IsBanned(uid int64) bool {
	now := Now()
	for _, b := range a.T.Bans.By("user", i64(uid)) {
		if b.Navsegda == 1 || b.Time > now {
			return true
		}
	}
	return false
}
