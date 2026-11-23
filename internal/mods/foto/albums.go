package foto

import (
	"strings"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

func banCheck(a *core.App, c *core.Ctx) error {
	if me := c.User(); me != nil {
		if b, ok := a.ActiveBan(me.ID, "foto"); ok {
			msg := "Вам запрещён доступ в фотоальбомы"
			if b.Prich != "" {
				msg += ". Причина: " + b.Prich
			}
			return core.ErrForbidden(msg)
		}
	}
	return nil
}

func albumCard(a *core.App, c *core.Ctx, g Gallery) map[string]any {
	m := map[string]any{"id": g.ID, "name": g.Name, "opis": a.Text.Render(g.Opis), "time": g.Time, "owner": a.Ref(g.IDUser),
		"count": fotos.CountBy("gallery", i(g.ID)), "locked": g.Pass != "" || g.Privat != 0, "privat": g.Privat}
	if cf, ok := cover(g); ok && galleryAccess(a, c, g).ok && !cf.Metka {
		m["cover"] = imgURL(cf, 200)
	}
	return m
}

type albumIn struct {
	Name       string `json:"name"`
	Opis       string `json:"opis"`
	Pass       string `json:"pass"`
	Privat     int64  `json:"privat"`
	PrivatKomm int64  `json:"privat_komm"`
}

func (in *albumIn) check(a *core.App, owner int64, self int64) error {
	in.Name, in.Opis, in.Pass = strings.TrimSpace(in.Name), strings.TrimSpace(in.Opis), strings.TrimSpace(in.Pass)
	if err := lenOK(in.Name, 3, 32, "Название"); err != nil {
		return err
	}
	if err := lenOK(in.Opis, 0, 256, "Описание"); err != nil {
		return err
	}
	if utf8Len(in.Pass) > 11 {
		return core.ErrBad("Пароль не длиннее 11 символов")
	}
	if in.Privat < 0 || in.Privat > 2 || in.PrivatKomm < 0 || in.PrivatKomm > 2 {
		return core.ErrBad("Неверные параметры приватности")
	}
	in.Name = a.Text.Clean(in.Name)
	if galleries.Count(func(g *Gallery) bool { return g.IDUser == owner && g.ID != self && g.Name == in.Name }) > 0 {
		return core.ErrBad("Альбом с таким названием уже существует")
	}
	return nil
}

func utf8Len(s string) int { return len([]rune(s)) }

func registerAlbums(a *core.App) {
	a.GET("/api/foto", func(c *core.Ctx) error {
		if err := banCheck(a, c); err != nil {
			return err
		}
		p := c.Page()
		list, total := galleries.Select(store.Opts[Gallery]{Desc: true, Offset: p.Offset, Limit: p.Per, Less: func(x, y *Gallery) bool { return x.Time < y.Time }})
		items := make([]map[string]any, 0, len(list))
		for _, g := range list {
			items = append(items, albumCard(a, c, g))
		}
		return c.JSON(map[string]any{"items": items, "total": total, "page": p.Page, "per": p.Per, "pages": max(1, (total+p.Per-1)/p.Per), "photos": fotos.Len()})
	})

	a.GET("/api/foto/user/{uid}", func(c *core.Ctx) error {
		if err := banCheck(a, c); err != nil {
			return err
		}
		u, ok := a.GetUser(c.PathInt("uid"))
		if !ok {
			return core.ErrNotFound("Пользователь не найден")
		}
		p := c.Page()
		list, total := galleries.Select(store.Opts[Gallery]{Index: "user", Key: i(u.ID), Desc: true, Offset: p.Offset, Limit: p.Per, Less: func(x, y *Gallery) bool { return x.Time < y.Time }})
		items := make([]map[string]any, 0, len(list))
		me := c.User()
		for _, g := range list {
			m := albumCard(a, c, g)
			m["can_edit"] = canManage(a, me, g, "foto_alb_del")
			items = append(items, m)
		}
		return c.JSON(map[string]any{"owner": a.Ref(u.ID), "items": items, "total": total, "page": p.Page, "per": p.Per, "pages": max(1, (total+p.Per-1)/p.Per),
			"mine": me != nil && me.ID == u.ID, "photos": fotos.CountBy("user", i(u.ID))})
	})

	a.POST("/api/foto/album", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if err := banCheck(a, c); err != nil {
			return err
		}
		var in albumIn
		if err := c.Bind(&in); err != nil {
			return err
		}
		if err := in.check(a, me.ID, 0); err != nil {
			return err
		}
		now := core.Now()
		id := galleries.Insert(&Gallery{IDUser: me.ID, Name: in.Name, Opis: in.Opis, Pass: in.Pass, Privat: in.Privat, PrivatKomm: in.PrivatKomm, TimeCreate: now, Time: now})
		return c.OK("id", id, "owner", me.ID)
	}, core.Auth)

	a.GET("/api/foto/album/{id}", func(c *core.Ctx) error {
		if err := banCheck(a, c); err != nil {
			return err
		}
		g, ok := galleries.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Альбом не найден")
		}
		me := c.User()
		out := map[string]any{"id": g.ID, "name": g.Name, "owner": a.Ref(g.IDUser), "opis": a.Text.Render(g.Opis), "has_pass": g.Pass != "", "privat": g.Privat,
			"can_edit": canManage(a, me, g, "foto_alb_del"), "mine": me != nil && me.ID == g.IDUser, "time": g.Time}
		if me != nil && (me.ID == g.IDUser || out["can_edit"] == true) {
			out["pass"], out["privat_komm"], out["raw_opis"] = g.Pass, g.PrivatKomm, g.Opis
		}
		acc := galleryAccess(a, c, g)
		if !acc.ok {
			out["blocked"] = acc.reason
			return c.JSON(out)
		}
		p := c.Page()
		list, total := fotos.Select(store.Opts[Foto]{Index: "gallery", Key: i(g.ID), Desc: true, Offset: p.Offset, Limit: p.Per})
		items := make([]map[string]any, 0, len(list))
		for _, f := range list {
			items = append(items, map[string]any{"id": f.ID, "name": f.Name, "opis": a.Text.Plain(f.Opis, 120), "metka": f.Metka, "src": imgURL(f, 200), "rating": f.Rating, "komm": komms.CountBy("foto", i(f.ID)), "avatar": f.Avatar})
		}
		out["items"], out["total"], out["page"], out["per"], out["pages"] = items, total, p.Page, p.Per, max(1, (total+p.Per-1)/p.Per)
		return c.JSON(out)
	})

	a.POST("/api/foto/album/{id}/unlock", func(c *core.Ctx) error {
		g, ok := galleries.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		var in struct {
			Password string `json:"password"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if in.Password != g.Pass {
			return core.ErrBad("Неверный пароль")
		}
		setSecureCookie(c, "fp"+i(g.ID), passToken(g))
		return c.OK()
	})

	a.PUT("/api/foto/album/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		g, ok := galleries.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		if !canManage(a, me, g, "foto_alb_del") {
			return core.ErrForbidden()
		}
		var in albumIn
		if err := c.Bind(&in); err != nil {
			return err
		}
		if err := in.check(a, g.IDUser, g.ID); err != nil {
			return err
		}
		galleries.Update(g.ID, func(x *Gallery) {
			x.Name, x.Opis, x.Pass, x.Privat, x.PrivatKomm = in.Name, in.Opis, in.Pass, in.Privat, in.PrivatKomm
		})
		if me.ID != g.IDUser {
			a.AdminLog(me.ID, "Фотогалерея", "Фотоальбомы", "Изменение альбома "+g.Name)
		}
		return c.OK()
	}, core.Auth)

	a.DELETE("/api/foto/album/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		g, ok := galleries.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		if !canManage(a, me, g, "foto_alb_del") {
			return core.ErrForbidden()
		}
		n := fotos.CountBy("gallery", i(g.ID))
		deleteGallery(a, g)
		if me.ID != g.IDUser {
			a.AdminLog(me.ID, "Фотогалерея", "Фотоальбомы", "Удаление альбома "+g.Name+" (фотографий: "+itoa(n)+")")
		}
		return c.OK("owner", g.IDUser)
	}, core.Auth)
}

var _ = model.User{}
