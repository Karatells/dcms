// Package feeds — лента, обсуждения, уведомления, личная почта и контакты.
package feeds

import (
	"encoding/json"
	"strings"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

func Register(a *core.App) {
	registerTape(a)
	registerDiscussions(a)
	registerNotifications(a)
	registerMail(a)
}

// flagsOf — флаги настроек (все числовые поля, кроме id/служебных) в виде map.
func flagsOf(v any, prefix string) map[string]int64 {
	raw, _ := json.Marshal(v)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	out := map[string]int64{}
	for k, x := range m {
		if f, ok := x.(float64); ok && strings.HasPrefix(k, prefix) {
			out[k] = int64(f)
		}
	}
	return out
}

// applyFlags сливает в структуру переданные 0/1-флаги (только известные ключи с префиксом).
func applyFlags[T any](dst *T, in map[string]int64, prefix string) {
	raw, _ := json.Marshal(dst)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	for k, v := range in {
		if _, ok := m[k]; ok && strings.HasPrefix(k, prefix) && (v == 0 || v == 1) {
			m[k] = v
		}
	}
	raw, _ = json.Marshal(m)
	_ = json.Unmarshal(raw, dst)
}

// ---------------------------------------------------------------- лента

// tapeDesc — тип описателя для записи ленты.
func describeTape(a *core.App, t model.Tape) map[string]any {
	typ := t.Type
	dt := typ
	switch typ {
	case "frends":
		dt = "user"
	case "avatar":
		dt = "foto"
	case "status_like":
		dt = "status"
	case "files":
		dt = "obmen"
	}
	m := map[string]any{}
	if d, ok := a.Describe(dt, t.IDFile); ok {
		m["desc"] = d
	}
	if typ == "avatar" && t.Avatar > 0 {
		if d, ok := a.Describe("foto", t.Avatar); ok {
			m["old"] = d
		}
	}
	return m
}

func registerTape(a *core.App) {
	a.GET("/api/tape", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		p := c.Page()
		list, total := a.T.Tape.Select(store.Opts[model.Tape]{Index: "user", Key: store.I(me.ID), Desc: true, Offset: p.Offset, Limit: p.Per,
			Less: func(x, y *model.Tape) bool { return x.Time < y.Time }})
		items := make([]map[string]any, 0, len(list))
		for _, t := range list {
			it := describeTape(a, t)
			it["id"], it["type"], it["time"], it["read"] = t.ID, t.Type, t.Time, t.Read
			it["author"] = a.Ref(t.Avtor)
			if t.OtKogo > 0 {
				it["who"] = a.Ref(t.OtKogo)
			}
			it["mine"] = t.Avtor == me.ID
			items = append(items, it)
			if !t.Read {
				a.T.Tape.Update(t.ID, func(x *model.Tape) { x.Read = true })
			}
		}
		return c.List(items, total, p)
	}, core.Auth)

	a.POST("/api/tape/read", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		a.T.Tape.UpdateWhere(func(t *model.Tape) bool { return t.IDUser == me.ID && !t.Read }, func(t *model.Tape) { t.Read = true })
		return c.OK()
	}, core.Auth)
	a.DELETE("/api/tape", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		a.T.Tape.DeleteBy("user", store.I(me.ID))
		return c.OK()
	}, core.Auth)

	// общие настройки ленты
	a.GET("/api/tape/settings", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		return c.JSON(flagsOf(a.TapeSettings(me.ID), "lenta_"))
	}, core.Auth)
	a.PUT("/api/tape/settings", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		var in map[string]int64
		if err := c.Bind(&in); err != nil {
			return err
		}
		s := a.TapeSettings(me.ID)
		a.T.TapeSet.Update(s.ID, func(x *model.TapeSet) { applyFlags(x, in, "lenta_") })
		return c.OK()
	}, core.Auth)

	// настройки по конкретному другу
	friendRow := func(c *core.Ctx) (model.Frends, error) {
		me, err := c.Need()
		if err != nil {
			return model.Frends{}, err
		}
		f, ok := a.T.Friends.OneBy("pair", store.Key(me.ID, c.PathInt("id")))
		if !ok || f.I != 1 {
			return f, core.ErrNotFound("Пользователь не в списке ваших друзей")
		}
		return f, nil
	}
	for _, kind := range []struct{ path, prefix string }{{"tape", "lenta_"}, {"discussions", "disc_"}} {
		kind := kind
		a.GET("/api/"+kind.path+"/settings/friend/{id}", func(c *core.Ctx) error {
			f, err := friendRow(c)
			if err != nil {
				return err
			}
			return c.JSON(map[string]any{"user": a.Ref(f.Frend), "settings": flagsOf(f, kind.prefix)})
		}, core.Auth)
		a.PUT("/api/"+kind.path+"/settings/friend/{id}", func(c *core.Ctx) error {
			f, err := friendRow(c)
			if err != nil {
				return err
			}
			var in map[string]int64
			if err := c.Bind(&in); err != nil {
				return err
			}
			a.T.Friends.Update(f.ID, func(x *model.Frends) { applyFlags(x, in, kind.prefix) })
			return c.OK()
		}, core.Auth)
	}

}

// ---------------------------------------------------------------- обсуждения

func registerDiscussions(a *core.App) {
	a.GET("/api/discussions", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		order := c.Q("order") // my | frends | ""
		p := c.Page()
		pred := func(d *model.Discussions) bool {
			switch order {
			case "my":
				return d.Avtor == me.ID
			case "frends":
				return d.Avtor != me.ID
			}
			return true
		}
		list, total := a.T.Discussions.Select(store.Opts[model.Discussions]{Index: "user", Key: store.I(me.ID), Where: pred, Desc: true, Offset: p.Offset, Limit: p.Per,
			Less: func(x, y *model.Discussions) bool { return x.Time < y.Time }})
		items := make([]map[string]any, 0, len(list))
		for _, d := range list {
			dt := d.Type
			if dt == "files" {
				dt = "obmen"
			}
			it := map[string]any{"id": d.ID, "type": d.Type, "count": d.Count, "time": d.Time, "author": a.Ref(d.Avtor), "mine": d.Avtor == me.ID}
			if x, ok := a.Describe(dt, d.IDSim); ok {
				it["desc"] = x
			}
			items = append(items, it)
		}
		out := map[string]any{"items": items, "total": total, "page": p.Page, "per": p.Per, "pages": max(1, (total+p.Per-1)/p.Per)}
		out["unread_my"] = a.T.Discussions.Count(func(d *model.Discussions) bool { return d.IDUser == me.ID && d.Count > 0 && d.Avtor == me.ID })
		out["unread_frends"] = a.T.Discussions.Count(func(d *model.Discussions) bool { return d.IDUser == me.ID && d.Count > 0 && d.Avtor != me.ID })
		return c.JSON(out)
	}, core.Auth)

	a.POST("/api/discussions/read", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		a.T.Discussions.UpdateWhere(func(d *model.Discussions) bool { return d.IDUser == me.ID && d.Count > 0 }, func(d *model.Discussions) { d.Count = 0 })
		return c.OK()
	}, core.Auth)
	a.DELETE("/api/discussions", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		a.T.Discussions.DeleteBy("user", store.I(me.ID))
		return c.OK()
	}, core.Auth)
	a.DELETE("/api/discussions/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		d, ok := a.T.Discussions.Get(c.PathInt("id"))
		if !ok || d.IDUser != me.ID {
			return core.ErrNotFound()
		}
		a.T.Discussions.Delete(d.ID)
		return c.OK()
	}, core.Auth)

	a.GET("/api/discussions/settings", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		return c.JSON(flagsOf(a.DiscSettings(me.ID), "disc_"))
	}, core.Auth)
	a.PUT("/api/discussions/settings", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		var in map[string]int64
		if err := c.Bind(&in); err != nil {
			return err
		}
		s := a.DiscSettings(me.ID)
		a.T.DiscSet.Update(s.ID, func(x *model.DiscussionsSet) { applyFlags(x, in, "disc_") })
		return c.OK()
	}, core.Auth)
}

// ---------------------------------------------------------------- уведомления

var notifDescribe = map[string]string{
	"files_komm": "obmen", "obmen_komm": "obmen", "news_komm": "news", "status_komm": "status",
	"foto_komm": "foto", "notes_komm": "notes", "them_komm": "them", "stena_komm2": "stena",
	"shorts_komm": "shorts", "shorts_like": "shorts",
	"group_invite": "group_invite", "group_req": "group", "group_req_ok": "group",
}

func notifLink(a *core.App, me int64, n model.Notification) (string, map[string]any) {
	if dt, ok := notifDescribe[n.Type]; ok {
		if d, ok := a.Describe(dt, n.IDObject); ok {
			m := map[string]any{"title": d.Title, "text": d.Text, "image": d.Image}
			if d.Extra != nil {
				m["extra"] = d.Extra
			}
			return d.Link, m
		}
		return "", nil
	}
	switch n.Type {
	case "stena_komm":
		return "/user/stena/" + store.I(n.IDObject), nil
	case "stena":
		return "/user/stena/" + store.I(me), nil
	case "guest":
		return "/guest", nil
	case "adm_komm":
		return "/chat/admin", nil
	case "new_gift", "ok_gift", "no_gift":
		return "/user/gift/" + store.I(me), nil
	case "money_in":
		return "/user/money", nil
	case "ok_frend", "no_frend", "del_frend", "otm_frend":
		return "/id" + store.I(n.Avtor), nil
	case "user_rating":
		return "/user/info/who_rating/" + store.I(me), nil
	}
	return "", nil
}

func registerNotifications(a *core.App) {
	a.GET("/api/notification", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		p := c.Page()
		list, total := a.T.Notifications.Select(store.Opts[model.Notification]{Index: "user", Key: store.I(me.ID), Desc: true, Offset: p.Offset, Limit: p.Per,
			Less: func(x, y *model.Notification) bool { return x.Time < y.Time }})
		items := make([]map[string]any, 0, len(list))
		for _, n := range list {
			link, desc := notifLink(a, me.ID, n)
			items = append(items, map[string]any{"id": n.ID, "type": n.Type, "time": n.Time, "read": n.Read == 1, "author": a.Ref(n.Avtor), "link": link, "desc": desc, "object": n.IDObject})
			if n.Read == 0 {
				a.T.Notifications.Update(n.ID, func(x *model.Notification) { x.Read = 1 })
			}
		}
		return c.List(items, total, p)
	}, core.Auth)
	a.DELETE("/api/notification", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		a.T.Notifications.DeleteBy("user", store.I(me.ID))
		return c.OK()
	}, core.Auth)
	a.DELETE("/api/notification/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		n, ok := a.T.Notifications.Get(c.PathInt("id"))
		if !ok || n.IDUser != me.ID {
			return core.ErrNotFound()
		}
		a.T.Notifications.Delete(n.ID)
		return c.OK()
	}, core.Auth)
	a.GET("/api/notification/settings", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		return c.JSON(map[string]int64{"komm": a.NotifSettings(me.ID).Komm})
	}, core.Auth)
	a.PUT("/api/notification/settings", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		var in struct {
			Komm int64 `json:"komm"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		s := a.NotifSettings(me.ID)
		a.T.NotifSet.Update(s.ID, func(x *model.NotificationSet) { x.Komm = min(max(in.Komm, 0), 1) })
		return c.OK()
	}, core.Auth)
}
