package forum

import (
	"regexp"
	"strings"

	"dcms/internal/core"
	"dcms/internal/model"
)

var reIcon = regexp.MustCompile(`[^a-z0-9 _\-\.]`)

func forumView(a *core.App, f Forum, withRazdels bool) map[string]any {
	rs := razdels.By("forum", i(f.ID))
	nt, np := 0, 0
	rv := []map[string]any{}
	for _, r := range rs {
		ts := thems.By("razdel", i(r.ID))
		pc := posts.Count(func(p *Post) bool { return p.IDRazdel == r.ID })
		nt += len(ts)
		np += pc
		if withRazdels {
			rm := map[string]any{"id": r.ID, "name": r.Name, "opis": r.Opis, "time": r.Time, "themes": len(ts), "posts": pc}
			rv = append(rv, rm)
		}
	}
	m := map[string]any{"id": f.ID, "name": f.Name, "opis": f.Opis, "pos": f.Pos, "icon": f.Icon, "adm": f.Adm, "razdels": len(rs), "themes": nt, "posts": np}
	if withRazdels {
		sortByTimeDesc(rv)
		m["items"] = rv
	}
	return m
}

func sortByTimeDesc(rv []map[string]any) {
	for x := 1; x < len(rv); x++ {
		for y := x; y > 0 && rv[y]["time"].(int64) > rv[y-1]["time"].(int64); y-- {
			rv[y], rv[y-1] = rv[y-1], rv[y]
		}
	}
}

func registerStructure(a *core.App) {
	// главная форума
	a.GET("/api/forum", func(c *core.Ctx) error {
		if err := forumBan(a, c.User()); err != nil {
			return err
		}
		list := forums.Find(func(f *Forum) bool { return visibleForum(*f, c.User()) })
		for x := 1; x < len(list); x++ {
			for y := x; y > 0 && list[y].Pos < list[y-1].Pos; y-- {
				list[y], list[y-1] = list[y-1], list[y]
			}
		}
		items := make([]map[string]any, 0, len(list))
		for _, f := range list {
			items = append(items, forumView(a, f, false))
		}
		// кто сейчас на форуме
		now := core.Now()
		on := a.T.Users.Find(func(u *model.User) bool {
			return u.DateLast > now-100 && strings.HasPrefix(u.URL, "/forum") && u.ShowURL || u.DateLast > now-100 && strings.HasPrefix(u.URL, "/forum")
		})
		refs := []model.UserRef{}
		for x := 0; x < len(on) && len(refs) < 10; x++ {
			refs = append(refs, a.Ref(on[x].ID))
		}
		return c.JSON(map[string]any{"items": items, "online": refs, "online_total": len(on)})
	})

	// создать подфорум
	a.POST("/api/forum", func(c *core.Ctx) error {
		if err := c.NeedAccess("forum_for_create"); err != nil {
			return err
		}
		f, err := bindForum(c, Forum{})
		if err != nil {
			return err
		}
		forums.Insert(&f)
		a.AdminLog(c.UID(), "Форум", "Подфорумы", "Создание подфорума '"+f.Name+"'")
		return c.JSON(forumView(a, f, false))
	}, core.Auth)

	a.GET("/api/forum/{f}", func(c *core.Ctx) error {
		if err := forumBan(a, c.User()); err != nil {
			return err
		}
		f, err := forumOf(c)
		if err != nil {
			return err
		}
		return c.JSON(forumView(a, f, true))
	})

	a.PUT("/api/forum/{f}", func(c *core.Ctx) error {
		if err := c.NeedAccess("forum_for_edit"); err != nil {
			return err
		}
		f, err := forumOf(c)
		if err != nil {
			return err
		}
		nf, err := bindForum(c, f)
		if err != nil {
			return err
		}
		if nf.Name != f.Name {
			a.AdminLog(c.UID(), "Форум", "Подфорумы", "Подфорум '"+f.Name+"' переименован в '"+nf.Name+"'")
		}
		if nf.Opis != f.Opis {
			a.AdminLog(c.UID(), "Форум", "Подфорумы", "Изменено описание подфорума '"+nf.Name+"'")
		}
		if nf.Adm != f.Adm {
			if c.User().Level >= 3 {
				a.AdminLog(c.UID(), "Форум", "Подфорумы", "Изменена доступность подфорума '"+nf.Name+"' (только для администрации: "+map[bool]string{true: "да", false: "нет"}[nf.Adm]+")")
			} else {
				nf.Adm = f.Adm
			}
		}
		forums.Put(nf)
		return c.JSON(forumView(a, nf, false))
	}, core.Auth)

	a.DELETE("/api/forum/{f}", func(c *core.Ctx) error {
		if err := c.NeedAccess("forum_for_delete"); err != nil {
			return err
		}
		f, err := forumOf(c)
		if err != nil {
			return err
		}
		for _, r := range razdels.By("forum", i(f.ID)) {
			dropRazdel(a, r)
		}
		forums.Delete(f.ID)
		a.AdminLog(c.UID(), "Форум", "Подфорумы", "Удаление подфорума '"+f.Name+"'")
		return c.OK()
	}, core.Auth)

	// ---- разделы
	a.POST("/api/forum/{f}/razdel", func(c *core.Ctx) error {
		if err := c.NeedAccess("forum_razd_create"); err != nil {
			return err
		}
		f, err := forumOf(c)
		if err != nil {
			return err
		}
		var in struct{ Name, Opis string }
		if err := c.Bind(&in); err != nil {
			return err
		}
		name := strings.TrimSpace(in.Name)
		if err := lenOK(name, 2, 32, "Название"); err != nil {
			return err
		}
		r := Razdel{IDForum: f.ID, Name: name, Opis: strings.TrimSpace(in.Opis), Time: core.Now()}
		if err := lenOK2(r.Opis, 256, "Описание"); err != nil {
			return err
		}
		razdels.Insert(&r)
		a.AdminLog(c.UID(), "Форум", "Разделы", "Создание раздела '"+name+"' в подфоруме '"+f.Name+"'")
		return c.JSON(map[string]any{"id": r.ID})
	}, core.Auth)

	a.GET("/api/forum/{f}/{r}", func(c *core.Ctx) error {
		if err := forumBan(a, c.User()); err != nil {
			return err
		}
		f, r, err := razdelOf(c)
		if err != nil {
			return err
		}
		p := c.Page()
		list := thems.By("razdel", i(r.ID))
		for x := 1; x < len(list); x++ { // закреплённые сверху, затем по времени
			for y := x; y > 0 && (list[y].Up && !list[y-1].Up || list[y].Up == list[y-1].Up && list[y].Time > list[y-1].Time); y-- {
				list[y], list[y-1] = list[y-1], list[y]
			}
		}
		items := []map[string]any{}
		for x := p.Offset; x < min(p.Offset+p.Per, len(list)); x++ {
			items = append(items, themeSummary(a, list[x]))
		}
		u := c.User()
		_, throttled := canCreateTheme(u)
		return c.JSON(map[string]any{"forum": map[string]any{"id": f.ID, "name": f.Name}, "razdel": map[string]any{"id": r.ID, "name": r.Name, "opis": r.Opis},
			"items": items, "total": len(list), "page": p.Page, "per": p.Per, "pages": max(1, (len(list)+p.Per-1)/p.Per),
			"can_create": u != nil && !throttled, "throttled": throttled,
			"can_edit": c.Can("forum_razd_edit"), "can_delete": c.Can("forum_razd_edit")})
	})

	a.PUT("/api/forum/{f}/{r}", func(c *core.Ctx) error {
		if err := c.NeedAccess("forum_razd_edit"); err != nil {
			return err
		}
		f, r, err := razdelOf(c)
		if err != nil {
			return err
		}
		var in struct{ Name, Opis string }
		if err := c.Bind(&in); err != nil {
			return err
		}
		name := strings.TrimSpace(in.Name)
		if err := lenOK(name, 3, 32, "Название"); err != nil {
			return err
		}
		if err := lenOK2(in.Opis, 256, "Описание"); err != nil {
			return err
		}
		r.Name, r.Opis = name, strings.TrimSpace(in.Opis)
		razdels.Put(r)
		a.AdminLog(c.UID(), "Форум", "Разделы", "Изменён раздел '"+name+"' подфорума '"+f.Name+"'")
		return c.OK()
	}, core.Auth)

	a.POST("/api/forum/{f}/{r}/move", func(c *core.Ctx) error { // перенос раздела в другой подфорум
		if err := c.NeedAccess("forum_razd_edit"); err != nil {
			return err
		}
		f, r, err := razdelOf(c)
		if err != nil {
			return err
		}
		var in struct {
			Forum int64 `json:"forum"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		nf, ok := forums.Get(in.Forum)
		if !ok {
			return core.ErrBad("Подфорум не найден")
		}
		r.IDForum = nf.ID
		razdels.Put(r)
		thems.UpdateWhere(func(t *Them) bool { return t.IDRazdel == r.ID }, func(t *Them) { t.IDForum = nf.ID })
		posts.UpdateWhere(func(p *Post) bool { return p.IDRazdel == r.ID }, func(p *Post) { p.IDForum = nf.ID })
		a.AdminLog(c.UID(), "Форум", "Разделы", "Раздел '"+r.Name+"' перемещён из '"+f.Name+"' в '"+nf.Name+"'")
		return c.OK("forum", nf.ID)
	}, core.Auth)

	a.DELETE("/api/forum/{f}/{r}", func(c *core.Ctx) error {
		if err := c.NeedAccess("forum_razd_edit"); err != nil {
			return err
		}
		f, r, err := razdelOf(c)
		if err != nil {
			return err
		}
		dropRazdel(a, r)
		a.AdminLog(c.UID(), "Форум", "Разделы", "Удаление раздела '"+r.Name+"' подфорума '"+f.Name+"'")
		return c.OK()
	}, core.Auth)

	// дерево для выбора при переносе тем/разделов
	a.GET("/api/forum-tree", func(c *core.Ctx) error {
		out := []map[string]any{}
		for _, f := range forums.All() {
			if !visibleForum(f, c.User()) {
				continue
			}
			rs := []map[string]any{}
			for _, r := range razdels.By("forum", i(f.ID)) {
				rs = append(rs, map[string]any{"id": r.ID, "name": r.Name})
			}
			out = append(out, map[string]any{"id": f.ID, "name": f.Name, "pos": f.Pos, "razdels": rs})
		}
		return c.JSON(out)
	})
}

func dropRazdel(a *core.App, r Razdel) {
	for _, t := range thems.By("razdel", i(r.ID)) {
		deleteTheme(a, t)
	}
	razdels.Delete(r.ID)
}

func lenOK2(s string, max int, what string) error {
	if core.RuneLen(s) > max {
		return core.ErrBad(what + " слишком длинное")
	}
	return nil
}

func bindForum(c *core.Ctx, cur Forum) (Forum, error) {
	var in struct {
		Name, Opis, Icon string
		Pos              int64
		Adm              bool
	}
	if err := c.Bind(&in); err != nil {
		return cur, err
	}
	name := strings.TrimSpace(in.Name)
	if err := lenOK(name, 3, 32, "Название"); err != nil {
		return cur, err
	}
	if err := lenOK2(in.Opis, 512, "Описание"); err != nil {
		return cur, err
	}
	icon := reIcon.ReplaceAllString(strings.ToLower(in.Icon), "")
	if icon == "" {
		icon = "default"
	}
	cur.Name, cur.Opis, cur.Icon, cur.Pos = name, strings.TrimSpace(in.Opis), icon, in.Pos
	if c.User() != nil && c.User().Level >= 3 {
		cur.Adm = in.Adm
	}
	return cur, nil
}
