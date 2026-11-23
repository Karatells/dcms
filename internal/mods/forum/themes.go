package forum

import (
	"strings"
	"time"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

const themeCooldown = 600

// canCreateTheme — (можно ли, ограничено ли по частоте). Не-модераторы создают темы не чаще раза в 10 минут.
func canCreateTheme(u *model.User) (bool, bool) {
	if u == nil {
		return false, false
	}
	if u.Level > 0 {
		return true, false
	}
	if v, ok := themeThrottle.Load(u.ID); ok && core.Now()-v.(int64) < themeCooldown {
		return false, true
	}
	return true, false
}

func canEditTheme(c *core.Ctx, t Them) bool {
	u := c.User()
	return u != nil && (c.Can("forum_them_edit") || t.IDUser == u.ID)
}

func canWritePost(c *core.Ctx, t Them) bool {
	u := c.User()
	return u != nil && (!t.Close || c.Can("forum_post_close"))
}

func postCan(c *core.Ctx, a *core.App, p Post, last Post) (canEdit, canDel bool) {
	u := c.User()
	if u == nil || p.IDUser == 0 {
		return false, false
	}
	author, _ := a.GetUser(p.IDUser)
	t, _ := thems.Get(p.IDThem)
	if c.Can("forum_post_ed") && (author.Level < u.Level || p.IDUser == u.ID) {
		return true, true
	}
	if p.IDUser == u.ID && !t.Close {
		if p.ID == last.ID && core.Now()-p.Time < 600 {
			return true, true
		}
	}
	return false, false
}

func registerThemes(a *core.App) {
	// ---- создание темы
	a.POST("/api/forum/{f}/{r}/new", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if err := forumBan(a, me); err != nil {
			return err
		}
		f, r, err := razdelOf(c)
		if err != nil {
			return err
		}
		if ok, thr := canCreateTheme(me); !ok {
			if thr {
				return core.ErrBad("Нельзя так часто создавать темы")
			}
			return core.ErrAuth()
		}
		var in struct {
			Name, Msg string
			Vote      *pollIn `json:"poll"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		name := a.Text.Clean(in.Name)
		msg := a.Text.Clean(in.Msg)
		if err := lenOK(name, 3, 32, "Название темы"); err != nil {
			return err
		}
		if err := noMat(a, name+" "+msg, "тексте темы"); err != nil {
			return err
		}
		if err := lenOK(msg, 10, 30000, "Сообщение"); err != nil {
			return err
		}
		now := core.Now()
		t := Them{IDForum: f.ID, IDRazdel: r.ID, Name: name, IDUser: me.ID, TimeCreate: now, Time: now, Text: msg}
		if in.Vote != nil && strings.TrimSpace(in.Vote.Text) != "" {
			if err := validatePoll(a, in.Vote); err != nil {
				return err
			}
		}
		thems.Insert(&t)
		if in.Vote != nil && strings.TrimSpace(in.Vote.Text) != "" {
			savePoll(&t, in.Vote)
		}
		themeThrottle.Store(me.ID, now)
		razdels.Update(r.ID, func(x *Razdel) { x.Time = now })
		a.AddBalls(me.ID, 0, 1)
		if !f.Adm {
			a.FanTape(me.ID, core.KForum, "them", t.ID, core.TapeOpts{})
		}
		return c.JSON(map[string]any{"id": t.ID, "link": themeLink(t)})
	}, core.Auth)

	// ---- просмотр темы
	a.GET("/api/forum/{f}/{r}/{t}", func(c *core.Ctx) error {
		if err := forumBan(a, c.User()); err != nil {
			return err
		}
		f, r, t, err := themOf(c)
		if err != nil {
			return err
		}
		me := c.User()
		desc := c.Q("order") == "desc"
		total := postCount(t)
		p := c.Page()
		if c.Q("page") == "last" {
			p.Page = max(1, (total+p.Per-1)/p.Per)
			p.Offset = (p.Page - 1) * p.Per
		}
		if pid := c.QInt("post"); pid > 0 { // перейти к странице с сообщением
			n := 0
			for _, x := range posts.By("them", i(t.ID)) {
				if x.ID < pid {
					n++
				}
			}
			p.Page = n/p.Per + 1
			p.Offset = (p.Page - 1) * p.Per
		}
		list, _ := posts.Select(store.Opts[Post]{Index: "them", Key: i(t.ID), Desc: desc, Offset: p.Offset, Limit: p.Per,
			Less: func(x, y *Post) bool { return x.Time < y.Time || x.Time == y.Time && x.ID < y.ID }})
		last, _ := lastPost(t)
		items := make([]map[string]any, 0, len(list))
		for _, pp := range list {
			items = append(items, postView(a, c, pp, last))
		}
		out := map[string]any{
			"forum": map[string]any{"id": f.ID, "name": f.Name}, "razdel": map[string]any{"id": r.ID, "name": r.Name},
			"id": t.ID, "name": t.Name, "text": a.Text.Render(t.Text), "raw_text": t.Text, "close": t.Close, "up": t.Up,
			"author": a.Ref(t.IDUser), "time_create": t.TimeCreate, "time": t.Time,
			"items": items, "total": total, "page": p.Page, "per": p.Per, "pages": max(1, (total+p.Per-1)/p.Per),
		}
		if t.IDEdit > 0 {
			out["edited_by"], out["time_edit"] = a.Ref(t.IDEdit), t.TimeEdit
		}
		if t.Close && t.IDClose > 0 {
			out["closed_by"] = a.Ref(t.IDClose)
		}
		if poll := pollView(a, t, me); poll != nil {
			out["poll"] = poll
		}
		out["can_edit"] = canEditTheme(c, t)
		out["can_delete"] = me != nil && (c.Can("forum_them_del") && (mustUser(a, t.IDUser).Level <= me.Level) || t.IDUser == me.ID)
		out["can_post"] = canWritePost(c, t)
		out["is_staff"] = isStaff(me)
		if me != nil {
			_, fav := a.T.Bookmarks.OneBy("uobj", store.Key(me.ID, "forum", t.ID))
			out["bookmarked"] = fav
			out["my_posts"] = me.ForumThemComm
			// сбросить уведомления и счётчик обсуждения
			a.T.Notifications.UpdateWhere(func(n *model.Notification) bool {
				return n.IDUser == me.ID && n.Type == "them_komm" && n.IDObject == t.ID && n.Read == 0
			}, func(n *model.Notification) { n.Read = 1 })
			for _, d := range a.T.Discussions.By("user", i(me.ID)) {
				if d.Type == "them" && d.IDSim == t.ID && d.Count > 0 {
					a.T.Discussions.Update(d.ID, func(x *model.Discussions) { x.Count = 0 })
				}
			}
			// закладка: отметка «просмотрено»
			if b, ok := a.T.Bookmarks.OneBy("uobj", store.Key(me.ID, "forum", t.ID)); ok {
				a.T.Bookmarks.Update(b.ID, func(x *model.Bookmarks) { x.TimeObn = core.Now() })
			}
		}
		return c.JSON(out)
	})

	// ---- настройки темы (название, текст, закрепление, закрытие, автор)
	a.PUT("/api/forum/{f}/{r}/{t}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		f, r, t, err := themOf(c)
		if err != nil {
			return err
		}
		if !canEditTheme(c, t) {
			return core.ErrForbidden()
		}
		var in struct {
			Name, Text string
			Up, Close  *bool
			Autor      bool
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		name := a.Text.Clean(in.Name)
		if err := lenOK(name, 3, 32, "Название"); err != nil {
			return err
		}
		text := a.Text.Clean(in.Text)
		if err := lenOK(text, 10, 30000, "Текст темы"); err != nil {
			return err
		}
		own := t.IDUser == me.ID
		logIt := func(act, d string) {
			if !own {
				a.AdminLog(me.ID, "Форум", "Параметры темы", d+" '"+t.Name+"' (раздел '"+r.Name+"')")
			}
		}
		grp := groupName(a, me) + " " + me.Nick
		if in.Up != nil && me.Level > 0 {
			if *in.Up && !t.Up {
				logIt("up", "Закрепление темы")
				sysPost(a, t, "[red]Тему закрепил "+grp+"[/red]")
			} else if !*in.Up && t.Up {
				logIt("up", "Открепление темы")
				sysPost(a, t, "[red]Тему открепил "+grp+"[/red]")
			}
			t.Up = *in.Up
		}
		if in.Close != nil {
			if *in.Close && !t.Close {
				t.IDClose = me.ID
				logIt("close", "Закрытие темы")
				sysPost(a, t, "[red]Тему закрыл "+grp+"[/red]")
			} else if !*in.Close && t.Close {
				logIt("close", "Открытие темы")
				sysPost(a, t, "[red]Тему открыл "+grp+"[/red]")
			}
			t.Close = *in.Close
		}
		if in.Autor && !own {
			t.IDUser = me.ID
		}
		t.Name, t.Text, t.IDEdit, t.TimeEdit = name, text, me.ID, core.Now()
		thems.Put(t)
		_ = f
		return c.OK()
	}, core.Auth)

	// ---- перенос темы
	a.POST("/api/forum/{f}/{r}/{t}/move", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		f, r, t, err := themOf(c)
		if err != nil {
			return err
		}
		if !canEditTheme(c, t) {
			return core.ErrForbidden()
		}
		var in struct {
			Razdel int64 `json:"razdel"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		nr, ok := razdels.Get(in.Razdel)
		if !ok {
			return core.ErrBad("Раздел не найден")
		}
		if !c.Can("forum_them_edit") && nr.IDForum != f.ID { // автор может переносить только внутри подфорума
			return core.ErrForbidden("Переносить можно только в пределах подфорума")
		}
		nf, _ := forums.Get(nr.IDForum)
		if !visibleForum(nf, me) {
			return core.ErrForbidden()
		}
		posts.UpdateWhere(func(p *Post) bool { return p.IDThem == t.ID }, func(p *Post) { p.IDForum, p.IDRazdel = nr.IDForum, nr.ID })
		t.IDForum, t.IDRazdel = nr.IDForum, nr.ID
		thems.Put(t)
		sysPost(a, t, "[red]Тему переместил "+groupName(a, me)+" "+me.Nick+" из раздела "+r.Name+" в раздел "+nr.Name+"[/red]")
		if t.IDUser != me.ID {
			a.AdminLog(me.ID, "Форум", "Перемещение темы", "Перемещение темы '"+t.Name+"' из раздела '"+r.Name+"' в раздел '"+nr.Name+"'")
		}
		return c.JSON(map[string]any{"link": themeLink(t)})
	}, core.Auth)

	// ---- удаление темы
	a.DELETE("/api/forum/{f}/{r}/{t}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		_, _, t, err := themOf(c)
		if err != nil {
			return err
		}
		author := mustUser(a, t.IDUser)
		if !(t.IDUser == me.ID || c.Can("forum_them_del") && author.Level <= me.Level) {
			return core.ErrForbidden()
		}
		deleteTheme(a, t)
		if t.IDUser != me.ID {
			a.AdminLog(me.ID, "Форум", "Удаление темы", "Удаление темы '"+t.Name+"' (автор '"+author.Nick+"')")
		}
		return c.OK()
	}, core.Auth)

	// ---- закладка темы
	a.POST("/api/forum-theme/{t}/bookmark", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		t, ok := thems.Get(c.PathInt("t"))
		if !ok {
			return core.ErrNotFound()
		}
		var in struct {
			On bool `json:"on"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		ex, has := a.T.Bookmarks.OneBy("uobj", store.Key(me.ID, "forum", t.ID))
		if in.On && !has {
			a.T.Bookmarks.Insert(&model.Bookmarks{IDUser: me.ID, IDObject: t.ID, Type: "forum", Time: core.Now(), TimeObn: core.Now()})
		} else if !in.On && has {
			a.T.Bookmarks.Delete(ex.ID)
		}
		return c.OK()
	}, core.Auth)
}

func mustUser(a *core.App, id int64) model.User { u, _ := a.GetUser(id); return u }

var _ = time.Now
