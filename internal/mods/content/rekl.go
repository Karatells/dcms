package content

import (
	"net/url"
	"sort"
	"strings"

	"dcms/internal/core"
)

var reklNames = map[int64]string{1: "Верх сайта", 2: "Под шапкой (код)", 3: "Низ сайта (главная)", 4: "Низ сайта (остальные)"}

func reklItem(r Rekl, admin bool) map[string]any {
	link := r.Link
	if r.DopStr {
		link = "/go/" + i(r.ID)
	}
	m := map[string]any{"id": r.ID, "name": r.Name, "img": r.Img, "link": link}
	if admin {
		m["real_link"], m["time_last"], m["count"], m["dop_str"], m["sel"], m["active"] = r.Link, r.TimeLast, r.Count, r.DopStr, r.Sel, r.TimeLast > core.Now()
	}
	return m
}

func registerRekl(a *core.App) {
	// активная реклама по позициям
	a.GET("/api/rekl/show", func(c *core.Ctx) error {
		out := map[string][]map[string]any{"1": {}, "2": {}, "3": {}, "4": {}}
		now := core.Now()
		l := rekls.Find(func(r *Rekl) bool { return r.TimeLast > now })
		sort.Slice(l, func(x, y int) bool { return l[x].ID < l[y].ID })
		for _, r := range l {
			k := i(r.Sel)
			out[k] = append(out[k], reklItem(r, false))
		}
		return c.JSON(out)
	})
	// переход по рекламной ссылке со счётчиком (go.php?go=ID)
	a.GET("/api/go/{id}", func(c *core.Ctx) error {
		r, ok := rekls.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		rekls.Update(r.ID, func(x *Rekl) { x.Count++ })
		return c.JSON(map[string]any{"url": r.Link, "name": r.Name})
	})

	adm := func(c *core.Ctx) error { return c.NeedAccess("adm_rekl") }
	a.GET("/api/admin/rekl", func(c *core.Ctx) error {
		if err := adm(c); err != nil {
			return err
		}
		sel := c.QInt("sel")
		if sel == 0 {
			var names []map[string]any
			for k := int64(3); k <= 4; k++ {
				names = append(names, map[string]any{"sel": k, "name": reklNames[k], "count": rekls.Count(func(r *Rekl) bool { return r.Sel == k })})
			}
			return c.JSON(map[string]any{"places": names})
		}
		if sel < 1 || sel > 4 {
			return core.ErrBad("Неверная позиция")
		}
		p := c.Page()
		l := rekls.Find(func(r *Rekl) bool { return r.Sel == sel })
		sort.Slice(l, func(x, y int) bool { return l[x].TimeLast > l[y].TimeLast })
		total := len(l)
		l = l[min(p.Offset, total):min(p.Offset+p.Per, total)]
		items := make([]map[string]any, 0, len(l))
		for _, r := range l {
			items = append(items, reklItem(r, true))
		}
		return c.JSON(map[string]any{"items": items, "total": total, "page": p.Page, "per": p.Per, "pages": max(1, (total+p.Per-1)/p.Per), "name": reklNames[sel]})
	}, core.Auth)

	type in struct {
		Name   string `json:"name"`
		Link   string `json:"link"`
		Img    string `json:"img"`
		Ch     int64  `json:"ch"`
		Mn     int64  `json:"mn"`
		DopStr bool   `json:"dop_str"`
		Sel    int64  `json:"sel"`
	}
	check := func(v *in) error {
		v.Name, v.Link, v.Img = strings.TrimSpace(v.Name), strings.TrimSpace(v.Link), strings.TrimSpace(v.Img)
		if v.Name == "" {
			return core.ErrBad("Введите название")
		}
		if err := lenOK(v.Name, 1, 32, "Название"); err != nil {
			return err
		}
		if err := lenOK(v.Link, 1, 64, "Ссылка"); err != nil {
			return err
		}
		if len(v.Img) > 64 {
			return core.ErrBad("Адрес картинки слишком длинный")
		}
		if u, err := url.Parse(v.Link); err != nil || (u.Scheme != "" && u.Scheme != "http" && u.Scheme != "https") {
			return core.ErrBad("Неверная ссылка")
		}
		if v.Mn != 1 && v.Mn != 7 && v.Mn != 31 {
			v.Mn = 1
		}
		return nil
	}
	a.POST("/api/admin/rekl", func(c *core.Ctx) error {
		if err := adm(c); err != nil {
			return err
		}
		var v in
		if err := c.Bind(&v); err != nil {
			return err
		}
		if v.Sel < 1 || v.Sel > 4 {
			return core.ErrBad("Неверная позиция")
		}
		if err := check(&v); err != nil {
			return err
		}
		id := rekls.Insert(&Rekl{Name: v.Name, Link: v.Link, Img: v.Img, Sel: v.Sel, DopStr: v.DopStr, TimeLast: core.Now() + v.Ch*v.Mn*86400})
		a.AdminLog(c.UID(), "Реклама", "Добавление", v.Name)
		return c.OK("id", id)
	}, core.Auth)
	a.PUT("/api/admin/rekl/{id}", func(c *core.Ctx) error {
		if err := adm(c); err != nil {
			return err
		}
		r, ok := rekls.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		var v in
		if err := c.Bind(&v); err != nil {
			return err
		}
		if err := check(&v); err != nil {
			return err
		}
		base := core.Now()
		if r.TimeLast > base {
			base = r.TimeLast
		}
		rekls.Update(r.ID, func(x *Rekl) {
			x.Name, x.Link, x.Img, x.DopStr, x.TimeLast = v.Name, v.Link, v.Img, v.DopStr, base+v.Ch*v.Mn*86400
		})
		return c.OK()
	}, core.Auth)
	a.DELETE("/api/admin/rekl/{id}", func(c *core.Ctx) error {
		if err := adm(c); err != nil {
			return err
		}
		if !rekls.Delete(c.PathInt("id")) {
			return core.ErrNotFound()
		}
		return c.OK()
	}, core.Auth)
}
