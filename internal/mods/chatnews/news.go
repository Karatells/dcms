package chatnews

import (
	"net/url"
	"strings"
	"unicode/utf8"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

type News struct {
	store.Base
	Msg      string `json:"msg"`
	Time     int64  `json:"time"`
	IDUser   int64  `json:"id_user"`
	Title    string `json:"title"`
	MainTime int64  `json:"main_time"` // до какого времени показывать на главной
	Link     string `json:"link"`
}
type NewsKomm struct {
	store.Base
	IDUser int64  `json:"id_user"`
	Msg    string `json:"msg"`
	Time   int64  `json:"time"`
	IDNews int64  `json:"id_news"`
}

var (
	newsT  *store.Table[News, *News]
	kommsT *store.Table[NewsKomm, *NewsKomm]
)

func newsCard(a *core.App, n News, full bool) map[string]any {
	m := map[string]any{"id": n.ID, "title": n.Title, "time": n.Time, "author": a.Ref(n.IDUser), "link": n.Link, "komm": kommsT.CountBy("news", i(n.ID)),
		"main": n.MainTime > core.Now()}
	m["msg"] = a.Text.Render(n.Msg)
	return m
}

func normLink(l string) string {
	l = strings.TrimSpace(l)
	if l != "" && !strings.HasPrefix(l, "http://") && !strings.HasPrefix(l, "https://") && !strings.HasPrefix(l, "/") {
		l = "/" + l
	}
	return l
}

func registerNews(a *core.App) {
	newsT = store.NewTable[News](a.DB, "news")
	kommsT = store.NewTable[NewsKomm](a.DB, "news_komm").
		Index("news", func(k *NewsKomm) string { return i(k.IDNews) }).
		Index("user", func(k *NewsKomm) string { return i(k.IDUser) })

	a.Describer("news", func(id int64) (core.Desc, bool) {
		n, ok := newsT.Get(id)
		if !ok {
			return core.Desc{}, false
		}
		return core.Desc{Title: n.Title, Text: a.Text.Plain(n.Msg, 160), Link: "/news/" + i(n.ID), Owner: n.IDUser}, true
	})
	a.OnUserDelete(func(uid int64) {
		kommsT.DeleteBy("user", i(uid))
		a.T.Likes.DeleteWhere(func(l *model.LikeObject) bool { return l.IDUser == uid && l.Type == "news" })
	})
	a.Counter("news", func(a *core.App, u *model.User) string {
		total := newsT.Len()
		n := 0
		if u != nil {
			n = newsT.Count(func(x *News) bool { return x.Time > u.DateLast })
		}
		if n > 0 {
			return itoa(total) + " +" + itoa(n)
		}
		return itoa(total)
	})

	a.GET("/api/news", func(c *core.Ctx) error {
		p := c.Page()
		list, total := newsT.Select(store.Opts[News]{Desc: true, Offset: p.Offset, Limit: p.Per})
		items := make([]map[string]any, 0, len(list))
		for _, n := range list {
			items = append(items, newsCard(a, n, false))
		}
		return c.JSON(map[string]any{"items": items, "total": total, "page": p.Page, "per": p.Per, "pages": max(1, (total+p.Per-1)/p.Per), "can_add": c.Can("adm_news")})
	})

	a.GET("/api/news/{id}", func(c *core.Ctx) error {
		n, ok := newsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Новость не найдена")
		}
		me := c.User()
		if me != nil {
			a.T.Notifications.UpdateWhere(func(x *model.Notification) bool {
				return x.IDUser == me.ID && x.Type == "news_komm" && x.IDObject == n.ID && x.Read == 0
			}, func(x *model.Notification) { x.Read = 1 })
			if s := c.Q("sort"); s == "0" || s == "1" {
				v := int64(0)
				if s == "1" {
					v = 1
				}
				a.T.Users.Update(me.ID, func(u *model.User) { u.Sort = v })
				me.Sort = v
			}
		}
		out := newsCard(a, n, true)
		// лайки
		likes, dislikes, my := 0, 0, -1
		for _, l := range a.T.Likes.By("obj", store.Key("news", n.ID)) {
			if l.Like {
				likes++
			} else {
				dislikes++
			}
			if me != nil && l.IDUser == me.ID {
				my = 0
				if l.Like {
					my = 1
				}
			}
		}
		out["likes"], out["dislikes"], out["my_like"] = likes, dislikes, my
		out["can_edit"] = c.Can("adm_news")
		out["raw"] = n.Msg
		out["main_time"] = n.MainTime
		// соседние новости
		if prev, _ := newsT.Select(store.Opts[News]{Desc: true, Limit: 1, Where: func(x *News) bool { return x.ID < n.ID }}); len(prev) > 0 {
			out["older"] = prev[0].ID
		}
		if nxt, _ := newsT.Select(store.Opts[News]{Limit: 1, Where: func(x *News) bool { return x.ID > n.ID }}); len(nxt) > 0 {
			out["newer"] = nxt[0].ID
		}
		out["pos"] = newsT.Count(func(x *News) bool { return x.ID > n.ID }) + 1
		out["count"] = newsT.Len()
		// комментарии
		p := c.Page()
		desc := !(me != nil && me.Sort == 1) // sort=1 «Внизу» — новые комментарии внизу; 0 «Вверху»
		list, total := kommsT.Select(store.Opts[NewsKomm]{Index: "news", Key: i(n.ID), Desc: desc, Offset: p.Offset, Limit: p.Per})
		ks := make([]map[string]any, 0, len(list))
		for _, k := range list {
			m := postView(a, k.ID, k.Msg, k.Time, k.IDUser)
			if me != nil {
				m["can_reply"] = k.IDUser != me.ID
				m["can_delete"] = me.Level > a.Ref(k.IDUser).Level
			}
			ks = append(ks, m)
		}
		out["komms"] = map[string]any{"items": ks, "total": total, "page": p.Page, "per": p.Per, "pages": max(1, (total+p.Per-1)/p.Per)}
		out["sort"] = 0
		if me != nil {
			out["sort"] = me.Sort
		}
		return c.JSON(out)
	})

	type newsIn struct {
		Title string `json:"title"`
		Msg   string `json:"msg"`
		Link  string `json:"link"`
		Days  int64  `json:"days"` // сколько показывать на главной
		Unit  int64  `json:"unit"` // 0 не показывать, 1 дней, 7 недель, 31 месяцев
		Mail  bool   `json:"mail"` // рассылка на e-mail
	}
	check := func(in *newsIn) error {
		in.Title, in.Msg, in.Link = strings.TrimSpace(in.Title), strings.TrimSpace(in.Msg), normLink(in.Link)
		if n := utf8.RuneCountInString(in.Title); n < 3 {
			return core.ErrBad("Короткий заголовок")
		} else if n > 50 {
			return core.ErrBad("Слишком большой заголовок новости")
		}
		if n := utf8.RuneCountInString(in.Msg); n < 2 {
			return core.ErrBad("Новость слишком короткая")
		} else if n > 10024 {
			return core.ErrBad("Содержимое новости слишком большое")
		}
		if err := noMat(a, in.Title, "заголовке новости"); err != nil {
			return err
		}
		return noMat(a, in.Msg, "содержании")
	}
	mainTime := func(in newsIn) int64 {
		t := core.Now() + in.Days*in.Unit*86400
		if t <= core.Now() {
			return 0
		}
		return t
	}

	a.POST("/api/news", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if err := c.NeedAccess("adm_news"); err != nil {
			return err
		}
		var in newsIn
		if err := c.Bind(&in); err != nil {
			return err
		}
		if err := check(&in); err != nil {
			return err
		}
		id := newsT.Insert(&News{IDUser: me.ID, Time: core.Now(), Msg: in.Msg, Title: in.Title, MainTime: mainTime(in), Link: in.Link})
		a.T.Users.UpdateWhere(func(u *model.User) bool { return u.NewsRead != 0 }, func(u *model.User) { u.NewsRead = 0 })
		if in.Mail {
			body := "<h3>" + a.Text.Clean(in.Title) + "</h3>" + a.Text.Render(in.Msg)
			for _, u := range a.T.Users.Find(func(u *model.User) bool { return u.SetNewsToMail && u.Email != "" }) {
				a.SendMail(u.Email, "Новости", body)
			}
		}
		a.AdminLog(me.ID, "Новости", "Добавление", "Добавлена новость '"+in.Title+"'")
		return c.OK("id", id)
	}, core.Auth)

	a.PUT("/api/news/{id}", func(c *core.Ctx) error {
		if err := c.NeedAccess("adm_news"); err != nil {
			return err
		}
		n, ok := newsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		var in newsIn
		if err := c.Bind(&in); err != nil {
			return err
		}
		if err := check(&in); err != nil {
			return err
		}
		newsT.Update(n.ID, func(x *News) {
			x.Title, x.Msg, x.Link, x.MainTime, x.Time = in.Title, in.Msg, in.Link, mainTime(in), core.Now()
		})
		a.T.Users.UpdateWhere(func(u *model.User) bool { return u.NewsRead != 0 }, func(u *model.User) { u.NewsRead = 0 })
		a.AdminLog(c.UID(), "Новости", "Изменение", "Изменена новость '"+in.Title+"'")
		return c.OK()
	}, core.Auth)

	a.DELETE("/api/news/{id}", func(c *core.Ctx) error {
		if err := c.NeedAccess("adm_news"); err != nil {
			return err
		}
		n, ok := newsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		newsT.Delete(n.ID)
		kommsT.DeleteBy("news", i(n.ID))
		a.T.Likes.DeleteWhere(func(l *model.LikeObject) bool { return l.Type == "news" && l.IDObject == n.ID })
		a.T.Notifications.DeleteWhere(func(x *model.Notification) bool { return x.Type == "news_komm" && x.IDObject == n.ID })
		a.AdminLog(c.UID(), "Новости", "Удаление", "Удалена новость '"+n.Title+"'")
		return c.OK()
	}, core.Auth)

	a.POST("/api/news/{id}/like", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		n, ok := newsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		var in struct {
			Like bool `json:"like"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if _, ok := a.T.Likes.OneBy("uobj", store.Key(me.ID, "news", n.ID)); ok {
			return core.ErrBad("Вы уже голосовали")
		}
		a.T.Likes.Insert(&model.LikeObject{IDUser: me.ID, IDObject: n.ID, Type: "news", Like: in.Like})
		a.AddBalls(me.ID, 1, 0)
		return c.OK()
	}, core.Auth)

	a.POST("/api/news/{id}/komm", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		n, ok := newsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		var in struct {
			Msg     string `json:"msg"`
			ReplyTo int64  `json:"reply_to"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		msg := strings.TrimSpace(in.Msg)
		if err := noMat(a, msg, "тексте сообщения"); err != nil {
			return err
		}
		if err := lenOK(msg, 2, 1024, "Сообщение"); err != nil {
			return err
		}
		if kommsT.Count(func(k *NewsKomm) bool { return k.IDNews == n.ID && k.IDUser == me.ID && k.Msg == msg }) > 0 {
			return core.ErrBad("Ваше сообщение повторяет предыдущее")
		}
		id := kommsT.Insert(&NewsKomm{IDUser: me.ID, Time: core.Now(), Msg: msg, IDNews: n.ID})
		a.AddBalls(me.ID, 1, 0)
		if in.ReplyTo != 0 {
			a.Notify(me.ID, in.ReplyTo, "news_komm", n.ID, true)
		}
		return c.OK("id", id)
	}, core.Auth)

	a.DELETE("/api/news-komm/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		k, ok := kommsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		if me.Level <= a.Ref(k.IDUser).Level {
			return core.ErrForbidden("Недостаточно прав")
		}
		kommsT.Delete(k.ID)
		return c.OK()
	}, core.Auth)

	a.POST("/api/news-read", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		a.T.Users.Update(me.ID, func(u *model.User) { u.NewsRead = 1 })
		return c.OK()
	}, core.Auth)
}

var _ = url.QueryEscape
