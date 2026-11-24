package chatnews

import (
	"sort"
	"strings"
	"sync"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

type GuestPost struct {
	store.Base
	IDUser int64  `json:"id_user"` // 0 — гость
	Time   int64  `json:"time"`
	Msg    string `json:"msg"`
}
type AdmPost struct {
	store.Base
	IDUser int64  `json:"id_user"`
	Time   int64  `json:"time"`
	Msg    string `json:"msg"`
}

var (
	guestPosts *store.Table[GuestPost, *GuestPost]
	admPosts   *store.Table[AdmPost, *AdmPost]
	antiflood  sync.Map // ip -> last guest post time
)

func postView(a *core.App, id int64, msg string, t, uid int64) map[string]any {
	m := map[string]any{"id": id, "msg": a.Text.Render(msg), "time": t}
	if uid == 0 {
		m["user"] = map[string]any{"id": 0, "nick": "Гость", "pol": 1}
	} else {
		m["user"] = a.Ref(uid)
	}
	return m
}

func registerGuest(a *core.App) {
	guestPosts = store.NewTable[GuestPost](a.DB, "guest").Index("user", func(p *GuestPost) string { return i(p.IDUser) })

	a.Describer("guest", func(id int64) (core.Desc, bool) {
		p, ok := guestPosts.Get(id)
		if !ok {
			return core.Desc{}, false
		}
		return core.Desc{Title: "Гостевая книга", Text: a.Text.Plain(p.Msg, 160), Link: "/guest", Owner: p.IDUser}, true
	})
	a.OnUserDelete(func(uid int64) { guestPosts.DeleteBy("user", i(uid)) })
	a.Counter("guest", func(a *core.App, u *model.User) string {
		total := guestPosts.Len()
		n := 0
		if u != nil {
			n = guestPosts.Count(func(p *GuestPost) bool { return p.Time > u.DateLast })
		}
		if n > 0 {
			return itoa(total) + " +" + itoa(n)
		}
		return itoa(total)
	})

	a.GET("/api/guest", func(c *core.Ctx) error {
		if err := banErr(a, c.User(), "guest", "в гостевую"); err != nil {
			return err
		}
		me := c.User()
		if me != nil {
			a.T.Notifications.UpdateWhere(func(n *model.Notification) bool { return n.IDUser == me.ID && n.Type == "guest" && n.Read == 0 },
				func(n *model.Notification) { n.Read = 1 })
		}
		p := c.Page()
		list, total := guestPosts.Select(store.Opts[GuestPost]{Desc: true, Offset: p.Offset, Limit: p.Per})
		items := make([]map[string]any, 0, len(list))
		for _, x := range list {
			m := postView(a, x.ID, x.Msg, x.Time, x.IDUser)
			if me != nil {
				m["can_reply"] = x.IDUser != 0 && x.IDUser != me.ID
				m["can_delete"] = a.Can(me.ID, "guest_delete") && (x.IDUser == 0 || me.Level > a.Ref(x.IDUser).Level || (me.Level != 0 && me.ID == x.IDUser))
			}
			items = append(items, m)
		}
		canWrite := me != nil || a.Cfg.Bool("write_guest")
		wait := int64(0)
		if me == nil && canWrite {
			if t, ok := antiflood.Load(c.IP()); ok {
				if w := t.(int64) + 300 - core.Now(); w > 0 {
					wait = w
				}
			}
		}
		out := map[string]any{"items": items, "total": total, "page": p.Page, "per": p.Per, "pages": max(1, (total+p.Per-1)/p.Per),
			"can_write": canWrite, "captcha": me == nil, "wait": wait, "reading": len(onlineByURL(a, "/guest")), "can_clear": c.Can("guest_clear")}
		if me != nil {
			out["my_posts"] = me.GuestMess
		}
		return c.JSON(out)
	})

	a.POST("/api/guest", func(c *core.Ctx) error {
		var in struct {
			Msg     string `json:"msg"`
			ReplyTo int64  `json:"reply_to"`
			Captcha string `json:"captcha"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		me := c.User()
		msg := strings.TrimSpace(in.Msg)
		if err := noMat(a, msg, "тексте сообщения"); err != nil {
			return err
		}
		if err := lenOK(msg, 2, 1024, "Сообщение"); err != nil {
			return err
		}
		now := core.Now()
		if me == nil {
			if !a.Cfg.Bool("write_guest") {
				return core.ErrAuth()
			}
			if !core.CaptchaCheck(c, in.Captcha) {
				return core.ErrBad("Неверное проверочное число")
			}
			if t, ok := antiflood.Load(c.IP()); ok && t.(int64) > now-300 {
				return core.ErrBad("Для того чтобы чаще писать нужно авторизоваться")
			}
			antiflood.Store(c.IP(), now)
			id := guestPosts.Insert(&GuestPost{Time: now, Msg: msg})
			return c.OK("id", id)
		}
		if err := banErr(a, me, "guest", "в гостевую"); err != nil {
			return err
		}
		if guestPosts.Count(func(p *GuestPost) bool { return p.IDUser == me.ID && p.Msg == msg }) > 0 {
			return core.ErrBad("Ваше сообщение повторяет предыдущее")
		}
		id := guestPosts.Insert(&GuestPost{IDUser: me.ID, Time: now, Msg: msg})
		a.T.Users.Update(me.ID, func(u *model.User) { u.GuestMess++ })
		a.AddBalls(me.ID, 1, 0)
		if in.ReplyTo != 0 {
			a.Notify(me.ID, in.ReplyTo, "guest", 0, true)
		}
		return c.OK("id", id)
	})

	a.DELETE("/api/guest/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if err := c.NeedAccess("guest_delete"); err != nil {
			return err
		}
		p, ok := guestPosts.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		nick := "Гость"
		if p.IDUser != 0 {
			au := a.Ref(p.IDUser)
			if me.Level <= au.Level && me.ID != p.IDUser {
				return core.ErrForbidden("Нельзя удалить сообщение пользователя с равным или большим уровнем")
			}
			nick = au.Nick
		}
		guestPosts.Delete(p.ID)
		a.AdminLog(me.ID, "Гостевая", "Удаление сообщения", "Удаление сообщения от "+nick)
		return c.OK()
	}, core.Auth)

	a.POST("/api/guest-clear", func(c *core.Ctx) error {
		if err := c.NeedAccess("guest_clear"); err != nil {
			return err
		}
		var in struct {
			Write int64  `json:"write"`
			Unit  string `json:"unit"` // sut | mes
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		var t int64
		switch in.Unit {
		case "sut":
			t = core.Now() - in.Write*86400
		case "mes":
			t = core.Now() - in.Write*86400*30
		default:
			return core.ErrBad("Неверная единица времени")
		}
		n := guestPosts.DeleteWhere(func(p *GuestPost) bool { return p.Time < t })
		a.AdminLog(c.UID(), "Гостевая", "Очистка", "Удалено "+itoa(n)+" постов")
		return c.OK("deleted", n)
	}, core.Auth)

	a.GET("/api/guest-top", func(c *core.Ctx) error {
		users := a.T.Users.Find(func(u *model.User) bool { return u.GuestMess > 0 })
		sort.Slice(users, func(x, y int) bool { return users[x].GuestMess > users[y].GuestMess })
		p := c.Page()
		items := []map[string]any{}
		for x := p.Offset; x < min(p.Offset+p.Per, len(users)); x++ {
			items = append(items, map[string]any{"user": a.Ref(users[x].ID), "total": users[x].GuestMess, "active": guestPosts.CountBy("user", i(users[x].ID))})
		}
		return c.List(items, len(users), p)
	})

	a.GET("/api/guest-who", func(c *core.Ctx) error {
		list := onlineByURL(a, "/guest")
		sort.Slice(list, func(x, y int) bool { return list[x].DateLast > list[y].DateLast })
		p := c.Page()
		items := []map[string]any{}
		for x := p.Offset; x < min(p.Offset+p.Per, len(list)); x++ {
			items = append(items, map[string]any{"user": a.Ref(list[x].ID), "date_last": list[x].DateLast})
		}
		return c.List(items, len(list), p)
	})
}

// ---------- админ-чат (adm_chat)

func registerAdminChat(a *core.App) {
	admPosts = store.NewTable[AdmPost](a.DB, "adm_chat")
	a.OnUserDelete(func(uid int64) { admPosts.DeleteWhere(func(p *AdmPost) bool { return p.IDUser == uid }) })

	isStaff := func(u *model.User) bool {
		return u != nil && (u.Level >= 1 || a.Can(u.ID, "adm_panel_show"))
	}
	need := func(c *core.Ctx) (*model.User, error) {
		me, err := c.Need()
		if err != nil {
			return nil, err
		}
		if !isStaff(me) {
			return nil, core.ErrForbidden("Доступно только администрации и модераторам")
		}
		return me, nil
	}
	notifyStaff := func(event string, data any) {
		a.Hub.Publish("adm_chat", event, data)
		now := core.Now()
		for _, u := range a.T.Users.All() {
			if u.DateLast > now-900 && isStaff(&u) {
				a.Hub.ToUser(u.ID, event, data)
			}
		}
	}

	a.GET("/api/adm-chat", func(c *core.Ctx) error {
		me, err := need(c)
		if err != nil {
			return err
		}
		a.T.Notifications.UpdateWhere(func(n *model.Notification) bool { return n.IDUser == me.ID && n.Type == "adm_komm" && n.Read == 0 },
			func(n *model.Notification) { n.Read = 1 })
		limit := int(c.QInt("limit"))
		if limit < 1 || limit > 150 {
			limit = 80
		}
		list, total := admPosts.Select(store.Opts[AdmPost]{Desc: true, Offset: 0, Limit: limit})
		// Хронологический порядок: старые сверху, новые снизу (как в личной переписке)
		for l, r := 0, len(list)-1; l < r; l, r = l+1, r-1 {
			list[l], list[r] = list[r], list[l]
		}
		canModDel := me.Level >= 3 || c.Can("guest_delete")
		items := make([]map[string]any, 0, len(list))
		for _, x := range list {
			m := postView(a, x.ID, x.Msg, x.Time, x.IDUser)
			m["can_reply"] = x.IDUser != me.ID
			m["can_delete"] = x.IDUser == me.ID || canModDel
			items = append(items, m)
		}
		now := core.Now()
		staffOnline := 0
		for _, u := range a.T.Users.All() {
			if u.DateLast > now-600 && isStaff(&u) {
				staffOnline++
			}
		}
		return c.JSON(map[string]any{
			"items":        items,
			"total":        total,
			"can_write":    true,
			"staff_online": staffOnline,
		})
	}, core.Auth)

	a.POST("/api/adm-chat", func(c *core.Ctx) error {
		me, err := need(c)
		if err != nil {
			return err
		}
		var in struct {
			Msg     string `json:"msg"`
			ReplyTo int64  `json:"reply_to"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		msg := strings.TrimSpace(in.Msg)
		if err := lenOK(msg, 1, 2048, "Сообщение"); err != nil {
			return err
		}
		now := core.Now()
		id := admPosts.Insert(&AdmPost{IDUser: me.ID, Time: now, Msg: msg})
		if in.ReplyTo != 0 && in.ReplyTo != me.ID {
			a.Notify(me.ID, in.ReplyTo, "adm_komm", 0, true)
		}
		dto := postView(a, id, msg, now, me.ID)
		dto["can_reply"] = true
		dto["can_delete"] = false
		notifyStaff("adm_chat", dto)
		dto["can_delete"] = true
		return c.JSON(dto)
	}, core.Auth)

	a.DELETE("/api/adm-chat/{id}", func(c *core.Ctx) error {
		me, err := need(c)
		if err != nil {
			return err
		}
		p, ok := admPosts.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		if p.IDUser != me.ID && me.Level < 3 && !c.Can("guest_delete") {
			return core.ErrForbidden("Нет прав на удаление этого сообщения")
		}
		admPosts.Delete(p.ID)
		notifyStaff("adm_chat_del", map[string]any{"id": p.ID})
		return c.OK()
	}, core.Auth)
}
