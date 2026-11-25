package content

import (
	"sort"
	"strings"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/mods/profile"
	"dcms/internal/store"
)

const wordsPerPage = 900

// canView — доступ к записи по приватности.
func canView(a *core.App, me *model.User, n *Note) bool {
	if n.Private == 0 {
		return true
	}
	if me == nil {
		return false
	}
	if me.ID == n.IDUser || a.Can(me.ID, "notes_delete") {
		return true
	}
	return n.Private == 1 && a.IsFriend(me.ID, n.IDUser)
}

func canKomm(a *core.App, me *model.User, n *Note) bool {
	if me == nil {
		return false
	}
	if n.PrivateKomm == 0 || me.ID == n.IDUser || a.Can(me.ID, "notes_delete") {
		return true
	}
	return n.PrivateKomm == 1 && a.IsFriend(me.ID, n.IDUser)
}

func shareBlock(a *core.App, n Note) any {
	if !n.Share {
		return nil
	}
	b := map[string]any{"type": n.ShareType, "id": n.ShareID, "name": n.ShareName, "text": a.Text.Render(n.ShareText), "user": a.Ref(n.ShareIDUser)}
	if n.ShareType == "forum" {
		if d, ok := a.Describe("them", n.ShareID); ok {
			b["link"] = d.Link
		}
	} else {
		b["link"] = "/notes/" + i(n.ShareID)
	}
	return b
}

func noteItem(a *core.App, n Note) map[string]any {
	return map[string]any{
		"id": n.ID, "name": n.Name, "time": n.Time, "user": a.Ref(n.IDUser), "private": n.Private, "count": n.Count,
		"preview": a.Text.Plain(n.Msg, 180), "share": shareBlock(a, n),
		"komm":      noteKomms.CountBy("note", i(n.ID)),
		"bookmarks": a.T.Bookmarks.Count(func(b *model.Bookmarks) bool { return b.Type == "notes" && b.IDObject == n.ID }),
		"shares":    notes.CountBy("share", "notes|"+i(n.ID)),
		"id_dir":    n.IDDir,
	}
}

func listOpts(sortBy string, since int64, where func(*Note) bool) store.Opts[Note] {
	o := store.Opts[Note]{Desc: true}
	o.Where = func(n *Note) bool { return (since == 0 || n.Time > since) && (where == nil || where(n)) }
	o.Less = func(x, y *Note) bool { return x.Time < y.Time }
	if sortBy == "c" {
		o.Less = func(x, y *Note) bool {
			if x.Count != y.Count {
				return x.Count < y.Count
			}
			return x.Time < y.Time
		}
	}
	return o
}

func sinceFor(c *core.Ctx) int64 {
	if c.Q("sort") != "c" {
		return 0
	}
	now := core.Now()
	switch c.Q("new") {
	case "t":
		return now - 86400
	case "m":
		return now - 2592000
	}
	return 0
}

func noteList(a *core.App, c *core.Ctx, o store.Opts[Note]) error {
	p := c.Page()
	o.Offset, o.Limit = p.Offset, p.Per
	list, total := notes.Select(o)
	items := make([]map[string]any, 0, len(list))
	for _, n := range list {
		items = append(items, noteItem(a, n))
	}
	return c.List(items, total, p)
}

func registerNotes(a *core.App) {
	a.Describer("notes", func(id int64) (core.Desc, bool) {
		n, ok := notes.Get(id)
		if !ok {
			return core.Desc{}, false
		}
		return core.Desc{Title: n.Name, Text: a.Text.Plain(n.Msg, 200), Link: "/notes/" + i(n.ID), Owner: n.IDUser}, true
	})
	a.ProfileStat(func(a *core.App, viewer, target *model.User) []core.ProfileItem {
		n := notes.CountBy("user", i(target.ID))
		return []core.ProfileItem{{Key: "notes", Title: "Блоги", Icon: "book", Link: "/notes/user/" + i(target.ID), Count: int64(n)}}
	})
	a.Counter("notes", func(a *core.App, u *model.User) string {
		total := notes.Len()
		since := core.Now() - 86400
		if u != nil && u.DateLast > 0 {
			since = u.DateLast
		}
		n := notes.Count(func(x *Note) bool { return x.Time > since && x.Private == 0 })
		if n > 0 {
			return store.I(int64(total)) + " +" + store.I(int64(n))
		}
		return store.I(int64(total))
	})
	a.HomeBlock("notes", func(a *core.App, u *model.User) any {
		list, _ := notes.Select(store.Opts[Note]{Desc: true, Limit: 3, Where: func(n *Note) bool { return n.Private == 0 }})
		day := core.Now() - 86400
		items := make([]map[string]any, 0, len(list))
		for _, n := range list {
			items = append(items, map[string]any{"id": n.ID, "name": n.Name, "title": n.Name, "author": a.Ref(n.IDUser), "preview": a.Text.Plain(n.Msg, 90), "time": n.Time, "user": a.Ref(n.IDUser), "komm": noteKomms.CountBy("note", i(n.ID))})
		}
		return map[string]any{"total": notes.Len(), "plus": notes.Count(func(n *Note) bool { return n.Time > day && n.Private == 0 }), "items": items}
	})
	a.OnUserDelete(func(uid int64) {
		for _, n := range notes.By("user", i(uid)) {
			deleteNote(n.ID)
		}
		noteKomms.DeleteWhere(func(k *NoteKomm) bool { return k.IDUser == uid })
		noteLikes.DeleteWhere(func(k *NoteLike) bool { return k.IDUser == uid })
		noteViews.DeleteWhere(func(k *NoteView) bool { return k.IDUser == uid })
	})

	// ── ленты
	a.GET("/api/notes", func(c *core.Ctx) error {
		if err := banErr(a, c.User()); err != nil {
			return err
		}
		return noteList(a, c, listOpts(c.Q("sort"), sinceFor(c), func(n *Note) bool { return n.Private == 0 }))
	})
	a.GET("/api/notes-user/{uid}", func(c *core.Ctx) error {
		if err := banErr(a, c.User()); err != nil {
			return err
		}
		uid := c.PathInt("uid")
		ank, ok := a.GetUser(uid)
		if !ok {
			return core.ErrNotFound("Пользователь не найден")
		}
		me := c.User()
		o := listOpts(c.Q("sort"), 0, func(n *Note) bool { return canView(a, me, n) })
		o.Index, o.Key = "user", i(uid)
		p := c.Page()
		o.Offset, o.Limit = p.Offset, p.Per
		list, total := notes.Select(o)
		items := make([]map[string]any, 0, len(list))
		for _, n := range list {
			items = append(items, noteItem(a, n))
		}
		return c.JSON(map[string]any{"items": items, "total": total, "page": p.Page, "per": p.Per, "pages": max(1, (total+p.Per-1)/p.Per), "owner": a.Ref(ank.ID), "mine": me != nil && me.ID == uid})
	})
	a.GET("/api/notes-search", func(c *core.Ctx) error {
		q := strings.ToLower(strings.TrimSpace(c.Q("q")))
		if len([]rune(q)) < 1 {
			return core.ErrBad("Введите строку поиска")
		}
		me := c.User()
		return noteList(a, c, listOpts("t", 0, func(n *Note) bool { return canView(a, me, n) && strings.Contains(strings.ToLower(n.Name), q) }))
	})

	// ── категории
	a.GET("/api/notes-dirs", func(c *core.Ctx) error {
		since := core.Now() - 86400
		if u := c.User(); u != nil && u.DateLast > 0 {
			since = u.DateLast
		}
		ds := noteDirs.All()
		sort.Slice(ds, func(x, y int) bool { return ds[x].ID > ds[y].ID })
		out := make([]map[string]any, 0, len(ds))
		for _, d := range ds {
			out = append(out, map[string]any{"id": d.ID, "name": d.Name, "msg": d.Msg, "msg_html": a.Text.Render(d.Msg),
				"count": notes.CountBy("dir", i(d.ID)), "new": notes.Count(func(n *Note) bool { return n.IDDir == d.ID && n.Time > since })})
		}
		me := c.User()
		return c.JSON(map[string]any{"items": out, "can_create": me != nil && a.Can(me.ID, "notes_edit"), "can_delete": me != nil && a.Can(me.ID, "notes_delete")})
	})
	a.GET("/api/notes-dirs/{id}", func(c *core.Ctx) error {
		d, ok := noteDirs.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Категория не найдена")
		}
		me := c.User()
		o := listOpts(c.Q("sort"), 0, func(n *Note) bool { return n.IDDir == d.ID && canView(a, me, n) })
		o.Index, o.Key = "dir", i(d.ID)
		p := c.Page()
		o.Offset, o.Limit = p.Offset, p.Per
		list, total := notes.Select(o)
		items := make([]map[string]any, 0, len(list))
		for _, n := range list {
			items = append(items, noteItem(a, n))
		}
		return c.JSON(map[string]any{"items": items, "total": total, "page": p.Page, "per": p.Per, "pages": max(1, (total+p.Per-1)/p.Per), "dir": map[string]any{"id": d.ID, "name": d.Name, "msg_html": a.Text.Render(d.Msg)}})
	})
	a.POST("/api/notes-dirs", func(c *core.Ctx) error {
		if err := c.NeedAccess("notes_edit"); err != nil {
			return err
		}
		var in struct{ Title, Name, Msg string }
		if err := c.Bind(&in); err != nil {
			return err
		}
		name := strings.TrimSpace(in.Title + in.Name)
		msg := a.Text.Clean(strings.TrimSpace(in.Msg))
		if err := lenOK(name, 3, 32, "Название"); err != nil {
			return err
		}
		if err := lenOK(msg, 2, 10024, "Содержание"); err != nil {
			return err
		}
		id := noteDirs.Insert(&NoteDir{Name: name, Msg: msg})
		return c.OK("id", id)
	}, core.Auth)
	a.DELETE("/api/notes-dirs/{id}", func(c *core.Ctx) error {
		if err := c.NeedAccess("notes_delete"); err != nil {
			return err
		}
		d, ok := noteDirs.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Категория не найдена")
		}
		for _, n := range notes.By("dir", i(d.ID)) {
			deleteNote(n.ID)
		}
		noteDirs.Delete(d.ID)
		a.AdminLog(c.UID(), "Блоги", "Удаление категории", d.Name)
		return c.OK()
	}, core.Auth)

	// ── создание
	a.POST("/api/notes", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if err := banErr(a, me); err != nil {
			return err
		}
		var in struct {
			Title       string `json:"title"`
			Msg         string `json:"msg"`
			IDDir       int64  `json:"id_dir"`
			Private     int64  `json:"private"`
			PrivateKomm int64  `json:"private_komm"`
			Captcha     string `json:"captcha"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if me.Rating < 2 || me.GroupAccess < 6 {
			if !core.CaptchaCheck(c, in.Captcha) {
				return core.ErrBad("Пройдите проверку, что вы не робот")
			}
		}
		msg := a.Text.Clean(strings.TrimSpace(in.Msg))
		title := strings.TrimSpace(in.Title)
		if title == "" {
			title = a.Text.Plain(msg, 24) + " ..."
		}
		if err := lenOK(title, 1, 32, "Название"); err != nil {
			return err
		}
		if err := lenOK(msg, 2, 30000, "Содержание"); err != nil {
			return err
		}
		if in.IDDir != 0 && !noteDirs.Has(in.IDDir) {
			in.IDDir = 0
		}
		n := Note{Time: core.Now(), Msg: msg, Name: title, IDUser: me.ID, Private: clamp3(in.Private), PrivateKomm: clamp3(in.PrivateKomm), IDDir: in.IDDir}
		id := notes.Insert(&n)
		if n.Private != 2 {
			profile.PostWall(me.ID, "новый блог: [url=/notes/"+i(id)+"]"+strings.NewReplacer("[", "(", "]", ")").Replace(title)+"[/url]")
			a.FanTape(me.ID, core.KNotes, "notes", id, core.TapeOpts{})
		}
		return c.OK("id", id)
	}, core.Auth)

	// ── просмотр
	a.GET("/api/notes/{id}", func(c *core.Ctx) error {
		me := c.User()
		if err := banErr(a, me); err != nil {
			return err
		}
		n, ok := notes.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Запись не найдена")
		}
		if !canView(a, me, &n) {
			if n.Private == 1 {
				return core.ErrForbidden("Блог доступен только для друзей")
			}
			return core.ErrForbidden("Пользователь запретил просмотр блога")
		}
		if me != nil {
			if _, seen := noteViews.OneBy("un", store.Key(me.ID, n.ID)); !seen {
				noteViews.Insert(&NoteView{IDNotes: n.ID, IDUser: me.ID})
				n, _ = notes.Update(n.ID, func(x *Note) { x.Count++ })
			}
			a.T.Notifications.UpdateWhere(func(x *model.Notification) bool {
				return x.Type == "notes_komm" && x.IDUser == me.ID && x.IDObject == n.ID && x.Read == 0
			}, func(x *model.Notification) { x.Read = 1 })
			a.T.Discussions.UpdateWhere(func(x *model.Discussions) bool { return x.IDUser == me.ID && x.Type == "notes" && x.IDSim == n.ID }, func(x *model.Discussions) { x.Count = 0 })
			if s := c.Q("sort"); s == "0" || s == "1" {
				v := int64(0)
				if s == "1" {
					v = 1
				}
				a.T.Users.Update(me.ID, func(u *model.User) { u.Sort = v })
				me.Sort = v
			}
		}
		// текст постранично по словам, как в PHP
		words := strings.Split(n.Msg, " ")
		pages := max(1, (len(words)+wordsPerPage-1)/wordsPerPage)
		tp := min(max(int(c.QInt("tp")), 1), pages)
		body := n.Msg
		if pages > 1 {
			body = strings.Join(words[(tp-1)*wordsPerPage:min(tp*wordsPerPage, len(words))], " ")
		}
		var likes, dislikes int
		var myLike = -1
		for _, l := range noteLikes.By("note", i(n.ID)) {
			if l.Like == 1 {
				likes++
			} else {
				dislikes++
			}
			if me != nil && l.IDUser == me.ID {
				myLike = int(l.Like)
			}
		}
		mine := me != nil && me.ID == n.IDUser
		out := map[string]any{"id": n.ID, "name": n.Name, "msg": n.Msg, "msg_html": a.Text.Render(body), "time": n.Time, "user": a.Ref(n.IDUser), "private": n.Private, "private_komm": n.PrivateKomm,
			"id_dir": n.IDDir, "count": n.Count, "share": shareBlock(a, n), "text_page": tp, "text_pages": pages, "shares": notes.CountBy("share", "notes|"+i(n.ID)),
			"likes": likes, "dislikes": dislikes, "rating": likes - dislikes, "my_like": myLike, "mine": mine,
			"can_edit": mine || (me != nil && a.Can(me.ID, "notes_edit")), "can_delete": mine || (me != nil && a.Can(me.ID, "notes_delete")),
			"can_komm": canKomm(a, me, &n), "bookmarks": a.T.Bookmarks.Count(func(b *model.Bookmarks) bool { return b.Type == "notes" && b.IDObject == n.ID })}
		if n.IDDir != 0 {
			if d, ok := noteDirs.Get(n.IDDir); ok {
				out["dir"] = map[string]any{"id": d.ID, "name": d.Name}
			}
		}
		if me != nil {
			_, fav := a.T.Bookmarks.OneBy("uobj", store.Key(me.ID, "notes", n.ID))
			out["bookmarked"] = fav
			out["can_share"] = !mine && notes.Count(func(x *Note) bool { return x.IDUser == me.ID && x.Share && x.ShareType == "notes" && x.ShareID == n.ID }) == 0
			out["can_like"] = !mine && myLike < 0
		}
		p := c.Page()
		desc := !(me != nil && me.Sort == 1)
		list, total := noteKomms.Select(store.Opts[NoteKomm]{Index: "note", Key: i(n.ID), Desc: desc, Offset: p.Offset, Limit: p.Per})
		ks := make([]map[string]any, 0, len(list))
		for _, k := range list {
			m := map[string]any{"id": k.ID, "msg": a.Text.Render(k.Msg), "time": k.Time, "user": a.Ref(k.IDUser)}
			if me != nil {
				m["can_reply"] = k.IDUser != me.ID
				m["can_spam"] = k.IDUser != me.ID
				m["can_delete"] = mine || a.Can(me.ID, "notes_delete")
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

	// ── редактирование/удаление
	a.PUT("/api/notes/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if err := banErr(a, me); err != nil {
			return err
		}
		n, ok := notes.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Запись не найдена")
		}
		if me.ID != n.IDUser && !a.Can(me.ID, "notes_edit") {
			return core.ErrForbidden()
		}
		var in struct {
			Name        string `json:"name"`
			Title       string `json:"title"`
			Msg         string `json:"msg"`
			IDDir       int64  `json:"id_dir"`
			Private     int64  `json:"private"`
			PrivateKomm int64  `json:"private_komm"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		name := strings.TrimSpace(in.Name + in.Title)
		msg := a.Text.Clean(strings.TrimSpace(in.Msg))
		if name == "" {
			name = a.Text.Plain(msg, 24)
		}
		if err := lenOK(name, 1, 50, "Название"); err != nil {
			return err
		}
		if err := lenOK(msg, 3, 10000, "Текст"); err != nil {
			return err
		}
		if in.IDDir != 0 && !noteDirs.Has(in.IDDir) {
			in.IDDir = 0
		}
		notes.Update(n.ID, func(x *Note) {
			x.Name, x.Msg, x.IDDir, x.Private, x.PrivateKomm = name, msg, in.IDDir, clamp3(in.Private), clamp3(in.PrivateKomm)
		})
		return c.OK()
	}, core.Auth)
	a.DELETE("/api/notes/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		n, ok := notes.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Запись не найдена")
		}
		if me.ID != n.IDUser && !a.Can(me.ID, "notes_delete") {
			return core.ErrForbidden()
		}
		deleteNote(n.ID)
		if me.ID != n.IDUser {
			a.AdminLog(me.ID, "Блоги", "Удаление записи", n.Name)
		}
		return c.OK()
	}, core.Auth)

	// ── голосование
	a.POST("/api/notes/{id}/like", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if err := banErr(a, me); err != nil {
			return err
		}
		n, ok := notes.Get(c.PathInt("id"))
		if !ok || !canView(a, me, &n) {
			return core.ErrNotFound("Запись не найдена")
		}
		if me.ID == n.IDUser {
			return core.ErrBad("Нельзя голосовать за свою запись")
		}
		var in struct {
			Like bool `json:"like"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if _, done := noteLikes.OneBy("un", store.Key(me.ID, n.ID)); done {
			return core.ErrBad("Вы уже голосовали")
		}
		l := int64(0)
		d := int64(-1)
		if in.Like {
			l, d = 1, 1
		}
		noteLikes.Insert(&NoteLike{IDNotes: n.ID, IDUser: me.ID, Like: l})
		notes.Update(n.ID, func(x *Note) { x.Count += d })
		return c.OK()
	}, core.Auth)

	// ── репост
	a.POST("/api/notes/{id}/share", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if err := banErr(a, me); err != nil {
			return err
		}
		n, ok := notes.Get(c.PathInt("id"))
		if !ok || !canView(a, me, &n) {
			return core.ErrNotFound("Такой записи не существует")
		}
		if n.IDUser == me.ID {
			return core.ErrBad("Нельзя репостить свои записи")
		}
		if notes.Count(func(x *Note) bool { return x.IDUser == me.ID && x.Share && x.ShareType == "notes" && x.ShareID == n.ID }) > 0 {
			return core.ErrBad("Вы уже поделились данной записью")
		}
		var in struct {
			Text string `json:"text"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		nn := Note{IDUser: me.ID, Name: n.Name, Msg: a.Text.Clean(strings.TrimSpace(in.Text)), Share: true, ShareText: n.Msg, ShareID: n.ID, ShareIDUser: n.IDUser, ShareName: n.Name, ShareType: "notes", Time: core.Now()}
		return c.OK("id", notes.Insert(&nn))
	}, core.Auth)
	a.POST("/api/notes-forum-share/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if b, ok := a.ActiveBan(me.ID, "forum"); ok {
			_ = b
			return core.ErrForbidden("Вам запрещён доступ к форуму")
		}
		d, ok := a.Describe("them", c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Такой темы не существует")
		}
		id := c.PathInt("id")
		if notes.Count(func(x *Note) bool { return x.IDUser == me.ID && x.Share && x.ShareType == "forum" && x.ShareID == id }) > 0 {
			return core.ErrBad("Вы уже поделились данной темой")
		}
		var in struct {
			Text string `json:"text"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		nn := Note{IDUser: me.ID, Name: d.Title, Msg: a.Text.Clean(strings.TrimSpace(in.Text)), Share: true, ShareText: d.Text, ShareID: id, ShareIDUser: d.Owner, ShareName: d.Title, ShareType: "forum", Time: core.Now()}
		return c.OK("id", notes.Insert(&nn))
	}, core.Auth)
	a.GET("/api/notes-forum-share/{id}", func(c *core.Ctx) error {
		d, ok := a.Describe("them", c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Такой темы не существует")
		}
		id := c.PathInt("id")
		me := c.User()
		done := me != nil && notes.Count(func(x *Note) bool { return x.IDUser == me.ID && x.Share && x.ShareType == "forum" && x.ShareID == id }) > 0
		return c.JSON(map[string]any{"title": d.Title, "link": d.Link, "user": a.Ref(d.Owner), "done": done})
	}, core.Auth)

	// ── закладки блога (fav.php)
	a.GET("/api/notes/{id}/bookmarks", func(c *core.Ctx) error {
		id := c.PathInt("id")
		if !notes.Has(id) {
			return core.ErrNotFound("Блог не найден")
		}
		p := c.Page()
		list, total := a.T.Bookmarks.Select(store.Opts[model.Bookmarks]{Where: func(b *model.Bookmarks) bool { return b.Type == "notes" && b.IDObject == id }, Offset: p.Offset, Limit: p.Per})
		items := make([]map[string]any, 0, len(list))
		for _, b := range list {
			items = append(items, map[string]any{"user": a.Ref(b.IDUser), "time": b.Time})
		}
		return c.List(items, total, p)
	})

	// ── комментарии
	a.POST("/api/notes/{id}/komm", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if err := banErr(a, me); err != nil {
			return err
		}
		n, ok := notes.Get(c.PathInt("id"))
		if !ok || !canView(a, me, &n) {
			return core.ErrNotFound("Запись не найдена")
		}
		if !canKomm(a, me, &n) {
			if n.PrivateKomm == 1 {
				return core.ErrForbidden("Комментировать могут только друзья автора")
			}
			return core.ErrForbidden("Автор запретил комментирование")
		}
		var in struct {
			Msg     string `json:"msg"`
			ReplyTo int64  `json:"reply_to"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		msg := a.Text.Clean(strings.TrimSpace(in.Msg))
		if err := lenOK(msg, 2, 1024, "Сообщение"); err != nil {
			return err
		}
		if err := noMat(a, msg, "тексте сообщения"); err != nil {
			return err
		}
		if noteKomms.Count(func(k *NoteKomm) bool { return k.IDNotes == n.ID && k.IDUser == me.ID && k.Msg == msg }) > 0 {
			return core.ErrBad("Ваше сообщение повторяет предыдущее")
		}
		if in.ReplyTo != 0 {
			a.Notify(me.ID, in.ReplyTo, "notes_komm", n.ID, true)
		}
		a.Discuss(n.IDUser, me.ID, core.KNotes, "notes", n.ID)
		a.DiscussFor(me.ID, n.IDUser, "notes", n.ID)
		id := noteKomms.Insert(&NoteKomm{IDUser: me.ID, IDNotes: n.ID, Msg: msg, Time: core.Now()})
		a.AddBalls(me.ID, 1, 0)
		return c.OK("id", id)
	}, core.Auth)
	a.DELETE("/api/notes-komm/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		k, ok := noteKomms.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Запись не найдена")
		}
		n, _ := notes.Get(k.IDNotes)
		if me.ID != n.IDUser && !a.Can(me.ID, "notes_delete") {
			return core.ErrForbidden()
		}
		noteKomms.Delete(k.ID)
		return c.OK()
	}, core.Auth)
	a.POST("/api/notes-komm/{id}/spam", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		k, ok := noteKomms.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		if k.IDUser == me.ID {
			return core.ErrBad("Нельзя жаловаться на себя")
		}
		var in struct {
			Msg   string `json:"msg"`
			Types int64  `json:"types"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if err := lenOK(strings.TrimSpace(in.Msg), 3, 1512, "Описание жалобы"); err != nil {
			return err
		}
		if a.T.Spam.Count(func(s *model.Spamus) bool {
			return s.IDUser == me.ID && s.IDSpam == k.IDUser && s.Razdel == "notes_komm" && s.Spam == k.Msg
		}) > 0 {
			return core.ErrBad("Жалоба уже отправлена и будет рассмотрена в ближайшее время")
		}
		a.ReportSpam(me.ID, k.IDUser, "notes_komm", k.ID, k.IDNotes, k.Msg, strings.TrimSpace(in.Msg))
		return c.OK()
	}, core.Auth)
}

func clamp3(v int64) int64 {
	if v < 0 || v > 2 {
		return 0
	}
	return v
}

func deleteNote(id int64) {
	notes.Delete(id)
	noteViews.DeleteWhere(func(k *NoteView) bool { return k.IDNotes == id })
	noteKomms.DeleteBy("note", i(id))
	noteLikes.DeleteBy("note", i(id))
}
