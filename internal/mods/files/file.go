package files

import (
	"strings"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

// fileAccess — доступ к файлу: пароль личной папки (родителей тоже).
func fileOpen(a *core.App, c *core.Ctx, f File) bool {
	if fo, ok := folders.Get(f.MyDir); ok {
		return folderChainOpen(a, c, fo)
	}
	return true
}

func registerFiles(a *core.App) {
	a.GET("/api/obmen/file/{id}", func(c *core.Ctx) error {
		if err := banErr(a, c); err != nil {
			return err
		}
		f, ok := files.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Файл не найден")
		}
		me := c.User()
		d, _ := dirs.Get(f.IDDir)
		fo, _ := folders.Get(f.MyDir)
		out := map[string]any{"id": f.ID, "name": f.Name, "ras": f.Ras, "owner": a.Ref(f.IDUser), "dir": map[string]any{"id": d.ID, "name": d.Name, "my": d.My},
			"folder": map[string]any{"id": fo.ID, "name": fo.Name, "path": folderPath(fo)}, "echo_ras": a.Cfg.Int("echo_rassh")}
		if !fileOpen(a, c, f) {
			out["blocked"] = "password"
			out["folder_id"] = fo.ID
			return c.JSON(out)
		}
		out["metka"] = f.Metka
		if !adultOK(c, f) {
			out["adult"] = true
			out["need_login"] = me == nil
			return c.JSON(out)
		}
		out["opis"], out["raw_opis"] = a.Text.Render(f.Opis), f.Opis
		out["size"], out["size_str"], out["time"], out["loads"], out["type"] = f.Size, core.SizeStr(f.Size), f.Time, f.KLoads, f.Type
		out["screen"] = f.Screen
		if f.Screen {
			out["screen_url"] = "/api/obmen/file/" + i(f.ID) + "/screen"
		}
		out["download"] = "/api/obmen/file/" + i(f.ID) + "/download"
		out["can_edit"], out["can_delete"] = canEditFile(a, me, f), canDeleteFile(a, me, f)
		out["mine"] = me != nil && me.ID == f.IDUser
		ext := strings.ToLower(f.Ras)
		out["kind"] = map[string]string{"mp3": "audio", "wav": "audio", "ogg": "audio", "flac": "audio", "m4a": "audio", "mp4": "video", "webm": "video",
			"jpg": "image", "jpeg": "image", "png": "image", "gif": "image", "webp": "image"}[ext]
		likes, dislikes, my := 0, 0, -1
		for _, l := range a.T.Likes.By("obj", store.Key("obmen", f.ID)) {
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
		if me != nil {
			_, fav := a.T.Bookmarks.OneBy("uobj", store.Key(me.ID, "file", f.ID))
			out["bookmarked"] = fav
			if isAudio(f.Ras) {
				out["music"] = map[string]any{"in": music.Count(func(m *Music) bool { return m.IDUser == me.ID && m.IDFile == f.ID }) > 0, "people": music.CountBy("file", i(f.ID))}
			}
			a.T.Notifications.UpdateWhere(func(n *model.Notification) bool {
				return n.IDUser == me.ID && (n.Type == "obmen_komm" || n.Type == "files_komm") && n.IDObject == f.ID && n.Read == 0
			}, func(n *model.Notification) { n.Read = 1 })
			for _, ds := range a.T.Discussions.By("user", i(me.ID)) {
				if ds.Type == "obmen" && ds.IDSim == f.ID && ds.Count > 0 {
					a.T.Discussions.Update(ds.ID, func(x *model.Discussions) { x.Count = 0 })
				}
			}
			if s := c.Q("sort"); s == "0" || s == "1" {
				v := int64(0)
				if s == "1" {
					v = 1
				}
				a.T.Users.Update(me.ID, func(u *model.User) { u.Sort = v })
				me.Sort = v
			}
		}
		out["bookmarks"] = a.T.Bookmarks.Count(func(b *model.Bookmarks) bool { return b.Type == "file" && b.IDObject == f.ID })
		// соседи по личной папке
		if prev, _ := files.Select(store.Opts[File]{Index: "mydir", Key: i(f.MyDir), Desc: true, Limit: 1, Where: func(x *File) bool { return x.ID < f.ID }}); len(prev) > 0 {
			out["older"] = prev[0].ID
		}
		if nxt, _ := files.Select(store.Opts[File]{Index: "mydir", Key: i(f.MyDir), Limit: 1, Where: func(x *File) bool { return x.ID > f.ID }}); len(nxt) > 0 {
			out["newer"] = nxt[0].ID
		}
		p := c.Page()
		desc := !(me != nil && me.Sort == 1)
		list, total := komms.Select(store.Opts[Komm]{Index: "file", Key: i(f.ID), Desc: desc, Offset: p.Offset, Limit: p.Per})
		ks := make([]map[string]any, 0, len(list))
		for _, k := range list {
			m := map[string]any{"id": k.ID, "msg": a.Text.Render(k.Msg), "time": k.Time, "user": a.Ref(k.IDUser)}
			if me != nil {
				m["can_reply"] = k.IDUser != me.ID
				m["can_delete"] = me.ID == f.IDUser || a.Can(me.ID, "obmen_komm_del")
				m["can_spam"] = k.IDUser != me.ID
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

	a.GET("/api/obmen/file/{id}/screen", func(c *core.Ctx) error {
		f, ok := files.Get(c.PathInt("id"))
		if !ok || !f.Screen || !fileOpen(a, c, f) {
			return core.ErrNotFound()
		}
		c.W.Header().Set("Cache-Control", "public, max-age=86400")
		return c.ServeFile(screenPath(a, f.ID), "", "image/jpeg")
	})

	a.GET("/api/obmen/file/{id}/download", func(c *core.Ctx) error {
		if err := banErr(a, c); err != nil {
			return err
		}
		f, ok := files.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Файл не найден")
		}
		if !fileOpen(a, c, f) {
			return core.ErrForbidden("Папка защищена паролем")
		}
		if !adultOK(c, f) {
			return core.ErrForbidden("Файл содержит материалы 18+")
		}
		if me := c.User(); me != nil && me.ID != f.IDUser {
			if _, dup := downloaded.LoadOrStore(i(me.ID)+"|"+i(f.ID), struct{}{}); !dup {
				a.AddRatingTmp(f.IDUser, 1)
			}
		}
		files.Update(f.ID, func(x *File) { x.KLoads++ })
		name := f.Name + "." + f.Ras
		mime := f.Type
		if c.Q("inline") == "1" && (strings.HasPrefix(mime, "audio/") || strings.HasPrefix(mime, "video/") || strings.HasPrefix(mime, "image/")) {
			return c.ServeFile(filePath(a, f.ID), "", mime)
		}
		return c.ServeFile(filePath(a, f.ID), name, mime)
	})

	a.PUT("/api/obmen/file/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		f, ok := files.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		if !canEditFile(a, me, f) {
			return core.ErrForbidden()
		}
		var in struct {
			Name  string `json:"name"`
			Opis  string `json:"opis"`
			Metka bool   `json:"metka"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		in.Name, in.Opis = strings.TrimSpace(in.Name), strings.TrimSpace(in.Opis)
		if err := lenOK(in.Name, 2, 128, "Название"); err != nil {
			return err
		}
		if err := lenOK(in.Opis, 0, 2048, "Описание"); err != nil {
			return err
		}
		files.Update(f.ID, func(x *File) { x.Name, x.Opis, x.Metka = a.Text.Clean(in.Name), in.Opis, in.Metka })
		if me.ID != f.IDUser {
			a.AdminLog(me.ID, "Обменник", "Редактирование файла", "Редактирование файла [url=/obmen/file/"+i(f.ID)+"]"+f.Name+"[/url]")
		}
		return c.OK()
	}, core.Auth)

	a.DELETE("/api/obmen/file/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		f, ok := files.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		if !canDeleteFile(a, me, f) {
			return core.ErrForbidden()
		}
		deleteFile(a, f)
		if me.ID != f.IDUser {
			a.AdminLog(me.ID, "Обменник", "Удаление файла", "Удалён файл '"+f.Name+"' пользователя "+a.Ref(f.IDUser).Nick)
		}
		return c.OK("dir", f.IDDir, "folder", f.MyDir, "owner", f.IDUser)
	}, core.Auth)

	// выложить файл в зону обмена (dir=0 — снять с публикации)
	a.POST("/api/obmen/file/{id}/publish", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		f, ok := files.Get(c.PathInt("id"))
		if !ok || f.IDUser != me.ID {
			return core.ErrForbidden()
		}
		var in struct {
			Dir int64 `json:"dir"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if in.Dir == 0 {
			files.Update(f.ID, func(x *File) { x.IDDir, x.TimeGo = myDir().ID, 0 })
			return c.OK()
		}
		d, ok := dirs.Get(in.Dir)
		if !ok || !d.Upload || d.My {
			return core.ErrBad("Эта папка не доступна!")
		}
		if !extAllowed(d, f.Ras) {
			return core.ErrBad("Неверное расширение файла")
		}
		if d.MaxSize > 0 && f.Size > d.MaxSize {
			return core.ErrBad("Размер файла превышает установленные ограничения")
		}
		if lim := int64(a.Cfg.Int("obmen_limit_up")); me.Balls < lim {
			return core.ErrBad("Для выгрузки в зону обмена нужно набрать минимум " + itoa(int(lim)) + " баллов")
		}
		files.Update(f.ID, func(x *File) { x.IDDir, x.TimeGo = d.ID, core.Now() })
		return c.OK("dir", d.ID)
	}, core.Auth)

	// переместить файл в другую личную папку
	a.POST("/api/obmen/file/{id}/move", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		f, ok := files.Get(c.PathInt("id"))
		if !ok || f.IDUser != me.ID {
			return core.ErrForbidden()
		}
		var in struct {
			Folder int64 `json:"folder"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		fo, ok := folders.Get(in.Folder)
		if !ok || fo.IDUser != me.ID {
			return core.ErrBad("Папка не найдена")
		}
		files.Update(f.ID, func(x *File) { x.MyDir = fo.ID })
		return c.OK()
	}, core.Auth)

	a.POST("/api/obmen/file/{id}/like", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		f, ok := files.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		if f.IDUser == me.ID {
			return core.ErrBad("Нельзя голосовать за свой файл")
		}
		var in struct {
			Like bool `json:"like"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if _, ok := a.T.Likes.OneBy("uobj", store.Key(me.ID, "obmen", f.ID)); ok {
			return core.ErrBad("Вы уже голосовали")
		}
		a.T.Likes.Insert(&model.LikeObject{IDUser: me.ID, IDObject: f.ID, Type: "obmen", Like: in.Like})
		a.AddBalls(f.IDUser, 1, 1)
		return c.OK()
	}, core.Auth)

	a.POST("/api/obmen/file/{id}/komm", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if err := banErr(a, c); err != nil {
			return err
		}
		f, ok := files.Get(c.PathInt("id"))
		if !ok || !fileOpen(a, c, f) {
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
		if a.Text.Censor(msg) != msg {
			return core.ErrBad("В тексте сообщения обнаружен мат")
		}
		if err := lenOK(msg, 2, 1024, "Сообщение"); err != nil {
			return err
		}
		if komms.Count(func(k *Komm) bool { return k.IDFile == f.ID && k.IDUser == me.ID && k.Msg == msg }) > 0 {
			return core.ErrBad("Ваше сообщение повторяет предыдущее")
		}
		id := komms.Insert(&Komm{IDFile: f.ID, IDUser: me.ID, Time: core.Now(), Msg: msg})
		a.AddBalls(me.ID, 1, 1)
		a.Discuss(f.IDUser, me.ID, core.KObmen, "obmen", f.ID)
		a.DiscussFor(me.ID, f.IDUser, "obmen", f.ID)
		if in.ReplyTo != 0 {
			a.Notify(me.ID, in.ReplyTo, "obmen_komm", f.ID, true)
		}
		return c.OK("id", id)
	}, core.Auth)

	a.DELETE("/api/obmen-komm/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		k, ok := komms.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		f, _ := files.Get(k.IDFile)
		if !(me.ID == f.IDUser || a.Can(me.ID, "obmen_komm_del")) {
			return core.ErrForbidden()
		}
		komms.Delete(k.ID)
		return c.OK()
	}, core.Auth)

	a.POST("/api/obmen-komm/{id}/spam", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		k, ok := komms.Get(c.PathInt("id"))
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
			return s.IDUser == me.ID && s.IDSpam == k.IDUser && s.Razdel == "obmen_komm" && s.Spam == k.Msg
		}) > 0 {
			return core.ErrBad("Жалоба уже отправлена")
		}
		a.ReportSpam(me.ID, k.IDUser, "obmen_komm", k.ID, k.IDFile, k.Msg, in.Msg)
		return c.OK()
	}, core.Auth)

	a.POST("/api/obmen/abuld", func(c *core.Ctx) error {
		if _, err := c.Need(); err != nil {
			return err
		}
		setCookie(c, "abuld", "1")
		return c.OK()
	}, core.Auth)
}

func registerAdmin(a *core.App) {
	const acc = "adm_set_loads"
	get := func(c *core.Ctx) error {
		if err := c.NeedAccess(acc); err != nil {
			return err
		}
		return c.JSON(map[string]any{"echo_rassh": a.Cfg.Int("echo_rassh"), "obmen_limit_up": a.Cfg.Int("obmen_limit_up"), "loads_new_file_hour": a.Cfg.Int("loads_new_file_hour")})
	}
	a.GET("/api/admin/loads/settings", get, core.Auth)
	a.PUT("/api/admin/loads/settings", func(c *core.Ctx) error {
		if err := c.NeedAccess(acc); err != nil {
			return err
		}
		var in struct {
			EchoRassh        int `json:"echo_rassh"`
			ObmenLimitUp     int `json:"obmen_limit_up"`
			LoadsNewFileHour int `json:"loads_new_file_hour"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		a.Cfg.Set("echo_rassh", itoa(min(max(in.EchoRassh, 0), 1)))
		a.Cfg.Set("obmen_limit_up", itoa(max(in.ObmenLimitUp, 0)))
		a.Cfg.Set("loads_new_file_hour", itoa(max(in.LoadsNewFileHour, 1)))
		a.AdminLog(c.UID(), "Обменник", "Настройки", "Изменены настройки загрузок")
		return c.OK()
	}, core.Auth)
}
