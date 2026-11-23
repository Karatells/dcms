package admin

import (
	"sort"
	"strings"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

var banRazdels = [][2]string{{"all", "Везде"}, {"guest", "Гостевая"}, {"notes", "Дневники"}, {"forum", "Форум"}, {"files", "Файлы"}, {"chat", "Чат"}, {"lib", "Библиотека"}, {"foto", "Фото"}}
var banReasons = [][2]any{{1, "Спам/Реклама"}, {2, "Мошенничество"}, {3, "Нецензурная брань"}, {4, "Клонирование ников"}, {5, "Подстрекательство"}, {6, "Флуд"}, {7, "Флейм"}, {0, "Другое"}}

func reasonName(n int64) string {
	for _, r := range banReasons {
		if int64(r[0].(int)) == n {
			return r[1].(string)
		}
	}
	return "Другое"
}

func banView(a *core.App, b model.Ban) map[string]any {
	rz := b.Razdel
	for _, r := range banRazdels {
		if r[0] == b.Razdel {
			rz = r[1]
		}
	}
	return map[string]any{"id": b.ID, "user": a.Ref(b.IDUser), "by": a.Ref(b.IDBan), "until": b.Time, "forever": b.Navsegda == 1,
		"razdel": b.Razdel, "razdel_name": rz, "reason": b.Prich, "why": reasonName(b.Pochemu), "why_id": b.Pochemu, "start": b.Post}
}

func active(b model.Ban, now int64) bool { return b.Navsegda == 1 || b.Time > now }

func registerBans(a *core.App) {
	// форма и текущие баны пользователя
	a.GET("/api/admin/ban/{uid}", func(c *core.Ctx) error {
		if _, err := c.Need(); err != nil {
			return err
		}
		if !c.Can("user_ban_set") && !c.Can("user_ban_set_h") && !c.Can("user_ban_unset") {
			return core.ErrForbidden()
		}
		u, ok := a.GetUser(c.PathInt("uid"))
		if !ok {
			return core.ErrNotFound("Пользователь не найден")
		}
		now := core.Now()
		list := []map[string]any{}
		for _, b := range a.T.Bans.By("user", store.I(u.ID)) {
			if active(b, now) {
				list = append(list, banView(a, b))
			}
		}
		rz := []map[string]string{}
		for _, r := range banRazdels {
			rz = append(rz, map[string]string{"id": r[0], "name": r[1]})
		}
		rs := []map[string]any{}
		for _, r := range banReasons {
			rs = append(rs, map[string]any{"id": r[0], "name": r[1]})
		}
		return c.JSON(map[string]any{"user": a.Ref(u.ID), "bans": list, "razdels": rz, "reasons": rs,
			"can_set": (c.Can("user_ban_set") || c.Can("user_ban_set_h")) && canTouch(c.User(), u),
			"full":    c.Can("user_ban_set"), "can_unset": c.Can("user_ban_unset")})
	}, core.Auth)

	a.POST("/api/admin/ban/{uid}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		full := c.Can("user_ban_set")
		if !full && !c.Can("user_ban_set_h") {
			return core.ErrForbidden()
		}
		u, ok := a.GetUser(c.PathInt("uid"))
		if !ok {
			return core.ErrNotFound("Пользователь не найден")
		}
		if !canTouch(me, u) {
			return core.ErrForbidden("Нельзя банить пользователя вашего или более высокого уровня")
		}
		var in struct {
			Razdel  string `json:"razdel"`
			Time    int64  `json:"time"`
			Unit    string `json:"unit"`
			Reason  string `json:"reason"`
			Why     int64  `json:"why"`
			Forever bool   `json:"forever"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		okR := false
		for _, r := range banRazdels {
			if r[0] == in.Razdel {
				okR = true
			}
		}
		if !okR {
			return core.ErrBad("Неверный раздел")
		}
		in.Reason = strings.TrimSpace(in.Reason)
		if rlen(in.Reason) < 10 || rlen(in.Reason) > 1024 {
			return core.ErrBad("Причина бана: 10–1024 символа")
		}
		if in.Why < 0 || in.Why > 7 {
			in.Why = 0
		}
		var sec int64
		switch in.Unit {
		case "min":
			sec = 60
		case "chas":
			sec = 3600
		case "sut":
			sec = 86400
		case "mes":
			sec = 86400 * 30
		default:
			return core.ErrBad("Неверная единица времени")
		}
		if in.Forever && !full {
			return core.ErrForbidden("Бессрочный бан недоступен")
		}
		total := in.Time * sec
		if !in.Forever {
			if in.Time < 1 || in.Time > 10000 {
				return core.ErrBad("Неверное время бана")
			}
			if !full && total > 86400 {
				return core.ErrForbidden("Вы можете банить максимум на 1 сутки")
			}
		}
		now := core.Now()
		// продление уже действующего бана в том же разделе
		if old, ok := a.ActiveBan(u.ID, in.Razdel); ok {
			a.T.Bans.Delete(old.ID)
		}
		b := model.Ban{Time: now + total, IDUser: u.ID, IDBan: me.ID, Prich: a.Text.Clean(in.Reason), View: false, Razdel: in.Razdel, Post: now, Pochemu: in.Why}
		if in.Forever {
			b.Navsegda = 1
			b.Time = 0
		}
		a.T.Bans.Insert(&b)
		a.AdminLog(me.ID, "Пользователи", "Бан", "Пользователь '"+u.Nick+"', раздел '"+in.Razdel+"': "+reasonName(in.Why))
		return c.OK()
	}, core.Auth)

	a.DELETE("/api/admin/ban-item/{id}", func(c *core.Ctx) error {
		if err := c.NeedAccess("user_ban_unset"); err != nil {
			return err
		}
		me := c.User()
		b, ok := a.T.Bans.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		if by, ok := a.GetUser(b.IDBan); ok && !(me.Level > by.Level || by.ID == me.ID || me.Level == 4) && me.Level < 10 {
			return core.ErrForbidden("Снять этот бан может только вынесший его или модератор выше уровнем")
		}
		a.T.Bans.Delete(b.ID)
		if u, ok := a.GetUser(b.IDUser); ok {
			a.AdminLog(me.ID, "Пользователи", "Снятие бана", "Пользователь '"+u.Nick+"', раздел '"+b.Razdel+"'")
		}
		return c.OK()
	}, core.Auth)

	// список всех активных банов
	a.GET("/api/admin/banlist", func(c *core.Ctx) error {
		if err := c.NeedAccess("adm_banlist"); err != nil {
			return err
		}
		p := c.Page()
		now := core.Now()
		rz := c.Q("razdel")
		list, total := a.T.Bans.Select(store.Opts[model.Ban]{Desc: true, Offset: p.Offset, Limit: p.Per,
			Where: func(b *model.Ban) bool { return active(*b, now) && (rz == "" || b.Razdel == rz) }})
		out := make([]map[string]any, 0, len(list))
		for _, b := range list {
			out = append(out, banView(a, b))
		}
		return c.List(out, total, p)
	}, core.Auth)
}

func ipStr(n int64) string {
	return strings.Join([]string{store.I(n >> 24 & 255), store.I(n >> 16 & 255), store.I(n >> 8 & 255), store.I(n & 255)}, ".")
}

func registerIP(a *core.App) {
	parse := func(c *core.Ctx, minS, maxS string) (int64, int64, error) {
		mn, mx := core.IPToInt(strings.TrimSpace(minS)), core.IPToInt(strings.TrimSpace(maxS))
		if mn == 0 {
			return 0, 0, core.ErrBad("Неверный формат начального IP")
		}
		if mx == 0 {
			return 0, 0, core.ErrBad("Неверный формат конечного IP")
		}
		if mn > mx {
			mn, mx = mx, mn
		}
		return mn, mx, nil
	}
	// бан по диапазону IP
	a.GET("/api/admin/ban-ip", func(c *core.Ctx) error {
		if err := c.NeedAccess("adm_ban_ip"); err != nil {
			return err
		}
		l := a.T.BanIP.All()
		out := make([]map[string]any, 0, len(l))
		for _, b := range l {
			out = append(out, map[string]any{"id": b.ID, "min": ipStr(b.Min), "max": ipStr(b.Max)})
		}
		return c.OK("items", out, "my_ip", c.IP())
	}, core.Auth)
	a.POST("/api/admin/ban-ip", func(c *core.Ctx) error {
		if err := c.NeedAccess("adm_ban_ip"); err != nil {
			return err
		}
		var in struct{ Min, Max string }
		if err := c.Bind(&in); err != nil {
			return err
		}
		mn, mx, err := parse(c, in.Min, in.Max)
		if err != nil {
			return err
		}
		if me := core.IPToInt(c.IP()); me >= mn && me <= mx {
			return core.ErrBad("Нельзя заблокировать диапазон со своим IP")
		}
		a.T.BanIP.Insert(&model.BanIp{Min: mn, Max: mx})
		a.AdminLog(c.User().ID, "Система", "Бан по IP", ipStr(mn)+" – "+ipStr(mx))
		return c.OK()
	}, core.Auth)
	a.DELETE("/api/admin/ban-ip/{id}", func(c *core.Ctx) error {
		if err := c.NeedAccess("adm_ban_ip"); err != nil {
			return err
		}
		if !a.T.BanIP.Delete(c.PathInt("id")) {
			return core.ErrNotFound()
		}
		return c.OK()
	}, core.Auth)

	// операторы связи
	a.GET("/api/admin/opsos", func(c *core.Ctx) error {
		if err := c.NeedAccess("adm_ip_edit"); err != nil {
			return err
		}
		l := a.T.Opsos.All()
		sort.Slice(l, func(i, j int) bool { return l[i].Opsos < l[j].Opsos })
		out := make([]map[string]any, 0, len(l))
		for _, b := range l {
			out = append(out, map[string]any{"id": b.ID, "min": ipStr(b.Min), "max": ipStr(b.Max), "opsos": b.Opsos})
		}
		return c.OK("items", out)
	}, core.Auth)
	a.POST("/api/admin/opsos", func(c *core.Ctx) error {
		if err := c.NeedAccess("adm_ip_edit"); err != nil {
			return err
		}
		var in struct{ Min, Max, Opsos string }
		if err := c.Bind(&in); err != nil {
			return err
		}
		mn, mx, err := parse(c, in.Min, in.Max)
		if err != nil {
			return err
		}
		in.Opsos = strings.TrimSpace(in.Opsos)
		if in.Opsos == "" || rlen(in.Opsos) > 64 {
			return core.ErrBad("Укажите название оператора")
		}
		a.T.Opsos.Insert(&model.Opsos{Min: mn, Max: mx, Opsos: a.Text.Clean(in.Opsos)})
		return c.OK()
	}, core.Auth)
	a.DELETE("/api/admin/opsos/{id}", func(c *core.Ctx) error {
		if err := c.NeedAccess("adm_ip_edit"); err != nil {
			return err
		}
		if !a.T.Opsos.Delete(c.PathInt("id")) {
			return core.ErrNotFound()
		}
		return c.OK()
	}, core.Auth)
}

func registerGroups(a *core.App) {
	// список групп и привилегий
	a.GET("/api/admin/groups", func(c *core.Ctx) error {
		if err := c.NeedAccess("adm_accesses"); err != nil {
			return err
		}
		type g struct {
			model.UserGroup
			Users    int      `json:"users"`
			Accesses []string `json:"accesses"`
		}
		out := []g{}
		for _, x := range sortedGroups(a) {
			it := g{UserGroup: x, Users: a.T.Users.CountBy("group", store.I(x.ID)), Accesses: []string{}}
			for _, ga := range a.T.GroupAccess.By("group", store.I(x.ID)) {
				it.Accesses = append(it.Accesses, ga.IDAccess)
			}
			sort.Strings(it.Accesses)
			out = append(out, it)
		}
		all := a.T.AllAccesses.All()
		sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })
		return c.OK("groups", out, "accesses", all)
	}, core.Auth)
	a.PUT("/api/admin/groups/{id}", func(c *core.Ctx) error {
		if err := c.NeedAccess("adm_accesses"); err != nil {
			return err
		}
		gr, ok := a.T.Groups.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		var in struct {
			Name     string   `json:"name"`
			Accesses []string `json:"accesses"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if n := strings.TrimSpace(in.Name); n != "" && rlen(n) <= 32 {
			a.T.Groups.Update(gr.ID, func(x *model.UserGroup) { x.Name = a.Text.Clean(n) })
		}
		valid := map[string]bool{}
		for _, x := range a.T.AllAccesses.All() {
			valid[x.Type] = true
		}
		a.T.GroupAccess.DeleteBy("group", store.I(gr.ID))
		seen := map[string]bool{}
		for _, ac := range in.Accesses {
			if valid[ac] && !seen[ac] {
				seen[ac] = true
				a.T.GroupAccess.Insert(&model.UserGroupAccess{IDGroup: gr.ID, IDAccess: ac})
			}
		}
		a.ResetAccessCache()
		a.AdminLog(c.User().ID, "Система", "Привилегии", "Изменены привилегии группы '"+gr.Name+"'")
		return c.OK()
	}, core.Auth)
	a.POST("/api/admin/groups", func(c *core.Ctx) error {
		if err := c.NeedAccess("adm_accesses"); err != nil {
			return err
		}
		var in struct {
			Name  string `json:"name"`
			Level int64  `json:"level"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		in.Name = strings.TrimSpace(in.Name)
		if in.Name == "" || rlen(in.Name) > 32 || in.Level < 0 || in.Level > 10 {
			return core.ErrBad("Укажите название и уровень (0–10)")
		}
		g := model.UserGroup{Name: a.Text.Clean(in.Name), Level: in.Level}
		a.T.Groups.Insert(&g)
		return c.OK("id", g.ID)
	}, core.Auth)
	a.DELETE("/api/admin/groups/{id}", func(c *core.Ctx) error {
		if err := c.NeedAccess("adm_accesses"); err != nil {
			return err
		}
		id := c.PathInt("id")
		if a.T.Users.CountBy("group", store.I(id)) > 0 {
			return core.ErrBad("В группе есть пользователи")
		}
		if !a.T.Groups.Delete(id) {
			return core.ErrNotFound()
		}
		a.T.GroupAccess.DeleteBy("group", store.I(id))
		a.ResetAccessCache()
		return c.OK()
	}, core.Auth)

	// список администрации
	a.GET("/api/admin/staff", func(c *core.Ctx) error {
		if err := c.NeedAccess("adm_show_adm"); err != nil {
			return err
		}
		us := a.T.Users.Find(func(u *model.User) bool { return a.Can(u.ID, "adm_panel_show") })
		sort.Slice(us, func(i, j int) bool { return us[i].Level > us[j].Level })
		out := []map[string]any{}
		for _, u := range us {
			out = append(out, map[string]any{"user": a.Ref(u.ID), "last": u.DateLast})
		}
		return c.OK("items", out)
	}, core.Auth)
}

func registerSpam(a *core.App) {
	// жалобы
	a.GET("/api/admin/spam", func(c *core.Ctx) error {
		if err := c.NeedAccess("adm_panel_show"); err != nil {
			return err
		}
		p := c.Page()
		rz := c.Q("razdel")
		list, total := a.T.Spam.Select(store.Opts[model.Spamus]{Desc: true, Offset: p.Offset, Limit: p.Per,
			Where: func(s *model.Spamus) bool { return rz == "" || s.Razdel == rz }})
		out := make([]map[string]any, 0, len(list))
		for _, s := range list {
			out = append(out, map[string]any{"id": s.ID, "razdel": s.Razdel, "from": a.Ref(s.IDUser), "violator": a.Ref(s.IDSpam),
				"msg": a.Text.Render(s.Msg), "text": a.Text.Render(s.Spam), "obj": s.IDObject, "post": s.IDPost, "time": s.Time})
		}
		return c.List(out, total, p)
	}, core.Auth)
	a.DELETE("/api/admin/spam/{id}", func(c *core.Ctx) error {
		if err := c.NeedAccess("adm_panel_show"); err != nil {
			return err
		}
		if !a.T.Spam.Delete(c.PathInt("id")) {
			return core.ErrNotFound()
		}
		return c.OK()
	}, core.Auth)
	a.Counter("spam", func(a *core.App, u *model.User) string {
		if !a.Can(u.ID, "adm_panel_show") {
			return ""
		}
		if n := a.T.Spam.Len(); n > 0 {
			return store.I(int64(n))
		}
		return ""
	})
}
