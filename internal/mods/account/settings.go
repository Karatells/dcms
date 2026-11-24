package account

import (
	"strings"

	"dcms/internal/core"
	"dcms/internal/model"
)

func registerSettings(a *core.App) {
	a.GET("/api/settings", func(c *core.Ctx) error {
		u := c.User()
		us := a.UserSettings(u.ID)
		return c.JSON(map[string]any{
			"add_konts": u.AddKonts, "set_time_chat": u.SetTimeChat, "set_news_to_mail": u.SetNewsToMail, "set_timesdvig": u.SetTimesdvig,
			"show_url": u.ShowURL, "abuld": u.Abuld, "set_files": u.SetFiles, "set_p_str": u.SetPStr, "theme": u.Theme, "accent": u.Accent,
			"privat_str": us.PrivatStr, "privat_mail": us.PrivatMail, "ocenka": us.Ocenka, "block_group_invites": us.BlockGroupInvites, "email": u.Email, "has_email": u.Email != "",
		})
	}, core.Auth)

	a.PUT("/api/settings/general", func(c *core.Ctx) error {
		u, _ := c.Need()
		var in struct {
			AddKonts   int64 `json:"add_konts"`
			TimeChat   int64 `json:"set_time_chat"`
			NewsToMail bool  `json:"set_news_to_mail"`
			Timesdvig  int64 `json:"set_timesdvig"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if in.AddKonts < 0 || in.AddKonts > 2 {
			return core.ErrBad("Ошибка режима добавления контактов")
		}
		if in.TimeChat < 0 || in.TimeChat > 900 {
			return core.ErrBad("Ошибка во времени автообновления")
		}
		if in.Timesdvig < -12 || in.Timesdvig > 12 {
			return core.ErrBad("Неправильное количество пунктов на страницу")
		}
		a.T.Users.Update(u.ID, func(x *model.User) {
			x.AddKonts, x.SetTimeChat, x.SetNewsToMail, x.SetTimesdvig = in.AddKonts, in.TimeChat, in.NewsToMail, in.Timesdvig
		})
		return c.OK("message", "Изменения сохранены")
	}, core.Auth)

	a.PUT("/api/settings/view", func(c *core.Ctx) error {
		u, _ := c.Need()
		var in struct {
			ShowURL bool   `json:"show_url"`
			Abuld   bool   `json:"abuld"`
			Files   bool   `json:"set_files"`
			PStr    int64  `json:"set_p_str"`
			Theme   string `json:"theme"`
			Accent  string `json:"accent"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if in.PStr < 1 || in.PStr > 100 {
			return core.ErrBad("Неправильное количество пунктов на страницу")
		}
		if err := checkTheme(in.Theme, in.Accent); err != nil {
			return err
		}
		a.T.Users.Update(u.ID, func(x *model.User) {
			x.ShowURL, x.Abuld, x.SetFiles, x.SetPStr, x.Theme, x.Accent = in.ShowURL, in.Abuld, in.Files, in.PStr, in.Theme, in.Accent
		})
		a.DropUserCache(u.ID)
		return c.OK("message", "Настройки обновлены")
	}, core.Auth)

	a.POST("/api/settings/theme", func(c *core.Ctx) error {
		u, _ := c.Need()
		var in struct{ Theme, Accent string }
		if err := c.Bind(&in); err != nil {
			return err
		}
		if err := checkTheme(in.Theme, in.Accent); err != nil {
			return err
		}
		a.T.Users.Update(u.ID, func(x *model.User) { x.Theme = in.Theme; x.Accent = in.Accent })
		a.DropUserCache(u.ID)
		return c.OK()
	}, core.Auth)

	a.PUT("/api/settings/privacy", func(c *core.Ctx) error {
		u, _ := c.Need()
		var in struct {
			PrivatStr         int64  `json:"privat_str"`
			PrivatMail        int64  `json:"privat_mail"`
			Ocenka            *int64 `json:"ocenka"`
			BlockGroupInvites *bool  `json:"block_group_invites"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if in.PrivatStr < 0 || in.PrivatStr > 2 || in.PrivatMail < 0 || in.PrivatMail > 2 {
			return core.ErrBad("Недопустимое значение")
		}
		us := a.UserSettings(u.ID)
		a.T.UserSet.Update(us.ID, func(x *model.UserSet) {
			x.PrivatStr, x.PrivatMail = in.PrivatStr, in.PrivatMail
			if in.Ocenka != nil && *in.Ocenka >= 0 && *in.Ocenka <= 2 {
				x.Ocenka = *in.Ocenka
			}
			if in.BlockGroupInvites != nil {
				x.BlockGroupInvites = *in.BlockGroupInvites
			}
		})
		return c.OK("message", "Изменения успешно приняты")
	}, core.Auth)

	a.POST("/api/settings/password", func(c *core.Ctx) error {
		u, _ := c.Need()
		var in struct{ Pass, Pass1, Pass2 string }
		if err := c.Bind(&in); err != nil {
			return err
		}
		if !core.CheckPassword(in.Pass, u.PassHash) {
			return core.ErrBad("Старый пароль неверен")
		}
		if in.Pass1 == "" {
			return core.ErrBad("Введите новый пароль")
		}
		if err := checkNewPass(in.Pass1, in.Pass2); err != nil {
			return err
		}
		a.T.Users.Update(u.ID, func(x *model.User) { x.PassHash = core.HashPassword(in.Pass1) })
		var keep int64
		if c.Sess != nil {
			keep = c.Sess.ID
		}
		a.LogoutAll(u.ID, keep)
		return c.OK("message", "Пароль успешно изменен")
	}, core.Auth)

	// сессии и журнал входов
	a.GET("/api/settings/sessions", func(c *core.Ctx) error {
		u, _ := c.Need()
		type s struct {
			ID      int64  `json:"id"`
			IP      string `json:"ip"`
			UA      string `json:"ua"`
			Created int64  `json:"created"`
			Last    int64  `json:"last"`
			Current bool   `json:"current"`
		}
		var out []s
		for _, x := range a.T.Sessions.By("user", itoa(u.ID)) {
			out = append(out, s{x.ID, x.IP, core.Trunc(x.UA, 120), x.Created, x.Last, c.Sess != nil && c.Sess.ID == x.ID})
		}
		log := a.T.UserLog.By("user", itoa(u.ID))
		if len(log) > 20 {
			log = log[len(log)-20:]
		}
		return c.JSON(map[string]any{"sessions": out, "log": log})
	}, core.Auth)
	a.POST("/api/settings/sessions/close-others", func(c *core.Ctx) error {
		u, _ := c.Need()
		var keep int64
		if c.Sess != nil {
			keep = c.Sess.ID
		}
		a.LogoutAll(u.ID, keep)
		return c.OK()
	}, core.Auth)

	registerTOTP(a)
}

func checkTheme(theme, accent string) error {
	switch theme {
	case "", "auto", "light", "dark":
	default:
		return core.ErrBad("Недопустимая тема")
	}
	if accent != "" && !(len(accent) == 7 && accent[0] == '#' && strings.Trim(accent[1:], "0123456789abcdefABCDEF") == "") {
		return core.ErrBad("Недопустимый цвет")
	}
	return nil
}

func itoa(n int64) string { return fmtInt(n) }
