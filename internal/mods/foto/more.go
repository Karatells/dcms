package foto

import (
	"fmt"
	"strings"

	"dcms/internal/core"
	"dcms/internal/mods/profile"
	"dcms/internal/store"
)

func profileWall(uid, fotoID int64) {
	profile.PostWall(uid, fmt.Sprintf("новый аватар: [url=/api/foto/img/%d.jpg]фото[/url]", fotoID))
}

func registerImages(a *core.App) {
	allowed := map[int]bool{48: true, 50: true, 64: true, 100: true, 128: true, 160: true, 200: true, 320: true, 640: true, 1024: true}
	a.GET("/api/foto/img/{file}", func(c *core.Ctx) error {
		name := c.R.PathValue("file")
		dot := strings.IndexByte(name, '.')
		if dot < 1 {
			return core.ErrNotFound()
		}
		var id int64
		if _, err := fmt.Sscanf(name[:dot], "%d", &id); err != nil {
			return core.ErrNotFound()
		}
		f, ok := fotos.Get(id)
		if !ok {
			return core.ErrNotFound()
		}
		if !f.Avatar {
			g, _ := galleries.Get(f.IDGallery)
			if !galleryAccess(a, c, g).ok {
				return core.ErrForbidden()
			}
		}
		orig := fotoPath(a, f.ID, f.Ras)
		s := int(c.QInt("s"))
		if s <= 0 || (f.W > 0 && int(f.W) <= s && int(f.H) <= s && s > 200) {
			c.W.Header().Set("Cache-Control", "private, max-age=86400")
			if c.Q("dl") == "1" {
				return c.ServeFile(orig, f.Name+"."+f.Ras, f.Type)
			}
			return c.ServeFile(orig, "", f.Type)
		}
		if !allowed[s] {
			// ближайший допустимый размер
			best := 640
			for k := range allowed {
				if k >= s && k < best {
					best = k
				}
			}
			s = best
		}
		cache := fmt.Sprintf("%s/%d_%d.jpg", a.UploadDir("foto", "cache"), f.ID, s)
		c.W.Header().Set("Cache-Control", "public, max-age=604800")
		if _, err := osStat(cache); err == nil {
			return c.ServeFile(cache, "", "image/jpeg")
		}
		thumbMu.Lock()
		defer thumbMu.Unlock()
		if _, err := osStat(cache); err == nil {
			return c.ServeFile(cache, "", "image/jpeg")
		}
		data, err := osRead(orig)
		if err != nil {
			return core.ErrNotFound()
		}
		th, mime, err := core.Thumb(data, s, s, s <= 200)
		if err != nil {
			return c.ServeFile(orig, "", f.Type)
		}
		_ = core.WriteFile(cache, th)
		return c.ServeBytes(th, mime, true)
	})
}

func registerAvatar(a *core.App) {
	// быстрая загрузка аватара: фото попадает в системный альбом «Фото со страницы»
	a.POST("/api/foto/avatar", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if err := banCheck(a, c); err != nil {
			return err
		}
		g, ok := galleries.Find(func(g *Gallery) bool { return g.IDUser == me.ID && g.My }), false
		_ = ok
		var gal Gallery
		if len(g) > 0 {
			gal = g[0]
		} else {
			now := core.Now()
			gal = Gallery{IDUser: me.ID, Name: "Фото со страницы", TimeCreate: now, Time: now, My: true}
			gal.ID = galleries.Insert(&gal)
		}
		f, err := savePhoto(a, c, gal, c.FormValue("name"), "", false, true)
		if err != nil {
			return err
		}
		return c.OK("id", f.ID)
	}, core.Auth)

	a.DELETE("/api/foto/avatar", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		for _, f := range fotos.By("user", i(me.ID)) {
			if f.Avatar {
				fotos.Update(f.ID, func(x *Foto) { x.Avatar = false })
			}
		}
		a.T.Users.Update(me.ID, func(u *modelUser) { u.AvatarID, u.AvatarExt = 0, "" })
		a.ForgetUser(me.ID)
		return c.OK()
	}, core.Auth)

	// комментарии
	a.POST("/api/foto/photo/{id}/komm", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if err := banCheck(a, c); err != nil {
			return err
		}
		f, ok := fotos.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		g, _ := galleries.Get(f.IDGallery)
		if !galleryAccess(a, c, g).ok || !canKomm(a, me, g) {
			return core.ErrForbidden("Вам запрещено комментировать эту фотографию")
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
		if komms.Count(func(k *Komm) bool { return k.IDFoto == f.ID && k.IDUser == me.ID && k.Msg == msg }) > 0 {
			return core.ErrBad("Ваше сообщение повторяет предыдущее")
		}
		id := komms.Insert(&Komm{IDFoto: f.ID, IDUser: me.ID, Time: core.Now(), Msg: msg})
		a.AddBalls(me.ID, 1, 0)
		a.Discuss(g.IDUser, me.ID, core.KFoto, "foto", f.ID)
		a.DiscussFor(me.ID, g.IDUser, "foto", f.ID)
		if in.ReplyTo != 0 {
			a.Notify(me.ID, in.ReplyTo, "foto_komm", f.ID, true)
		}
		return c.OK("id", id)
	}, core.Auth)

	a.DELETE("/api/foto-komm/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		k, ok := komms.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		f, _ := fotos.Get(k.IDFoto)
		g, _ := galleries.Get(f.IDGallery)
		if !(me.ID == g.IDUser || a.Can(me.ID, "foto_komm_del")) {
			return core.ErrForbidden()
		}
		komms.Delete(k.ID)
		if me.ID != g.IDUser {
			a.AdminLog(me.ID, "Фотогалерея", "Фотографии", "Удаление комментария к фото [url=/id"+i(g.IDUser)+"]"+a.Ref(g.IDUser).Nick+"[/url]")
		}
		return c.OK()
	}, core.Auth)
}

func registerAdmin(a *core.App) {
	const acc = "adm_set_foto"
	a.GET("/api/admin/foto/settings", func(c *core.Ctx) error {
		if err := c.NeedAccess(acc); err != nil {
			return err
		}
		return c.JSON(map[string]any{"max_upload_foto_x": a.Cfg.Int("max_upload_foto_x"), "max_upload_foto_y": a.Cfg.Int("max_upload_foto_y")})
	}, core.Auth)
	a.PUT("/api/admin/foto/settings", func(c *core.Ctx) error {
		if err := c.NeedAccess(acc); err != nil {
			return err
		}
		var in struct {
			X int `json:"max_upload_foto_x"`
			Y int `json:"max_upload_foto_y"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if in.X < 16 || in.Y < 16 || in.X > 20000 || in.Y > 20000 {
			return core.ErrBad("Недопустимые размеры")
		}
		a.Cfg.Set("max_upload_foto_x", itoa(in.X))
		a.Cfg.Set("max_upload_foto_y", itoa(in.Y))
		a.AdminLog(c.UID(), "Фотогалерея", "Настройки", "Изменены ограничения размера загружаемых фото")
		return c.OK()
	}, core.Auth)
}

var _ = store.I
