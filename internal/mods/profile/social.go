package profile

import (
	"strings"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

// checkText проверяет длину, мат и повтор.
func checkText(a *core.App, msg string, min, max int) (string, error) {
	msg = strings.TrimSpace(msg)
	if strLen(msg) > max {
		return "", core.ErrBad("Сообщение слишком длинное")
	}
	if strLen(msg) < min {
		return "", core.ErrBad("Короткое сообщение")
	}
	return a.Text.Clean(msg), nil
}

func notBanned(a *core.App, u *model.User) error {
	if _, ok := a.ActiveBan(u.ID, "all"); ok {
		return core.ErrForbidden("Вы заблокированы")
	}
	return nil
}

func stenaView(a *core.App, p Stena, viewer *model.User) map[string]any {
	likes := stenaLike.By("post", store.I(p.ID))
	m := map[string]any{"id": p.ID, "user": a.Ref(p.IDUser), "time": p.Time, "msg": a.Text.Render(p.Msg), "raw": p.Msg, "likes": len(likes)}
	if viewer != nil {
		for _, l := range likes {
			if l.IDUser == viewer.ID {
				m["liked"] = true
			}
		}
	}
	return m
}

func registerSocial(a *core.App) {
	// ================= стена =================
	a.GET("/api/user/{id}/stena", func(c *core.Ctx) error {
		u, err := userByPath(c)
		if err != nil {
			return err
		}
		if st := access(a, c.User(), &u); st != vOK {
			return core.ErrForbidden("Стена недоступна")
		}
		p := c.Page()
		desc := true
		if c.Q("order") == "asc" {
			desc = false
		}
		list, total := stena.Select(store.Opts[Stena]{Index: "wall", Key: store.I(u.ID), Desc: desc, Offset: p.Offset, Limit: p.Per})
		items := make([]map[string]any, 0, len(list))
		for _, s := range list {
			items = append(items, stenaView(a, s, c.User()))
		}
		return c.List(items, total, p)
	})

	a.POST("/api/user/{id}/stena", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if err := notBanned(a, me); err != nil {
			return err
		}
		u, err := userByPath(c)
		if err != nil {
			return err
		}
		if access(a, me, &u) != vOK || !a.CanAccessPrivacy(me, u.ID, a.UserSettings(u.ID).PrivatWall) {
			return core.ErrForbidden("Пользователь ограничил возможность писать на стене")
		}
		var in struct {
			Msg     string `json:"msg"`
			ReplyTo int64  `json:"reply_to"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		msg, err := checkText(a, in.Msg, 2, 1024)
		if err != nil {
			return err
		}
		for _, s := range stena.By("wall", store.I(u.ID)) {
			if s.IDUser == me.ID && s.Msg == msg {
				return core.ErrBad("Ваше сообщение повторяет предыдущее")
			}
		}
		if in.ReplyTo > 0 && in.ReplyTo != me.ID && a.T.Users.Has(in.ReplyTo) {
			a.Notify(me.ID, in.ReplyTo, "stena_komm", u.ID, true)
		}
		id := stena.Insert(&Stena{IDUser: me.ID, IDStena: u.ID, Time: core.Now(), Msg: msg})
		a.AddBalls(me.ID, 1, 1)
		if u.ID != me.ID {
			a.Notify(me.ID, u.ID, "stena", u.ID, true)
		}
		p, _ := stena.Get(id)
		return c.JSON(stenaView(a, p, me))
	}, core.Auth)

	a.DELETE("/api/stena/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		p, ok := stena.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		if !(c.Can("guest_delete") || p.IDStena == me.ID) {
			return core.ErrForbidden()
		}
		stena.Delete(p.ID)
		stenaLike.DeleteBy("post", store.I(p.ID))
		return c.OK()
	}, core.Auth)

	a.POST("/api/stena/{id}/like", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		p, ok := stena.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		for _, l := range stenaLike.By("post", store.I(p.ID)) {
			if l.IDUser == me.ID {
				return c.OK("liked", true)
			}
		}
		stenaLike.Insert(&StenaLike{IDUser: me.ID, IDStena: p.ID})
		if p.IDUser != me.ID {
			a.AddBalls(p.IDUser, 1, 0)
		}
		return c.OK("liked", true, "likes", stenaLike.CountBy("post", store.I(p.ID)))
	}, core.Auth)

	a.POST("/api/stena/{id}/spam", func(c *core.Ctx) error {
		return reportSpam(c, a, "stena")
	}, core.Auth)

	// ================= статусы =================
	a.POST("/api/status", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if err := notBanned(a, me); err != nil {
			return err
		}
		var in struct {
			Msg string `json:"msg"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if strings.TrimSpace(in.Msg) == "" { // пустой — снять статус
			for _, s := range statuses.By("user", store.I(me.ID)) {
				if s.Pokaz {
					statuses.Update(s.ID, func(x *Status) { x.Pokaz = false })
				}
			}
			return c.OK()
		}
		msg, err := checkText(a, in.Msg, 2, 512)
		if err != nil {
			return err
		}
		for _, s := range statuses.By("user", store.I(me.ID)) {
			if s.Msg == msg {
				return core.ErrBad("Ваше сообщение повторяет предыдущее")
			}
		}
		for _, s := range statuses.By("user", store.I(me.ID)) {
			if s.Pokaz {
				statuses.Update(s.ID, func(x *Status) { x.Pokaz = false })
			}
		}
		id := statuses.Insert(&Status{IDUser: me.ID, Time: core.Now(), Msg: msg, Pokaz: true})
		a.FanTape(me.ID, core.KStatus, "status", id, core.TapeOpts{OtKogo: me.ID})
		return c.OK("id", id)
	}, core.Auth)

	a.GET("/api/status/{id}", func(c *core.Ctx) error {
		s, ok := statuses.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Статус не найден")
		}
		owner, _ := a.GetUser(s.IDUser)
		if access(a, c.User(), &owner) != vOK {
			return core.ErrForbidden("Страница пользователя закрыта")
		}
		me := c.User()
		out := map[string]any{"id": s.ID, "user": a.Ref(s.IDUser), "time": s.Time, "msg": a.Text.Render(s.Msg), "current": s.Pokaz}
		ls := stLikes.By("st", store.I(s.ID))
		plus, minus := 0, 0
		for _, l := range ls {
			if l.Like == 1 {
				plus++
			} else {
				minus++
			}
			if me != nil && l.IDUser == me.ID {
				out["my_like"] = l.Like + 1
			}
		}
		out["plus"], out["minus"] = plus, minus
		if me != nil {
			// сбросить счётчики обсуждения/уведомлений
			for _, d := range a.T.Discussions.By("user", store.I(me.ID)) {
				if d.Type == "status" && d.IDSim == s.ID && d.Count > 0 {
					a.T.Discussions.Update(d.ID, func(x *model.Discussions) { x.Count = 0 })
				}
			}
			a.T.Notifications.UpdateWhere(func(n *model.Notification) bool {
				return n.IDUser == me.ID && n.Type == "status_komm" && n.IDObject == s.ID && n.Read == 0
			}, func(n *model.Notification) { n.Read = 1 })
		}
		p := c.Page()
		list, total := stKomm.Select(store.Opts[StatusKomm]{Index: "st", Key: store.I(s.ID), Desc: true, Offset: p.Offset, Limit: p.Per})
		kk := make([]map[string]any, 0, len(list))
		for _, k := range list {
			kk = append(kk, map[string]any{"id": k.ID, "user": a.Ref(k.IDUser), "time": k.Time, "msg": a.Text.Render(k.Msg)})
		}
		out["komm"] = map[string]any{"items": kk, "total": total, "page": p.Page, "per": p.Per, "pages": max(1, (total+p.Per-1)/p.Per)}
		out["can_delete"] = me != nil && (me.ID == s.IDUser || me.Level > owner.Level)
		return c.JSON(out)
	})

	a.POST("/api/status/{id}/like", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		s, ok := statuses.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		if s.IDUser == me.ID {
			return core.ErrBad("Нельзя оценить свой статус")
		}
		var in struct {
			Like int64 `json:"like"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		for _, l := range stLikes.By("st", store.I(s.ID)) {
			if l.IDUser == me.ID {
				return core.ErrBad("Вы уже оценили этот статус")
			}
		}
		like := int64(0)
		if in.Like == 1 {
			like = 1
		}
		stLikes.Insert(&StatusLike{IDUser: me.ID, IDStatus: s.ID, Time: core.Now(), Like: like})
		a.AddBalls(s.IDUser, 3, 3)
		a.FanTape(me.ID, core.KStatusLike, "status_like", s.ID, core.TapeOpts{OtKogo: me.ID})
		return c.OK()
	}, core.Auth)

	a.GET("/api/status/{id}/likes", func(c *core.Ctx) error {
		s, ok := statuses.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		p := c.Page()
		list, total := stLikes.Select(store.Opts[StatusLike]{Index: "st", Key: store.I(s.ID), Desc: true, Offset: p.Offset, Limit: p.Per})
		items := make([]map[string]any, 0, len(list))
		for _, l := range list {
			it := map[string]any{"user": a.Ref(l.IDUser), "time": l.Time, "like": l.Like}
			if cur := statusOf(l.IDUser); cur != nil {
				it["status"] = map[string]any{"id": cur.ID, "msg": a.Text.Render(cur.Msg)}
			}
			items = append(items, it)
		}
		return c.List(items, total, p)
	})

	a.DELETE("/api/status/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		s, ok := statuses.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		owner, _ := a.GetUser(s.IDUser)
		if !(s.IDUser == me.ID || me.Level > owner.Level) {
			return core.ErrForbidden()
		}
		statuses.Delete(s.ID)
		stLikes.DeleteBy("st", store.I(s.ID))
		stKomm.DeleteBy("st", store.I(s.ID))
		return c.OK()
	}, core.Auth)

	a.POST("/api/status/{id}/komm", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if err := notBanned(a, me); err != nil {
			return err
		}
		s, ok := statuses.Get(c.PathInt("id"))
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
		msg, err := checkText(a, in.Msg, 2, 1024)
		if err != nil {
			return err
		}
		for _, k := range stKomm.By("st", store.I(s.ID)) {
			if k.IDUser == me.ID && k.Msg == msg {
				return core.ErrBad("Ваше сообщение повторяет предыдущее")
			}
		}
		if in.ReplyTo > 0 && in.ReplyTo != me.ID {
			a.Notify(me.ID, in.ReplyTo, "status_komm", s.ID, true)
		}
		a.Discuss(s.IDUser, me.ID, core.KStatus, "status", s.ID)
		id := stKomm.Insert(&StatusKomm{IDUser: me.ID, IDStatus: s.ID, Time: core.Now(), Msg: msg})
		a.AddBalls(me.ID, 1, 0)
		return c.OK("id", id)
	}, core.Auth)

	a.DELETE("/api/status/komm/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		k, ok := stKomm.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		author, _ := a.GetUser(k.IDUser)
		st, _ := statuses.Get(k.IDStatus)
		if !(me.Level > author.Level || st.IDUser == me.ID) {
			return core.ErrForbidden()
		}
		stKomm.Delete(k.ID)
		return c.OK()
	}, core.Auth)

	a.POST("/api/status/komm/{id}/spam", func(c *core.Ctx) error {
		return reportSpam(c, a, "status_komm")
	}, core.Auth)

	// ================= друзья =================
	notify := func(me *model.User, to int64, typ string) { a.Notify(me.ID, to, typ, me.ID, false) }
	pairFriends := func(x, y int64) *model.Frends {
		f, ok := a.T.Friends.OneBy("pair", store.Key(x, y))
		if !ok {
			return nil
		}
		return &f
	}
	dropLinks := func(x, y int64) {
		a.T.Friends.DeleteBy("pair", store.Key(x, y))
		a.T.Friends.DeleteBy("pair", store.Key(y, x))
		a.T.FriendReqs.DeleteWhere(func(f *model.FrendsNew) bool { return (f.User == x && f.To == y) || (f.User == y && f.To == x) })
	}
	findReq := func(from, to int64) (model.FrendsNew, bool) {
		for _, r := range a.T.FriendReqs.By("to", store.I(to)) {
			if r.User == from {
				return r, true
			}
		}
		return model.FrendsNew{}, false
	}

	a.POST("/api/frends/{id}/add", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		id := c.PathInt("id")
		if id == me.ID || !a.T.Users.Has(id) {
			return core.ErrBad("Некорректный пользователь")
		}
		if st := friendState(a, me, id); st != "none" {
			return core.ErrBad("Заявка или дружба уже существует")
		}
		a.T.FriendReqs.Insert(&model.FrendsNew{User: me.ID, To: id, Time: core.Now()})
		a.Hub.ToUser(id, "counters", map[string]any{"frends": true})
		return c.OK("state", "sent")
	}, core.Auth)

	a.POST("/api/frends/{id}/accept", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		id := c.PathInt("id")
		if _, ok := findReq(id, me.ID); !ok {
			return core.ErrBad("Заявка не найдена")
		}
		now := core.Now()
		a.FanTape(me.ID, core.KFrends, "frends", id, core.TapeOpts{})
		a.FanTape(id, core.KFrends, "frends", me.ID, core.TapeOpts{})
		a.T.FriendReqs.DeleteWhere(func(f *model.FrendsNew) bool {
			return (f.User == id && f.To == me.ID) || (f.User == me.ID && f.To == id)
		})
		for _, pr := range [][2]int64{{me.ID, id}, {id, me.ID}} {
			if pairFriends(pr[0], pr[1]) == nil {
				a.T.Friends.Insert(defaultFriend(pr[0], pr[1], now))
			}
		}
		notify(me, id, "ok_frend")
		return c.OK("state", "friend")
	}, core.Auth)

	a.POST("/api/frends/{id}/decline", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		id := c.PathInt("id")
		if _, ok := findReq(id, me.ID); !ok {
			return core.ErrBad("Заявка не найдена")
		}
		dropLinks(me.ID, id)
		notify(me, id, "no_frend")
		return c.OK("state", "none")
	}, core.Auth)

	a.POST("/api/frends/{id}/cancel", func(c *core.Ctx) error { // отмена своей заявки (otm)
		me, err := c.Need()
		if err != nil {
			return err
		}
		id := c.PathInt("id")
		if _, ok := findReq(me.ID, id); !ok {
			return core.ErrBad("Заявка не найдена")
		}
		dropLinks(me.ID, id)
		notify(me, id, "otm_frend")
		return c.OK("state", "none")
	}, core.Auth)

	a.DELETE("/api/frends/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		id := c.PathInt("id")
		if !a.IsFriend(me.ID, id) {
			return core.ErrBad("Пользователь не в списке друзей")
		}
		dropLinks(me.ID, id)
		notify(me, id, "del_frend")
		return c.OK("state", "none")
	}, core.Auth)
}

func defaultFriend(user, frend, now int64) *model.Frends {
	return &model.Frends{User: user, Frend: frend, Time: now, I: 1,
		LentaForum: 1, LentaObmen: 1, LentaFoto: 1, LentaNotes: 1, LentaAvatar: 1, LentaFrends: 1, LentaStatus: 1, LentaStatusLike: 1,
		DiscForum: 1, DiscObmen: 1, DiscFoto: 1, DiscNotes: 1, DiscFrends: 1, DiscStatus: 1}
}

// reportSpam — жалоба на сообщение стены / комментарий статуса.
func reportSpam(c *core.Ctx, a *core.App, razdel string) error {
	me, err := c.Need()
	if err != nil {
		return err
	}
	id := c.PathInt("id")
	var spammer, obj int64
	var text string
	if razdel == "stena" {
		p, ok := stena.Get(id)
		if !ok {
			return core.ErrNotFound()
		}
		spammer, obj, text = p.IDUser, p.IDStena, p.Msg
	} else {
		k, ok := stKomm.Get(id)
		if !ok {
			return core.ErrNotFound()
		}
		spammer, obj, text = k.IDUser, k.IDStatus, k.Msg
	}
	if spammer == me.ID {
		return core.ErrBad("Нельзя жаловаться на себя")
	}
	var in struct {
		Msg   string `json:"msg"`
		Types int64  `json:"types"`
	}
	if err := c.Bind(&in); err != nil {
		return err
	}
	if strLen(in.Msg) < 3 || strLen(in.Msg) > 1512 {
		return core.ErrBad("Опишите нарушение (3–1512 символов)")
	}
	if a.T.Spam.Count(func(s *model.Spamus) bool {
		return s.IDUser == spammer && s.IDSpam == me.ID && s.Razdel == razdel && s.IDPost == id
	}) > 0 {
		return core.ErrBad("Жалоба уже отправлена")
	}
	a.ReportSpam(me.ID, spammer, razdel, id, obj, text, in.Msg)
	return c.OK()
}

// PostWall — системная запись на стене пользователя (например, «новый аватар»).
func PostWall(uid int64, msg string) {
	stena.Insert(&Stena{IDUser: uid, IDStena: uid, Time: core.Now(), Msg: msg})
}
