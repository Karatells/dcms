// Package account — вход, регистрация, капча, восстановление пароля, настройки пользователя, баны (страницы).
package account

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

func randTok() string { b := make([]byte, 16); rand.Read(b); return hex.EncodeToString(b) }

// RegMail — запись об активированном e-mail (reg_mail).
type RegMail struct {
	store.Base
	IDUser int64  `json:"id_user"`
	Mail   string `json:"mail"`
}

// PassReset — токен восстановления пароля (аналог user.sess в PHP).
type PassReset struct {
	store.Base
	UserID int64  `json:"user_id"`
	Token  string `json:"token"`
	Time   int64  `json:"time"`
}

var (
	reNick  = regexp.MustCompile(`^[A-Za-zА-Яа-яЁё0-9\-_ ]+$`)
	reLat   = regexp.MustCompile(`[A-Za-z]`)
	reCyr   = regexp.MustCompile(`[А-Яа-яЁё]`)
	reName  = regexp.MustCompile(`^[A-Za-zА-Яа-яЁё \-]*$`)
	reEmail = regexp.MustCompile(`^[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}$`)
)

func init() {
	core.Defaults["reg_select"] = "open"
}

var resets *store.Table[PassReset, *PassReset]

func Register(a *core.App) {
	a.InitMail()
	regMails := store.NewTable[RegMail](a.DB, "reg_mail")
	resets = store.NewTable[PassReset](a.DB, "pass_reset").Index("token", func(r *PassReset) string { return r.Token })
	_ = regMails

	// ---------- капча
	core.CaptchaCheck = func(c *core.Ctx, code string) bool {
		ck, _ := c.R.Cookie("dcms_cap")
		tok := ""
		if ck != nil {
			tok = ck.Value
		}
		return checkCaptcha(tok, strings.TrimSpace(code))
	}
	a.GET("/api/captcha", func(c *core.Ctx) error {
		tok, code := newCaptcha()
		http.SetCookie(c.W, &http.Cookie{Name: "dcms_cap", Value: tok, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
		return c.ServeBytes(renderCaptcha(code), "image/png", false)
	})

	// ---------- вход
	a.POST("/api/login", func(c *core.Ctx) error {
		var in struct {
			Nick string `json:"nick"`
			Pass string `json:"pass"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if ipBanned(a, c.IP()) {
			return core.ErrForbidden("Ваш IP-адрес заблокирован")
		}
		u, ok := a.UserByNick(in.Nick)
		if !ok || !core.CheckPassword(in.Pass, u.PassHash) {
			time.Sleep(400 * time.Millisecond) // замедляем подбор
			return core.ErrBad("Неправильный логин или пароль")
		}
		if u.Activation != "" {
			return core.ErrBad("Аккаунт не активирован. Проверьте почту и перейдите по ссылке из письма")
		}
		a.Login(c, u.ID)
		collisions(a, c, u)
		return c.OK("id", u.ID)
	})
	a.POST("/api/logout", func(c *core.Ctx) error {
		a.Logout(c)
		return c.OK()
	})

	// ---------- регистрация
	a.POST("/api/register", func(c *core.Ctx) error {
		if c.User() != nil {
			return core.ErrBad("Вы уже авторизованы")
		}
		mode := a.Cfg.Str("reg_select")
		if mode == "close" {
			return core.ErrBad("Регистрация временно приостановлена")
		}
		if ipBanned(a, c.IP()) {
			return core.ErrForbidden("Ваш IP-адрес заблокирован")
		}
		var in struct {
			Nick, Pass1, Pass2, Captcha, Email, Name string
			Pol                                      int64
			D, M, Y                                  int64
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		var errs []string
		nick := in.Nick
		if !reNick.MatchString(nick) {
			errs = append(errs, "В логине присутствуют запрещенные символы")
		}
		if reLat.MatchString(nick) && reCyr.MatchString(nick) {
			errs = append(errs, "Разрешается использовать символы только русского или только английского алфавита")
		}
		if strings.HasPrefix(nick, " ") || strings.HasSuffix(nick, " ") {
			errs = append(errs, "Запрещено использовать пробел в начале и конце ника")
		}
		if n := utf8.RuneCountInString(nick); n < 3 {
			errs = append(errs, "Короткий логин")
		} else if n > 32 {
			errs = append(errs, "Длина логина превышает 32 символа")
		}
		if _, ok := a.UserByNick(nick); ok {
			errs = append(errs, fmt.Sprintf("Логин \"%s\" уже зарегистрирован", nick))
		}
		if mode == "open_mail" {
			if strings.TrimSpace(in.Email) == "" {
				errs = append(errs, "Неоходимо ввести Email")
			} else if !reEmail.MatchString(in.Email) {
				errs = append(errs, "Неверный формат Email")
			} else if _, ok := a.T.Users.OneBy("email", strings.ToLower(in.Email)); ok {
				errs = append(errs, "Пользователь с этим E-mail уже зарегистрирован")
			}
		}
		if n := utf8.RuneCountInString(in.Pass1); n < 6 {
			errs = append(errs, "По соображениям безопасности пароль не может быть короче 6-ти символов")
		} else if n > 32 {
			errs = append(errs, "Длина пароля превышает 32 символа")
		}
		if in.Pass1 != in.Pass2 {
			errs = append(errs, "Пароли не совпадают")
		}
		ck, _ := c.R.Cookie("dcms_cap")
		tok := ""
		if ck != nil {
			tok = ck.Value
		}
		if !checkCaptcha(tok, strings.TrimSpace(in.Captcha)) {
			errs = append(errs, "Неверное проверочное число")
		}
		if !reName.MatchString(in.Name) {
			errs = append(errs, "Неверный формат имени")
		}
		if in.D < 0 || in.D > 31 {
			errs = append(errs, "Неверный формат дня рождения")
		}
		if in.M < 0 || in.M > 12 {
			errs = append(errs, "Неверный формат месяца рождения")
		}
		if in.Y < 0 || in.Y > int64(time.Now().Year()) {
			errs = append(errs, "Неверный формат года рождения")
		}
		if len(errs) > 0 {
			return core.ErrBad(strings.Join(errs, "\n"))
		}
		pol := int64(0)
		if in.Pol == 1 {
			pol = 1
		}
		u, err := a.CreateUser(nick, in.Pass1, pol, in.Email, mode != "open_mail")
		if err != nil {
			return err
		}
		a.T.Users.Update(u.ID, func(x *model.User) {
			x.AnkName, x.AnkDR, x.AnkMR, x.AnkGR = in.Name, in.D, in.M, in.Y
			x.IP, x.UA, x.Browser = c.IP(), core.Trunc(c.R.UserAgent(), 200), "web"
		})
		if mode == "open_mail" {
			link := fmt.Sprintf("%s/reg/activate/%d/%s", baseURL(c), u.ID, u.Activation)
			a.SendMail(in.Email, "Активация аккаунта", fmt.Sprintf("Здравствуйте %s<br />Для активации Вашего аккаунта перейдите по ссылке:<br /><a href=\"%s\">%s</a><br />Если аккаунт не будет активирован в течении 24 часов, он будет удален<br />С уважением, администрация сайта<br />", nick, link, link))
			return c.OK("activate", true, "message", "Вам необходимо активировать Ваш аккаунт по ссылке, высланной на Email")
		}
		a.Login(c, u.ID)
		return c.OK("id", u.ID)
	})
	a.POST("/api/register/check-nick", func(c *core.Ctx) error {
		var in struct{ Nick string }
		if err := c.Bind(&in); err != nil {
			return err
		}
		_, taken := a.UserByNick(in.Nick)
		return c.JSON(map[string]any{"free": !taken && reNick.MatchString(in.Nick) && utf8.RuneCountInString(in.Nick) >= 3})
	})
	a.POST("/api/register/activate", func(c *core.Ctx) error {
		var in struct {
			ID   int64  `json:"id"`
			Code string `json:"code"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		u, ok := a.T.Users.Get(in.ID)
		if !ok || u.Activation == "" || u.Activation != in.Code {
			return core.ErrBad("Ссылка активации недействительна")
		}
		a.T.Users.Update(u.ID, func(x *model.User) { x.Activation = "" })
		regMails.Insert(&RegMail{IDUser: u.ID, Mail: u.Email})
		a.Login(c, u.ID)
		return c.OK("message", "Ваш аккаунт успешно активирован")
	})

	// ---------- восстановление пароля
	a.POST("/api/pass/request", func(c *core.Ctx) error {
		var in struct{ Nick, Mail string }
		if err := c.Bind(&in); err != nil {
			return err
		}
		if in.Nick == "" || in.Mail == "" {
			return core.ErrBad("Введите логин и e-mail")
		}
		u, ok := a.UserByNick(in.Nick)
		if !ok {
			return core.ErrBad("Пользователь с таким логином не зарегистрирован")
		}
		if u.Email == "" || !strings.EqualFold(u.Email, strings.TrimSpace(in.Mail)) {
			return core.ErrBad("Неверный адрес E-mail или информация о E-mail отсутствует")
		}
		resets.DeleteWhere(func(r *PassReset) bool { return r.UserID == u.ID })
		r := PassReset{UserID: u.ID, Token: core.RandToken(10), Time: core.Now()}
		resets.Insert(&r)
		link := fmt.Sprintf("%s/pass/%d/%s", baseURL(c), u.ID, r.Token)
		a.SendMail(u.Email, "Восстановление пароля", fmt.Sprintf("Здравствуйте %s<br />Вы активировали восстановление пароля<br />Для установки нового пароля перейдите по ссылке:<br /><a href=\"%s\">%s</a><br />Данная ссылка действительна до первой авторизации под своим логином (%s)<br />С уважением, администрация сайта<br />", u.Nick, link, link, u.Nick))
		return c.OK("message", fmt.Sprintf("Ссылка для установки нового пароля отправлена на e-mail \"%s\"", u.Email))
	})
	a.GET("/api/pass/check", func(c *core.Ctx) error {
		r, ok := resets.OneBy("token", c.Q("token"))
		if !ok || r.UserID != c.QInt("id") || r.Time < core.Now()-86400*3 {
			return core.ErrNotFound("Ссылка недействительна")
		}
		u, _ := a.T.Users.Get(r.UserID)
		return c.JSON(map[string]any{"nick": u.Nick})
	})
	a.POST("/api/pass/reset", func(c *core.Ctx) error {
		var in struct {
			ID                  int64
			Token, Pass1, Pass2 string
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		r, ok := resets.OneBy("token", in.Token)
		if !ok || r.UserID != in.ID {
			return core.ErrBad("Ссылка недействительна")
		}
		if err := checkNewPass(in.Pass1, in.Pass2); err != nil {
			return err
		}
		a.T.Users.Update(r.UserID, func(x *model.User) { x.PassHash = core.HashPassword(in.Pass1) })
		resets.Delete(r.ID)
		a.LogoutAll(r.UserID, 0)
		return c.OK("message", "Пароль успешно изменен")
	})

	// ---------- блокировки: информация для пользователя (ban.php / ban_ip.php)
	a.GET("/api/ban/status", func(c *core.Ctx) error {
		out := map[string]any{"ip_banned": ipBanned(a, c.IP())}
		if u := c.User(); u != nil {
			now := core.Now()
			type bi struct {
				Razdel  string        `json:"razdel"`
				Reason  string        `json:"reason"`
				Until   int64         `json:"until"`
				Forever bool          `json:"forever"`
				By      model.UserRef `json:"by"`
				Time    int64         `json:"time"`
			}
			var list []bi
			for _, b := range a.T.Bans.By("user", store.I(u.ID)) {
				if b.Navsegda == 1 || b.Time > now {
					list = append(list, bi{b.Razdel, b.Prich, b.Time, b.Navsegda == 1, a.Ref(b.IDBan), b.Time})
				}
			}
			out["bans"] = list
		}
		return c.JSON(out)
	})

	registerSettings(a)
}

func baseURL(c *core.Ctx) string {
	sch := "http"
	if c.R.TLS != nil || c.R.Header.Get("X-Forwarded-Proto") == "https" {
		sch = "https"
	}
	return sch + "://" + c.R.Host
}

func checkNewPass(p1, p2 string) error {
	if p1 != p2 {
		return core.ErrBad("Новый пароль не совпадает с подтверждением")
	}
	if n := utf8.RuneCountInString(p1); n < 6 {
		return core.ErrBad("По соображениям безопасности новый пароль не может быть короче 6-ти символов")
	} else if n > 32 {
		return core.ErrBad("Длина пароля превышает 32 символа")
	}
	return nil
}

// ipBanned — попадает ли IP в диапазоны ban_ip.
func ipBanned(a *core.App, ip string) bool {
	n := core.IPToInt(ip)
	if n == 0 {
		return false
	}
	return a.T.BanIP.Count(func(b *model.BanIp) bool { return n >= b.Min && n <= b.Max }) > 0
}

// collisions фиксирует совпадение ip+ua у недавно активных пользователей (user_collision, как в sys/inc/user.php).
func collisions(a *core.App, c *core.Ctx, u model.User) {
	ua := core.Trunc(c.R.UserAgent(), 200)
	now := core.Now()
	for _, o := range a.T.Users.Find(func(x *model.User) bool {
		return x.ID != u.ID && x.IP == c.IP() && x.UA == ua && x.DateLast > now-600
	}) {
		exists := a.T.Collisions.Count(func(x *model.UserCollision) bool {
			return x.IDUser == u.ID && x.IDUser2 == o.ID || x.IDUser == o.ID && x.IDUser2 == u.ID
		}) > 0
		if !exists {
			a.T.Collisions.Insert(&model.UserCollision{IDUser: u.ID, IDUser2: o.ID, Type: "ip_ua_time"})
		}
	}
}

func fmtInt(n int64) string { return fmt.Sprintf("%d", n) }
