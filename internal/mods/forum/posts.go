package forum

import (
	"strings"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

func filesOf(a *core.App, c *core.Ctx, postID int64) []map[string]any {
	out := []map[string]any{}
	me := c.User()
	for _, f := range ffiles.By("post", i(postID)) {
		rs := frating.By("file", i(f.ID))
		var sum int64
		var mine int64
		for _, r := range rs {
			sum += r.Rating
			if me != nil && r.IDUser == me.ID {
				mine = r.Rating
			}
		}
		img := false
		switch f.Ras {
		case "jpg", "jpeg", "png", "gif":
			img = true
		}
		out = append(out, map[string]any{"id": f.ID, "name": f.Name, "ext": f.Ras, "size": f.Size, "size_str": core.SizeStr(f.Size), "count": f.Count,
			"rating": sum, "votes": len(rs), "my_vote": mine, "image": img, "url": "/api/forum-file/" + i(f.ID),
			"can_vote": me != nil && me.Balls >= 50 && me.Rating >= 0 && mine == 0})
	}
	return out
}

func postView(a *core.App, c *core.Ctx, p Post, last Post) map[string]any {
	m := map[string]any{"id": p.ID, "user": a.Ref(p.IDUser), "time": p.Time, "msg": a.Text.Render(p.Msg), "system": p.IDUser == 0, "files": filesOf(a, c, p.ID)}
	if p.Cit > 0 {
		if q, ok := posts.Get(p.Cit); ok {
			m["cit"] = map[string]any{"id": q.ID, "user": a.Ref(q.IDUser), "time": q.Time, "msg": a.Text.Render(q.Msg)}
		}
	}
	ed, del := postCan(c, a, p, last)
	m["can_edit"], m["can_delete"] = ed, del
	m["raw"] = ""
	if ed {
		m["raw"] = p.Msg
	}
	return m
}

func deletePostFiles(a *core.App, postID int64) {
	for _, f := range ffiles.By("post", i(postID)) {
		removeFile(a, f.ID)
	}
}

func removeFile(a *core.App, id int64) {
	frating.DeleteBy("file", i(id))
	ffiles.Delete(id)
	core.RemoveFile(filePath(a, id))
}

func filePath(a *core.App, id int64) string { return a.UploadDir("forum") + "/" + i(id) + ".frf" }

const maxForumFile = 10 << 20

func registerPosts(a *core.App) {
	// ---- новое сообщение (JSON или multipart с файлами files[])
	a.POST("/api/forum/{f}/{r}/{t}/post", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if err := forumBan(a, me); err != nil {
			return err
		}
		f, r, t, err := themOf(c)
		if err != nil {
			return err
		}
		if !canWritePost(c, t) {
			return core.ErrForbidden("Тема закрыта для обсуждения")
		}
		var msg string
		var cit, replyTo int64
		var uploads []*core.Upload
		if strings.HasPrefix(c.R.Header.Get("Content-Type"), "multipart/") {
			fMB := int64(a.Cfg.Int("max_upload_forum_mb"))
			if fMB <= 0 {
				fMB = maxForumFile >> 20
			}
			fCnt := a.Cfg.Int("max_upload_forum_count")
			if fCnt <= 0 {
				fCnt = 5
			}
			if uploads, err = c.FormFiles("files", fMB<<20, fCnt); err != nil {
				return err
			}
			for _, u := range uploads {
				if u.Ext == "" {
					return core.ErrBad("У файла «" + u.Name + "» нет расширения")
				}
			}
			msg = c.FormValue("msg")
			cit = int64(atoi(c.FormValue("cit")))
			replyTo = int64(atoi(c.FormValue("reply_to")))
		} else {
			var in struct {
				Msg     string `json:"msg"`
				Cit     int64  `json:"cit"`
				ReplyTo int64  `json:"reply_to"`
			}
			if err := c.Bind(&in); err != nil {
				return err
			}
			msg, cit, replyTo = in.Msg, in.Cit, in.ReplyTo
		}
		msg = a.Text.Clean(strings.TrimSpace(msg))
		if err := lenOK(msg, 2, 1024, "Сообщение"); err != nil {
			return err
		}
		if err := noMat(a, msg, "сообщении"); err != nil {
			return err
		}
		for _, p := range posts.By("them", i(t.ID)) {
			if p.IDUser == me.ID && p.Msg == msg {
				return core.ErrBad("Ваше сообщение повторяет предыдущее")
			}
		}
		var citAuthor int64
		if cit > 0 {
			q, ok := posts.Get(cit)
			if !ok || q.IDThem != t.ID {
				cit = 0
			} else {
				citAuthor = q.IDUser
			}
		}
		now := core.Now()
		p := Post{IDForum: f.ID, IDRazdel: r.ID, IDThem: t.ID, IDUser: me.ID, Time: now, Msg: msg, Cit: cit}
		posts.Insert(&p)
		for _, u := range uploads {
			ff := FFile{IDPost: p.ID, Name: core.SafeName(strings.TrimSuffix(u.Name, "."+u.Ext)), Ras: u.Ext, Size: u.Size, Type: u.MIME}
			ffiles.Insert(&ff)
			if err := core.WriteFile(filePath(a, ff.ID), u.Data); err != nil {
				ffiles.Delete(ff.ID)
			}
		}
		a.T.Users.Update(me.ID, func(u *model.User) { u.ForumThemComm++ })
		a.ForgetUser(me.ID)
		a.AddBalls(me.ID, 1, 0)
		a.AddRatingTmp(me.ID, 1)
		thems.Update(t.ID, func(x *Them) { x.Time = now })
		razdels.Update(r.ID, func(x *Razdel) { x.Time = now })
		a.T.Bookmarks.UpdateWhere(func(b *model.Bookmarks) bool { return b.Type == "forum" && b.IDObject == t.ID }, func(b *model.Bookmarks) { b.TimeObn = now })
		a.Discuss(t.IDUser, me.ID, core.KForum, "them", t.ID)
		notify := citAuthor
		if notify == 0 {
			notify = replyTo
		}
		if notify > 0 && notify != me.ID && a.T.Users.Has(notify) {
			a.Notify(me.ID, notify, "them_komm", t.ID, true)
		}
		return c.JSON(map[string]any{"id": p.ID, "page_last": max(1, (postCount(t)+c.Page().Per-1)/c.Page().Per)})
	}, core.Auth)

	// ---- редактирование сообщения
	a.PUT("/api/forum-post/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		p, ok := posts.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		last, _ := lastPost(mustThem(p.IDThem))
		if ed, _ := postCan(c, a, p, last); !ed {
			return core.ErrForbidden("Редактирование недоступно")
		}
		var in struct {
			Msg string `json:"msg"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		msg := a.Text.Clean(strings.TrimSpace(in.Msg))
		if err := lenOK(msg, 2, 1024, "Сообщение"); err != nil {
			return err
		}
		if err := noMat(a, msg, "сообщении"); err != nil {
			return err
		}
		posts.Update(p.ID, func(x *Post) { x.Msg = msg })
		if p.IDUser != me.ID {
			t := mustThem(p.IDThem)
			a.AdminLog(me.ID, "Форум", "Сообщения", "Редактирование сообщения в теме '"+t.Name+"' (автор '"+mustUser(a, p.IDUser).Nick+"')")
		}
		return c.OK()
	}, core.Auth)

	// ---- удаление сообщения
	a.DELETE("/api/forum-post/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		p, ok := posts.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		t := mustThem(p.IDThem)
		last, _ := lastPost(t)
		if _, del := postCan(c, a, p, last); !del {
			return core.ErrForbidden("Удаление недоступно")
		}
		deletePostFiles(a, p.ID)
		posts.Delete(p.ID)
		if p.IDUser != me.ID {
			a.AdminLog(me.ID, "Форум", "Сообщения", "Удаление сообщения в теме '"+t.Name+"' (автор '"+mustUser(a, p.IDUser).Nick+"')")
		}
		return c.OK()
	}, core.Auth)

	// ---- жалоба
	a.POST("/api/forum-post/{id}/spam", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		p, ok := posts.Get(c.PathInt("id"))
		if !ok || p.IDUser == 0 {
			return core.ErrNotFound()
		}
		if p.IDUser == me.ID {
			return core.ErrBad("Нельзя жаловаться на себя")
		}
		var in struct {
			Msg   string `json:"msg"`
			Types int64  `json:"types"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if err := lenOK(in.Msg, 3, 1512, "Описание жалобы"); err != nil {
			return err
		}
		if a.T.Spam.Count(func(s *model.Spamus) bool {
			return s.IDUser == me.ID && s.IDSpam == p.IDUser && s.Razdel == "forum" && s.Spam == p.Msg
		}) > 0 {
			return core.ErrBad("Жалоба уже отправлена")
		}
		a.ReportSpam(me.ID, p.IDUser, "forum", p.ID, p.IDThem, p.Msg, in.Msg)
		return c.OK()
	}, core.Auth)
}

func mustThem(id int64) Them { t, _ := thems.Get(id); return t }

var _ = store.I
