package groups

import (
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

func registerSections(a *core.App) {
	// ======================== БЛОГ СООБЩЕСТВА ========================
	a.GET("/api/groups/{id}/blog", func(c *core.Ctx) error {
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Сообщество не найдено")
		}
		if !g.EnableBlog {
			return core.ErrForbidden("Раздел «Блог» отключён в этом сообществе")
		}
		me := c.User()
		pr := permsOf(me, g)
		p := c.Page(15)
		all := blogT.By("group", i(g.ID))
		sort.Slice(all, func(x, y int) bool {
			if all[x].Pinned != all[y].Pinned {
				return all[x].Pinned
			}
			return all[x].ID > all[y].ID
		})
		total := len(all)
		end := p.Offset + p.Per
		if end > total {
			end = total
		}
		var sl []GroupPost
		if p.Offset < total {
			sl = all[p.Offset:end]
		}
		items := make([]map[string]any, 0, len(sl))
		for _, post := range sl {
			likes := a.T.Likes.By("obj", store.Key("gpost", post.ID))
			liked := false
			lcnt := 0
			for _, l := range likes {
				if l.Like {
					lcnt++
					if me != nil && l.IDUser == me.ID {
						liked = true
					}
				}
			}
			items = append(items, map[string]any{
				"id":         post.ID,
				"user":       a.Ref(post.IDUser),
				"title":      post.Title,
				"msg":        a.Text.Render(post.Msg),
				"raw":        post.Msg,
				"time":       post.Time,
				"pinned":     post.Pinned,
				"likes":      lcnt,
				"liked":      liked,
				"komms":      blogKT.CountBy("post", i(post.ID)),
				"can_delete": pr.ModBlog || (me != nil && me.ID == post.IDUser),
				"can_pin":    pr.ModBlog,
			})
		}
		return c.List(items, total, p)
	})

	a.POST("/api/groups/{id}/blog", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok || !g.EnableBlog {
			return core.ErrNotFound()
		}
		pr := permsOf(me, g)
		if !pr.ModBlog && !(g.OpenBlog && pr.IsMember) {
			return core.ErrForbidden("Публиковать записи могут только модераторы и администраторы блога")
		}
		var in struct {
			Title string `json:"title"`
			Msg   string `json:"msg"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		title := a.Text.Clean(strings.TrimSpace(in.Title))
		msg := a.Text.Clean(strings.TrimSpace(in.Msg))
		if utf8.RuneCountInString(msg) < 2 || utf8.RuneCountInString(msg) > 4096 {
			return core.ErrBad("Текст записи: от 2 до 4096 символов")
		}
		if utf8.RuneCountInString(title) > 120 {
			title = core.Trunc(title, 120)
		}
		pid := blogT.Insert(&GroupPost{IDGroup: g.ID, IDUser: me.ID, Title: title, Msg: msg, Time: core.Now()})
		return c.OK("id", pid)
	}, core.Auth)

	a.POST("/api/groups/blog/{pid}/like", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		pid := c.PathInt("pid")
		if !blogT.Has(pid) {
			return core.ErrNotFound()
		}
		key := store.Key(me.ID, "gpost", pid)
		liked := false
		if ex, ok := a.T.Likes.OneBy("uobj", key); ok {
			a.T.Likes.Delete(ex.ID)
		} else {
			a.T.Likes.Insert(&model.LikeObject{IDUser: me.ID, Type: "gpost", IDObject: pid, Like: true})
			liked = true
		}
		cnt := 0
		for _, l := range a.T.Likes.By("obj", store.Key("gpost", pid)) {
			if l.Like {
				cnt++
			}
		}
		return c.OK("liked", liked, "likes", cnt)
	}, core.Auth)

	a.POST("/api/groups/blog/{pid}/pin", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		post, ok := blogT.Get(c.PathInt("pid"))
		if !ok {
			return core.ErrNotFound()
		}
		g, _ := groupsT.Get(post.IDGroup)
		if !permsOf(me, g).ModBlog {
			return core.ErrForbidden()
		}
		upd, _ := blogT.Update(post.ID, func(x *GroupPost) { x.Pinned = !x.Pinned })
		return c.OK("pinned", upd.Pinned)
	}, core.Auth)

	a.DELETE("/api/groups/blog/{pid}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		post, ok := blogT.Get(c.PathInt("pid"))
		if !ok {
			return core.ErrNotFound()
		}
		g, _ := groupsT.Get(post.IDGroup)
		if !permsOf(me, g).ModBlog && me.ID != post.IDUser {
			return core.ErrForbidden()
		}
		blogT.Delete(post.ID)
		blogKT.DeleteBy("post", i(post.ID))
		a.T.Likes.DeleteWhere(func(l *model.LikeObject) bool { return l.Type == "gpost" && l.IDObject == post.ID })
		return c.OK()
	}, core.Auth)

	a.GET("/api/groups/blog/{pid}/komms", func(c *core.Ctx) error {
		post, ok := blogT.Get(c.PathInt("pid"))
		if !ok {
			return core.ErrNotFound()
		}
		g, _ := groupsT.Get(post.IDGroup)
		me := c.User()
		pr := permsOf(me, g)
		p := c.Page(20)
		list, total := blogKT.Select(store.Opts[GroupPostKomm]{Index: "post", Key: i(post.ID), Desc: true, Offset: p.Offset, Limit: p.Per})
		items := make([]map[string]any, 0, len(list))
		for _, k := range list {
			items = append(items, map[string]any{
				"id":         k.ID,
				"user":       a.Ref(k.IDUser),
				"time":       k.Time,
				"msg":        a.Text.Render(k.Msg),
				"can_delete": pr.ModBlog || (me != nil && me.ID == k.IDUser),
			})
		}
		return c.List(items, total, p)
	})

	a.POST("/api/groups/blog/{pid}/komms", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		post, ok := blogT.Get(c.PathInt("pid"))
		if !ok {
			return core.ErrNotFound()
		}
		g, _ := groupsT.Get(post.IDGroup)
		if !g.EnableBlog {
			return core.ErrForbidden()
		}
		var in struct {
			Msg string `json:"msg"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		msg := a.Text.Clean(strings.TrimSpace(in.Msg))
		if utf8.RuneCountInString(msg) < 1 || utf8.RuneCountInString(msg) > 1024 {
			return core.ErrBad("Комментарий: от 1 до 1024 символов")
		}
		id := blogKT.Insert(&GroupPostKomm{IDPost: post.ID, IDGroup: g.ID, IDUser: me.ID, Time: core.Now(), Msg: msg})
		return c.OK("id", id)
	}, core.Auth)

	a.DELETE("/api/groups/blog/komm/{kid}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		k, ok := blogKT.Get(c.PathInt("kid"))
		if !ok {
			return core.ErrNotFound()
		}
		g, _ := groupsT.Get(k.IDGroup)
		if !permsOf(me, g).ModBlog && me.ID != k.IDUser {
			return core.ErrForbidden()
		}
		blogKT.Delete(k.ID)
		return c.OK()
	}, core.Auth)

	// ======================== ФОРУМ СООБЩЕСТВА ========================
	a.GET("/api/groups/{id}/forum", func(c *core.Ctx) error {
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Сообщество не найдено")
		}
		if !g.EnableForum {
			return core.ErrForbidden("Раздел «Форум» отключён в этом сообществе")
		}
		me := c.User()
		pr := permsOf(me, g)
		p := c.Page(20)
		all := topicsT.By("group", i(g.ID))
		sort.Slice(all, func(x, y int) bool {
			if all[x].Pinned != all[y].Pinned {
				return all[x].Pinned
			}
			return all[x].TimeLast > all[y].TimeLast
		})
		total := len(all)
		end := p.Offset + p.Per
		if end > total {
			end = total
		}
		var sl []GroupTopic
		if p.Offset < total {
			sl = all[p.Offset:end]
		}
		items := make([]map[string]any, 0, len(sl))
		for _, t := range sl {
			items = append(items, map[string]any{
				"id":        t.ID,
				"name":      t.Name,
				"user":      a.Ref(t.IDUser),
				"time":      t.Time,
				"time_last": t.TimeLast,
				"closed":    t.Closed,
				"pinned":    t.Pinned,
				"posts":     tpostsT.CountBy("topic", i(t.ID)),
				"can_mod":   pr.ModForum,
			})
		}
		return c.List(items, total, p)
	})

	a.POST("/api/groups/{id}/forum", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok || !g.EnableForum {
			return core.ErrNotFound()
		}
		if !permsOf(me, g).IsMember {
			return core.ErrForbidden("Вступите в сообщество, чтобы создавать темы")
		}
		var in struct {
			Name string `json:"name"`
			Msg  string `json:"msg"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		name := a.Text.Clean(strings.TrimSpace(in.Name))
		msg := a.Text.Clean(strings.TrimSpace(in.Msg))
		if utf8.RuneCountInString(name) < 2 || utf8.RuneCountInString(name) > 120 {
			return core.ErrBad("Название темы: 2–120 символов")
		}
		if utf8.RuneCountInString(msg) < 2 || utf8.RuneCountInString(msg) > 4096 {
			return core.ErrBad("Текст темы: 2–4096 символов")
		}
		now := core.Now()
		tid := topicsT.Insert(&GroupTopic{IDGroup: g.ID, IDUser: me.ID, Name: name, Msg: msg, Time: now, TimeLast: now})
		return c.OK("id", tid)
	}, core.Auth)

	a.GET("/api/groups/forum/{tid}", func(c *core.Ctx) error {
		t, ok := topicsT.Get(c.PathInt("tid"))
		if !ok {
			return core.ErrNotFound("Тема не найдена")
		}
		g, _ := groupsT.Get(t.IDGroup)
		me := c.User()
		pr := permsOf(me, g)
		p := c.Page(20)
		list, total := tpostsT.Select(store.Opts[GroupTopicPost]{Index: "topic", Key: i(t.ID), Offset: p.Offset, Limit: p.Per})
		posts := make([]map[string]any, 0, len(list))
		for _, r := range list {
			posts = append(posts, map[string]any{
				"id":         r.ID,
				"user":       a.Ref(r.IDUser),
				"time":       r.Time,
				"msg":        a.Text.Render(r.Msg),
				"can_delete": pr.ModForum || (me != nil && me.ID == r.IDUser),
			})
		}
		return c.JSON(map[string]any{
			"topic": map[string]any{
				"id": t.ID, "group_id": t.IDGroup, "name": t.Name, "msg": a.Text.Render(t.Msg),
				"user": a.Ref(t.IDUser), "time": t.Time, "closed": t.Closed, "pinned": t.Pinned,
				"can_mod": pr.ModForum, "can_delete": pr.ModForum || (me != nil && me.ID == t.IDUser),
				"can_reply": pr.IsMember && (!t.Closed || pr.ModForum),
			},
			"items": posts, "total": total, "page": p.Page, "per": p.Per, "pages": max(1, (total+p.Per-1)/p.Per),
		})
	})

	a.POST("/api/groups/forum/{tid}/posts", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		t, ok := topicsT.Get(c.PathInt("tid"))
		if !ok {
			return core.ErrNotFound("Тема не найдена")
		}
		g, _ := groupsT.Get(t.IDGroup)
		pr := permsOf(me, g)
		if !g.EnableForum || !pr.IsMember {
			return core.ErrForbidden("Вступите в сообщество, чтобы отвечать")
		}
		if t.Closed && !pr.ModForum {
			return core.ErrForbidden("Тема закрыта")
		}
		var in struct {
			Msg string `json:"msg"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		msg := a.Text.Clean(strings.TrimSpace(in.Msg))
		if utf8.RuneCountInString(msg) < 1 || utf8.RuneCountInString(msg) > 4096 {
			return core.ErrBad("Сообщение: 1–4096 символов")
		}
		now := core.Now()
		pid := tpostsT.Insert(&GroupTopicPost{IDTopic: t.ID, IDGroup: g.ID, IDUser: me.ID, Time: now, Msg: msg})
		topicsT.Update(t.ID, func(x *GroupTopic) { x.TimeLast = now })
		return c.OK("id", pid)
	}, core.Auth)

	a.POST("/api/groups/forum/{tid}/toggle", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		t, ok := topicsT.Get(c.PathInt("tid"))
		if !ok {
			return core.ErrNotFound()
		}
		g, _ := groupsT.Get(t.IDGroup)
		if !permsOf(me, g).ModForum {
			return core.ErrForbidden()
		}
		var in struct {
			Field string `json:"field"` // pinned | closed
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		upd, _ := topicsT.Update(t.ID, func(x *GroupTopic) {
			if in.Field == "pinned" {
				x.Pinned = !x.Pinned
			} else if in.Field == "closed" {
				x.Closed = !x.Closed
			}
		})
		return c.OK("pinned", upd.Pinned, "closed", upd.Closed)
	}, core.Auth)

	a.DELETE("/api/groups/forum/{tid}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		t, ok := topicsT.Get(c.PathInt("tid"))
		if !ok {
			return core.ErrNotFound()
		}
		g, _ := groupsT.Get(t.IDGroup)
		if !permsOf(me, g).ModForum && me.ID != t.IDUser {
			return core.ErrForbidden()
		}
		topicsT.Delete(t.ID)
		tpostsT.DeleteBy("topic", i(t.ID))
		return c.OK()
	}, core.Auth)

	a.DELETE("/api/groups/forum/post/{pid}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		r, ok := tpostsT.Get(c.PathInt("pid"))
		if !ok {
			return core.ErrNotFound()
		}
		g, _ := groupsT.Get(r.IDGroup)
		if !permsOf(me, g).ModForum && me.ID != r.IDUser {
			return core.ErrForbidden()
		}
		tpostsT.Delete(r.ID)
		return c.OK()
	}, core.Auth)

	// ======================== ЧАТ СООБЩЕСТВА ========================
	a.GET("/api/groups/{id}/chat", func(c *core.Ctx) error {
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		if !g.EnableChat {
			return core.ErrForbidden("Раздел «Чат» отключён в этом сообществе")
		}
		me := c.User()
		pr := permsOf(me, g)
		list, total := chatT.Select(store.Opts[GroupChat]{Index: "group", Key: i(g.ID), Desc: true, Limit: 60})
		// Хронологический порядок для мессенджера
		items := make([]map[string]any, 0, len(list))
		for k := len(list) - 1; k >= 0; k-- {
			m := list[k]
			items = append(items, map[string]any{
				"id":         m.ID,
				"user":       a.Ref(m.IDUser),
				"time":       m.Time,
				"msg":        a.Text.Render(m.Msg),
				"can_delete": pr.ModChat || (me != nil && me.ID == m.IDUser),
			})
		}
		return c.JSON(map[string]any{"items": items, "total": total, "can_write": pr.IsMember, "can_mod": pr.ModChat})
	})

	a.POST("/api/groups/{id}/chat", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok || !g.EnableChat {
			return core.ErrNotFound()
		}
		if !permsOf(me, g).IsMember {
			return core.ErrForbidden("Вступите в сообщество, чтобы писать в чат")
		}
		var in struct {
			Msg string `json:"msg"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		msg := a.Text.Clean(strings.TrimSpace(in.Msg))
		if utf8.RuneCountInString(msg) < 1 || utf8.RuneCountInString(msg) > 1024 {
			return core.ErrBad("Сообщение: 1–1024 символа")
		}
		m := GroupChat{IDGroup: g.ID, IDUser: me.ID, Time: core.Now(), Msg: msg}
		id := chatT.Insert(&m)
		payload := map[string]any{"id": id, "group_id": g.ID, "user": a.Ref(me.ID), "time": m.Time, "msg": a.Text.Render(m.Msg)}
		a.Hub.Publish("group:"+i(g.ID), "group_chat", payload)
		return c.JSON(payload)
	}, core.Auth)

	a.DELETE("/api/groups/chat/{mid}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		m, ok := chatT.Get(c.PathInt("mid"))
		if !ok {
			return core.ErrNotFound()
		}
		g, _ := groupsT.Get(m.IDGroup)
		if !permsOf(me, g).ModChat && me.ID != m.IDUser {
			return core.ErrForbidden()
		}
		chatT.Delete(m.ID)
		return c.OK()
	}, core.Auth)

	// ======================== ФАЙЛЫ СООБЩЕСТВА ========================
	a.GET("/api/groups/{id}/files", func(c *core.Ctx) error {
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		if !g.EnableFiles {
			return core.ErrForbidden("Раздел «Файлы» отключён в этом сообществе")
		}
		me := c.User()
		pr := permsOf(me, g)
		p := c.Page(20)
		list, total := filesT.Select(store.Opts[GroupFile]{Index: "group", Key: i(g.ID), Desc: true, Offset: p.Offset, Limit: p.Per})
		items := make([]map[string]any, 0, len(list))
		for _, f := range list {
			items = append(items, map[string]any{
				"id":         f.ID,
				"name":       f.Name,
				"ras":        f.Ras,
				"size":       f.Size,
				"size_str":   core.SizeStr(f.Size),
				"opis":       a.Text.Render(f.Opis),
				"time":       f.Time,
				"loads":      f.Loads,
				"user":       a.Ref(f.IDUser),
				"download":   "/api/groups/files/" + i(f.ID) + "/download",
				"can_delete": pr.ModFiles || (me != nil && me.ID == f.IDUser),
			})
		}
		return c.List(items, total, p)
	})

	a.POST("/api/groups/{id}/files", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok || !g.EnableFiles {
			return core.ErrNotFound()
		}
		if !permsOf(me, g).IsMember {
			return core.ErrForbidden("Вступите в сообщество, чтобы загружать файлы")
		}
		maxMB := int64(a.Cfg.Int("max_upload_file_mb"))
		if maxMB <= 0 {
			maxMB = 100
		}
		up, err := c.FormFile("file", maxMB<<20)
		if err != nil {
			return err
		}
		if up == nil || len(up.Data) == 0 {
			return core.ErrBad("Файл не выбран")
		}
		name := a.Text.Clean(strings.TrimSpace(c.FormValue("name")))
		if name == "" {
			name = strings.TrimSuffix(up.Name, "."+up.Ext)
		}
		if utf8.RuneCountInString(name) > 120 {
			name = core.Trunc(name, 120)
		}
		if name == "" {
			name = "Файл"
		}
		opis := a.Text.Clean(strings.TrimSpace(c.FormValue("opis")))
		if utf8.RuneCountInString(opis) > 500 {
			return core.ErrBad("Описание слишком длинное (максимум 500 символов)")
		}
		f := GroupFile{IDGroup: g.ID, IDUser: me.ID, Name: name, Ras: strings.ToLower(up.Ext), Type: up.MIME, Size: up.Size, Opis: opis, Time: core.Now()}
		id := filesT.Insert(&f)
		if err := core.WriteFile(gfilePath(a, id), up.Data); err != nil {
			filesT.Delete(id)
			return core.Err(500, "Ошибка сохранения файла")
		}
		return c.OK("id", id)
	}, core.Auth)

	a.GET("/api/groups/files/{fid}/download", func(c *core.Ctx) error {
		f, ok := filesT.Get(c.PathInt("fid"))
		if !ok {
			return core.ErrNotFound("Файл не найден")
		}
		filesT.Update(f.ID, func(x *GroupFile) { x.Loads++ })
		dlName := f.Name
		if f.Ras != "" {
			dlName += "." + f.Ras
		}
		return c.ServeFile(gfilePath(a, f.ID), dlName, "application/octet-stream")
	})

	a.DELETE("/api/groups/files/{fid}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		f, ok := filesT.Get(c.PathInt("fid"))
		if !ok {
			return core.ErrNotFound()
		}
		g, _ := groupsT.Get(f.IDGroup)
		if !permsOf(me, g).ModFiles && me.ID != f.IDUser {
			return core.ErrForbidden()
		}
		filesT.Delete(f.ID)
		_ = os.Remove(gfilePath(a, f.ID))
		return c.OK()
	}, core.Auth)
}
