package foto

import (
	"strings"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

func extOf(format string) string {
	switch format {
	case "jpeg":
		return "jpg"
	case "png", "gif":
		return format
	}
	return ""
}

// savePhoto — проверка и сохранение загруженного изображения.
func savePhoto(a *core.App, c *core.Ctx, g Gallery, name, opis string, metka bool, makeAvatar bool) (Foto, error) {
	maxMB := int64(a.Cfg.Int("max_upload_foto_mb"))
	if maxMB <= 0 {
		maxMB = 12
	}
	up, err := c.FormFile("file", maxMB<<20)
	if err != nil {
		return Foto{}, err
	}
	if up == nil {
		return Foto{}, core.ErrBad("Файл не выбран")
	}
	w, h, format, err := core.ImageInfo(up.Data)
	ext := extOf(format)
	if err != nil || ext == "" {
		return Foto{}, core.ErrBad("Файл не является изображением (JPEG, PNG, GIF)")
	}
	if w > a.Cfg.Int("max_upload_foto_x") || h > a.Cfg.Int("max_upload_foto_y") {
		return Foto{}, core.ErrBad("Размер изображения превышает ограничения в " + itoa(a.Cfg.Int("max_upload_foto_x")) + "*" + itoa(a.Cfg.Int("max_upload_foto_y")))
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = strings.TrimSuffix(up.Name, "."+up.Ext)
		if r := []rune(name); len(r) > 32 {
			name = string(r[:32])
		}
	}
	if err := lenOK(name, 3, 32, "Название"); err != nil {
		if len([]rune(name)) < 3 {
			name += "..."[:3-len([]rune(name))]
		} else {
			return Foto{}, err
		}
	}
	if !reName.MatchString(name) {
		return Foto{}, core.ErrBad("В названии фото присутствуют запрещенные символы")
	}
	if err := lenOK(opis, 0, 1024, "Описание"); err != nil {
		return Foto{}, err
	}
	me := c.User()
	now := core.Now()
	f := Foto{IDGallery: g.ID, IDUser: me.ID, Name: a.Text.Clean(name), Ras: ext, Type: up.MIME, Opis: strings.TrimSpace(opis), Time: now, Metka: metka, W: int64(w), H: int64(h)}
	f.ID = fotos.Insert(&f)
	if err := core.WriteFile(fotoPath(a, f.ID, ext), up.Data); err != nil {
		fotos.Delete(f.ID)
		return Foto{}, core.ErrBad("Не удалось сохранить файл")
	}
	galleries.Update(g.ID, func(x *Gallery) { x.Time = now })
	if makeAvatar {
		setAvatar(a, me.ID, f)
		f.Avatar = true
	} else {
		// лента: «новые фото в альбоме»
		n := a.T.Tape.UpdateWhere(func(t *model.Tape) bool { return t.Type == "album" && t.IDFile == g.ID && t.Avtor == g.IDUser },
			func(t *model.Tape) { t.Count++; t.Read = false; t.Time = now })
		if n == 0 {
			a.FanTape(g.IDUser, core.KFoto, "album", g.ID, core.TapeOpts{})
		}
	}
	return f, nil
}

func setAvatar(a *core.App, uid int64, f Foto) {
	var old int64
	if u, ok := a.GetUser(uid); ok {
		old = u.AvatarID
	}
	for _, x := range fotos.By("user", i(uid)) {
		if x.Avatar && x.ID != f.ID {
			fotos.Update(x.ID, func(y *Foto) { y.Avatar = false })
		}
	}
	fotos.Update(f.ID, func(y *Foto) { y.Avatar = true })
	a.T.Users.Update(uid, func(u *model.User) { u.AvatarID, u.AvatarExt = f.ID, f.Ras })
	a.ForgetUser(uid)
	if old != f.ID {
		a.FanTape(uid, core.KAvatar, "avatar", f.ID, core.TapeOpts{Avatar: old})
		profileWall(uid, f.ID)
	}
}

func canKomm(a *core.App, me *model.User, g Gallery) bool {
	if me == nil {
		return false
	}
	if me.ID == g.IDUser {
		return true
	}
	switch g.PrivatKomm {
	case 1:
		return a.IsFriend(me.ID, g.IDUser)
	case 2:
		return false
	}
	return true
}

func registerPhotos(a *core.App) {
	a.POST("/api/foto/album/{id}/upload", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if err := banCheck(a, c); err != nil {
			return err
		}
		g, ok := galleries.Get(c.PathInt("id"))
		if !ok || g.IDUser != me.ID {
			return core.ErrForbidden("Загружать фото можно только в свои альбомы")
		}
		f, err := savePhoto(a, c, g, c.FormValue("name"), c.FormValue("opis"), c.FormValue("metka") == "1", c.FormValue("avatar") == "1")
		if err != nil {
			return err
		}
		return c.OK("id", f.ID, "album", g.ID)
	}, core.Auth)

	a.GET("/api/foto/photo/{id}", func(c *core.Ctx) error {
		if err := banCheck(a, c); err != nil {
			return err
		}
		f, ok := fotos.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Фото не найдено")
		}
		g, _ := galleries.Get(f.IDGallery)
		me := c.User()
		out := map[string]any{"id": f.ID, "name": f.Name, "gallery": map[string]any{"id": g.ID, "name": g.Name}, "owner": a.Ref(f.IDUser), "time": f.Time, "metka": f.Metka}
		acc := galleryAccess(a, c, g)
		if !acc.ok && !f.Avatar {
			out["blocked"] = acc.reason
			return c.JSON(out)
		}
		out["can_edit"] = me != nil && (me.ID == f.IDUser || (a.Can(me.ID, "foto_foto_edit") && me.Level > a.Ref(f.IDUser).Level))
		if !adultOK(c, f) {
			out["adult"] = true
			out["need_login"] = me == nil
			return c.JSON(out)
		}
		out["src"], out["src_big"], out["src_orig"] = imgURL(f, 640), imgURL(f, 640), imgURL(f, 0)
		out["w"], out["h"] = f.W, f.H
		out["opis"], out["raw_opis"] = a.Text.Render(f.Opis), f.Opis
		out["avatar"] = f.Avatar
		out["rating"] = f.Rating
		rs := ratings.By("foto", i(f.ID))
		plus := 0
		for _, r := range rs {
			if r.Like == 6 {
				plus++
			}
		}
		out["votes"], out["plus5"] = len(rs), plus
		if len(rs) > 0 {
			out["avg"] = float64(f.Rating) / float64(len(rs))
		}
		if me != nil {
			out["can_rate"] = me.ID != f.IDUser
			if r, ok := ratings.OneBy("uf", store.Key(me.ID, f.ID)); ok {
				out["my_rating"] = r.Like
			}
			_, fav := a.T.Bookmarks.OneBy("uobj", store.Key(me.ID, "foto", f.ID))
			out["bookmarked"] = fav
			a.T.Notifications.UpdateWhere(func(n *model.Notification) bool {
				return n.IDUser == me.ID && n.Type == "foto_komm" && n.IDObject == f.ID && n.Read == 0
			},
				func(n *model.Notification) { n.Read = 1 })
			for _, d := range a.T.Discussions.By("user", i(me.ID)) {
				if d.Type == "foto" && d.IDSim == f.ID && d.Count > 0 {
					a.T.Discussions.Update(d.ID, func(x *model.Discussions) { x.Count = 0 })
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
		out["bookmarks"] = a.T.Bookmarks.Count(func(b *model.Bookmarks) bool { return b.Type == "foto" && b.IDObject == f.ID })
		// соседние
		if prev, _ := fotos.Select(store.Opts[Foto]{Index: "gallery", Key: i(g.ID), Desc: true, Limit: 1, Where: func(x *Foto) bool { return x.ID < f.ID }}); len(prev) > 0 {
			out["older"] = prev[0].ID
		}
		if nxt, _ := fotos.Select(store.Opts[Foto]{Index: "gallery", Key: i(g.ID), Limit: 1, Where: func(x *Foto) bool { return x.ID > f.ID }}); len(nxt) > 0 {
			out["newer"] = nxt[0].ID
		}
		// комментарии
		p := c.Page()
		desc := !(me != nil && me.Sort == 1)
		list, total := komms.Select(store.Opts[Komm]{Index: "foto", Key: i(f.ID), Desc: desc, Offset: p.Offset, Limit: p.Per})
		ks := make([]map[string]any, 0, len(list))
		for _, k := range list {
			m := map[string]any{"id": k.ID, "msg": a.Text.Render(k.Msg), "time": k.Time, "user": a.Ref(k.IDUser)}
			if me != nil {
				m["can_reply"] = k.IDUser != me.ID
				m["can_delete"] = me.ID == g.IDUser || a.Can(me.ID, "foto_komm_del")
			}
			ks = append(ks, m)
		}
		out["can_komm"] = canKomm(a, me, g)
		out["komms"] = map[string]any{"items": ks, "total": total, "page": p.Page, "per": p.Per, "pages": max(1, (total+p.Per-1)/p.Per)}
		out["sort"] = 0
		if me != nil {
			out["sort"] = me.Sort
		}
		return c.JSON(out)
	})

	a.POST("/api/foto/abuld", func(c *core.Ctx) error {
		if _, err := c.Need(); err != nil {
			return err
		}
		setSecureCookie(c, "abuld", "1")
		return c.OK()
	}, core.Auth)

	a.PUT("/api/foto/photo/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		f, ok := fotos.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		if !(me.ID == f.IDUser || (a.Can(me.ID, "foto_foto_edit") && me.Level > a.Ref(f.IDUser).Level)) {
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
		if err := lenOK(in.Name, 3, 32, "Название"); err != nil {
			return err
		}
		if !reName.MatchString(in.Name) {
			return core.ErrBad("В названии фото присутствуют запрещенные символы")
		}
		if err := lenOK(in.Opis, 0, 1024, "Описание"); err != nil {
			return err
		}
		fotos.Update(f.ID, func(x *Foto) { x.Name, x.Opis, x.Metka = a.Text.Clean(in.Name), in.Opis, in.Metka })
		if me.ID != f.IDUser {
			a.AdminLog(me.ID, "Фотогалерея", "Фотографии", "Переименование фото пользователя '[url=/id"+i(f.IDUser)+"]"+a.Ref(f.IDUser).Nick+"[/url]'")
		}
		return c.OK()
	}, core.Auth)

	a.DELETE("/api/foto/photo/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		f, ok := fotos.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		if !(me.ID == f.IDUser || (a.Can(me.ID, "foto_foto_edit") && me.Level > a.Ref(f.IDUser).Level)) {
			return core.ErrForbidden()
		}
		deleteFoto(a, f)
		if me.ID != f.IDUser {
			a.AdminLog(me.ID, "Фотогалерея", "Фотографии", "Удаление фото пользователя '[url=/id"+i(f.IDUser)+"]"+a.Ref(f.IDUser).Nick+"[/url]'")
		}
		return c.OK("album", f.IDGallery, "owner", f.IDUser)
	}, core.Auth)

	a.POST("/api/foto/photo/{id}/avatar", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		f, ok := fotos.Get(c.PathInt("id"))
		if !ok || f.IDUser != me.ID {
			return core.ErrForbidden()
		}
		setAvatar(a, me.ID, f)
		return c.OK()
	}, core.Auth)

	a.POST("/api/foto/photo/{id}/rating", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		f, ok := fotos.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		g, _ := galleries.Get(f.IDGallery)
		if !galleryAccess(a, c, g).ok {
			return core.ErrForbidden()
		}
		var in struct {
			Rating int64 `json:"rating"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if in.Rating < 1 || in.Rating > 6 {
			return core.ErrBad("Неверная оценка")
		}
		if me.ID == f.IDUser {
			return core.ErrBad("Нельзя оценивать свои фото")
		}
		if _, ok := ratings.OneBy("uf", store.Key(me.ID, f.ID)); ok {
			return core.ErrBad("Вы уже оценили это фото")
		}
		if in.Rating == 6 && a.UserSettings(me.ID).Ocenka <= core.Now() {
			return core.ErrBad("Необходимо активировать услугу «Оценка 5+»")
		}
		ratings.Insert(&Rating{IDFoto: f.ID, IDUser: me.ID, Like: in.Rating, Avtor: f.IDUser, Time: core.Now()})
		fotos.Update(f.ID, func(x *Foto) { x.Rating += in.Rating })
		return c.OK()
	}, core.Auth)
}
