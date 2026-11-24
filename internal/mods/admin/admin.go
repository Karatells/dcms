// Package admin — панель управления DCMS Social: пользователи, баны, IP, группы и привилегии,
// жалобы, системные настройки, меню, статистика, журнал действий, резервные копии.
package admin

import (
	"sort"
	"strings"
	"time"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

// Register подключает маршруты модуля.
func Register(a *core.App) {
	registerUsers(a)
	registerBans(a)
	registerIP(a)
	registerGroups(a)
	registerSpam(a)
	registerSystem(a)
	registerMenu(a)
	registerStats(a)
}

func rlen(s string) int { return len([]rune(s)) }

// canTouch — можно ли модератору me воздействовать на пользователя u (уровень строго ниже, либо он сам Создатель).
func canTouch(me *model.User, u model.User) bool {
	return u.ID != me.ID && (u.Level < me.Level || me.Level >= 10)
}

// ---------- пользователи ----------

func registerUsers(a *core.App) {
	// ── Сводка для дашборда админ-панели
	a.GET("/api/admin/overview", func(c *core.Ctx) error {
		if err := c.NeedAccess("adm_panel_show"); err != nil {
			return err
		}
		now := core.Now()
		all := a.T.Users.All()
		var online, verified, totp int
		for _, u := range all {
			if u.DateLast > now-600 {
				online++
			}
			if u.Verified {
				verified++
			}
			if u.TOTPEnabled {
				totp++
			}
		}
		bans := a.T.Bans.Count(func(b *model.Ban) bool { return b.Navsegda == 1 || b.Time > now })
		return c.JSON(map[string]any{
			"users":    len(all),
			"online":   online,
			"verified": verified,
			"totp":     totp,
			"bans":     bans,
			"spam":     a.T.Spam.Len(),
		})
	}, core.Auth)

	// ── Список всех аккаунтов с живым поиском и фильтрами
	a.GET("/api/admin/users", func(c *core.Ctx) error {
		if err := c.NeedAccess("adm_panel_show"); err != nil {
			return err
		}
		me := c.User()
		q := strings.ToLower(strings.TrimSpace(c.Q("q")))
		filter := strings.TrimSpace(c.Q("filter"))
		p := c.Page(25)
		now := core.Now()
		showIP := c.Can("user_show_ip")
		canEdit := c.Can("user_prof_edit")
		canDel := c.Can("user_delete")

		all := a.T.Users.All()
		var filt []model.User
		for _, u := range all {
			switch filter {
			case "online":
				if u.DateLast <= now-600 {
					continue
				}
			case "verified":
				if !u.Verified {
					continue
				}
			case "2fa":
				if !u.TOTPEnabled {
					continue
				}
			case "banned":
				if !a.IsBanned(u.ID) {
					continue
				}
			case "staff":
				if u.Level < 1 {
					continue
				}
			}
			if q != "" {
				idStr := store.I(u.ID)
				match := strings.Contains(strings.ToLower(u.Nick), q) ||
					idStr == q ||
					strings.Contains(strings.ToLower(u.AnkName), q) ||
					strings.Contains(strings.ToLower(u.Email), q) ||
					(showIP && strings.Contains(strings.ToLower(u.IP), q))
				if !match {
					continue
				}
			}
			filt = append(filt, u)
		}
		sort.Slice(filt, func(i, j int) bool { return filt[i].ID > filt[j].ID })
		total := len(filt)
		end := p.Offset + p.Per
		if end > total {
			end = total
		}
		var sl []model.User
		if p.Offset < total {
			sl = filt[p.Offset:end]
		}
		items := make([]map[string]any, 0, len(sl))
		for _, u := range sl {
			items = append(items, map[string]any{
				"user":         a.Ref(u.ID),
				"email":        u.Email,
				"ip":           ifShow(showIP, u.IP),
				"date_reg":     u.DateReg,
				"date_last":    u.DateLast,
				"balls":        u.Balls,
				"money":        u.Money,
				"verified":     u.Verified,
				"totp_enabled": u.TOTPEnabled,
				"banned":       a.IsBanned(u.ID),
				"can_edit":     canEdit && (u.ID == me.ID || u.Level < me.Level || me.Level >= 10),
				"can_delete":   canDel && canTouch(me, u),
			})
		}
		return c.List(items, total, p)
	}, core.Auth)

	// ── Сброс 2FA пользователя администратором
	a.POST("/api/admin/user/{id}/reset-2fa", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if me.Level < 3 && !c.Can("user_prof_edit") {
			return core.ErrForbidden()
		}
		u, ok := a.T.Users.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Пользователь не найден")
		}
		if u.ID != me.ID && u.Level >= me.Level && me.Level < 10 {
			return core.ErrForbidden("Недостаточно прав")
		}
		a.T.Users.Update(u.ID, func(x *model.User) {
			x.TOTPEnabled = false
			x.TOTPSecret = ""
		})
		a.AdminLog(me.ID, "Пользователи", "Сброс 2FA", "Сброшена двухфакторная аутентификация для '"+u.Nick+"'")
		return c.OK()
	}, core.Auth)

	a.GET("/api/admin/user/{id}", func(c *core.Ctx) error {
		if err := c.NeedAccess("user_prof_edit"); err != nil {
			return err
		}
		u, ok := a.GetUser(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Пользователь не найден")
		}
		me := c.User()
		groups := []map[string]any{}
		for _, g := range sortedGroups(a) {
			groups = append(groups, map[string]any{"id": g.ID, "name": g.Name, "level": g.Level, "allowed": g.Level < me.Level})
		}
		return c.JSON(map[string]any{
			"id": u.ID, "nick": u.Nick, "name": u.AnkName, "city": u.AnkCity, "about": u.AnkOSebe, "skype": u.AnkSkype,
			"phone": u.AnkNTel, "email": u.Email, "day": u.AnkDR, "month": u.AnkMR, "year": u.AnkGR, "pol": u.Pol,
			"group": u.GroupAccess, "balls": u.Balls, "money": u.Money, "set_files": u.SetFiles, "set_time_chat": u.SetTimeChat, "set_p_str": u.SetPStr, "verified": u.Verified, "totp_enabled": u.TOTPEnabled,
			"groups": groups, "can_nick": c.Can("user_change_nick"), "can_group": c.Can("user_change_group"),
			"can_balls": me.Level >= 3 || me.ID == u.ID, "can_delete": c.Can("user_delete") && canTouch(me, u),
			"ip": ifShow(c.Can("user_show_ip"), u.IP), "ua": ifShow(c.Can("user_show_ua"), u.UA),
			"date_reg": u.DateReg, "date_last": u.DateLast,
		})
	}, core.Auth)

	a.PUT("/api/admin/user/{id}", func(c *core.Ctx) error {
		if err := c.NeedAccess("user_prof_edit"); err != nil {
			return err
		}
		me := c.User()
		u, ok := a.T.Users.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Пользователь не найден")
		}
		if u.ID != me.ID && u.Level >= me.Level && me.Level < 10 {
			return core.ErrForbidden("Нельзя редактировать пользователя вашего или более высокого уровня")
		}
		var in struct {
			Nick     *string `json:"nick"`
			Name     *string `json:"name"`
			City     *string `json:"city"`
			About    *string `json:"about"`
			Skype    *string `json:"skype"`
			Phone    *string `json:"phone"`
			Email    *string `json:"email"`
			Day      *int64  `json:"day"`
			Month    *int64  `json:"month"`
			Year     *int64  `json:"year"`
			Pol      *int64  `json:"pol"`
			Pass     string  `json:"new_pass"`
			Group    *int64  `json:"group"`
			Balls    *int64  `json:"balls"`
			SetFiles *bool   `json:"set_files"`
			Verified *bool   `json:"verified"`
			TimeChat *int64  `json:"set_time_chat"`
			PStr     *int64  `json:"set_p_str"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if in.Nick != nil && *in.Nick != u.Nick {
			if !c.Can("user_change_nick") {
				return core.ErrForbidden("Нет права менять логин")
			}
			n := strings.TrimSpace(*in.Nick)
			if rlen(n) < 3 || rlen(n) > 32 {
				return core.ErrBad("Логин: 3–32 символа")
			}
			if ex, ok := a.UserByNick(n); ok && ex.ID != u.ID {
				return core.ErrBad("Логин " + n + " уже занят")
			}
			a.AdminLog(me.ID, "Пользователи", "Смена логина", "Пользователь '"+u.Nick+"' → '"+n+"'")
			*in.Nick = n
		}
		if in.Email != nil && *in.Email != "" && !strings.Contains(*in.Email, "@") {
			return core.ErrBad("Неверный email")
		}
		if in.Day != nil && (*in.Day < 0 || *in.Day > 31) || in.Month != nil && (*in.Month < 0 || *in.Month > 12) ||
			in.Year != nil && (*in.Year != 0 && (*in.Year < 1900 || *in.Year > int64(time.Now().Year()))) {
			return core.ErrBad("Неверная дата рождения")
		}
		if in.TimeChat != nil && (*in.TimeChat < 0 || *in.TimeChat > 900) || in.PStr != nil && (*in.PStr < 0 || *in.PStr > 100) {
			return core.ErrBad("Неверные настройки отображения")
		}
		if in.Pass != "" && rlen(in.Pass) < 6 {
			return core.ErrBad("Пароль должен быть не короче 6 символов")
		}
		var newGroup *model.UserGroup
		if in.Group != nil && *in.Group != u.GroupAccess {
			if !c.Can("user_change_group") {
				return core.ErrForbidden("Нет права менять группу")
			}
			g, ok := a.T.Groups.Get(*in.Group)
			if !ok || g.Level >= me.Level {
				return core.ErrBad("Эту группу назначить нельзя")
			}
			newGroup = &g
		}
		if in.Balls != nil && !(me.Level >= 3 || me.ID == u.ID) {
			in.Balls = nil
		}
		old := a.Ref(u.ID)
		a.T.Users.Update(u.ID, func(x *model.User) {
			if in.Nick != nil {
				x.Nick = *in.Nick
				x.NickLower = strings.ToLower(*in.Nick)
			}
			if in.Name != nil {
				x.AnkName = a.Text.Clean(*in.Name)
			}
			if in.City != nil {
				x.AnkCity = a.Text.Clean(*in.City)
			}
			if in.About != nil {
				x.AnkOSebe = a.Text.Clean(*in.About)
			}
			if in.Skype != nil {
				x.AnkSkype = a.Text.Clean(*in.Skype)
			}
			if in.Phone != nil {
				x.AnkNTel = a.Text.Clean(*in.Phone)
			}
			if in.Email != nil {
				x.Email = strings.TrimSpace(*in.Email)
			}
			if in.Day != nil {
				x.AnkDR = *in.Day
			}
			if in.Month != nil {
				x.AnkMR = *in.Month
			}
			if in.Year != nil {
				x.AnkGR = *in.Year
			}
			if in.Pol != nil && (*in.Pol == 0 || *in.Pol == 1) {
				x.Pol = *in.Pol
			}
			if in.Pass != "" {
				x.PassHash = core.HashPassword(in.Pass)
			}
			if newGroup != nil {
				x.GroupAccess = newGroup.ID
				x.Level = newGroup.Level
			}
			if in.Balls != nil {
				x.Balls = *in.Balls
			}
			if in.SetFiles != nil {
				x.SetFiles = *in.SetFiles
			}
			if in.Verified != nil {
				x.Verified = *in.Verified
			}
			if in.TimeChat != nil {
				x.SetTimeChat = *in.TimeChat
			}
			if in.PStr != nil {
				x.SetPStr = *in.PStr
			}
		})
		if in.Pass != "" {
			a.LogoutAll(u.ID, 0)
			a.AdminLog(me.ID, "Пользователи", "Смена пароля", "Пользователь '"+u.Nick+"'")
		}
		if newGroup != nil {
			a.AdminLog(me.ID, "Пользователи", "Изменение статуса", "Пользователь '"+u.Nick+"': статус '"+old.Group+"' изменён на '"+newGroup.Name+"'")
		}
		a.ResetUserCache(u.ID)
		return c.OK()
	}, core.Auth)

	a.DELETE("/api/admin/user/{id}", func(c *core.Ctx) error {
		if err := c.NeedAccess("user_delete"); err != nil {
			return err
		}
		me := c.User()
		u, ok := a.T.Users.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Пользователь не найден")
		}
		if !canTouch(me, u) {
			return core.ErrForbidden("Нельзя удалить этого пользователя")
		}
		a.AdminLog(me.ID, "Пользователи", "Удаление", "Удалён пользователь '"+u.Nick+"'")
		a.DeleteUser(u.ID)
		return c.OK()
	}, core.Auth)

	// массовое удаление неактивных
	count := func(c *core.Ctx) (int64, []model.User) {
		n := c.QInt("n")
		step := int64(86400)
		if c.Q("unit") == "mes" {
			step *= 30
		}
		lim := core.Now() - n*step
		me := c.User()
		return lim, a.T.Users.Find(func(u *model.User) bool { return u.DateLast < lim && u.ID != me.ID && u.Level < me.Level })
	}
	a.GET("/api/admin/mass-delete", func(c *core.Ctx) error {
		if err := c.NeedAccess("user_mass_delete"); err != nil {
			return err
		}
		if c.QInt("n") < 1 {
			return core.ErrBad("Укажите период")
		}
		_, us := count(c)
		names := []string{}
		for i, u := range us {
			if i >= 30 {
				break
			}
			names = append(names, u.Nick)
		}
		return c.OK("count", len(us), "sample", names)
	}, core.Auth)
	a.POST("/api/admin/mass-delete", func(c *core.Ctx) error {
		if err := c.NeedAccess("user_mass_delete"); err != nil {
			return err
		}
		if c.QInt("n") < 1 {
			return core.ErrBad("Укажите период")
		}
		_, us := count(c)
		for _, u := range us {
			a.DeleteUser(u.ID)
		}
		a.AdminLog(c.User().ID, "Пользователи", "Массовое удаление", "Удалено неактивных пользователей: "+store.I(int64(len(us))))
		return c.OK("deleted", len(us))
	}, core.Auth)

	// совпадения (мультиаккаунты) по IP из журнала входов
	a.GET("/api/admin/collisions/{id}", func(c *core.Ctx) error {
		if err := c.NeedAccess("user_collisions"); err != nil {
			return err
		}
		uid := c.PathInt("id")
		ips := map[int64]bool{}
		for _, l := range a.T.UserLog.By("user", store.I(uid)) {
			ips[l.IP] = true
		}
		found := map[int64]int{}
		a.T.UserLog.Scan(func(l *model.UserLog) bool {
			if l.IDUser != uid && ips[l.IP] {
				found[l.IDUser]++
			}
			return true
		})
		out := []map[string]any{}
		for id, n := range found {
			out = append(out, map[string]any{"user": a.Ref(id), "count": n})
		}
		sort.Slice(out, func(i, j int) bool { return out[i]["count"].(int) > out[j]["count"].(int) })
		return c.OK("items", out)
	}, core.Auth)
}

func ifShow(ok bool, s string) string {
	if ok {
		return s
	}
	return ""
}

func sortedGroups(a *core.App) []model.UserGroup {
	gs := a.T.Groups.All()
	sort.Slice(gs, func(i, j int) bool {
		if gs[i].Level != gs[j].Level {
			return gs[i].Level < gs[j].Level
		}
		return gs[i].ID < gs[j].ID
	})
	return gs
}
