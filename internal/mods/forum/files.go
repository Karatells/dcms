package forum

import (
	"os"
	"strings"

	"dcms/internal/core"
	"dcms/internal/store"
)

func registerFiles(a *core.App) {
	a.GET("/api/forum-file/{id}", func(c *core.Ctx) error {
		f, ok := ffiles.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Файл не найден")
		}
		p, _ := posts.Get(f.IDPost)
		if fr, ok := forums.Get(p.IDForum); ok && !visibleForum(fr, c.User()) {
			return core.ErrNotFound("Файл не найден")
		}
		path := filePath(a, f.ID)
		img := f.Ras == "jpg" || f.Ras == "jpeg" || f.Ras == "png" || f.Ras == "gif"
		if img && c.Q("s") != "" {
			size := int(c.QInt("s"))
			if size < 16 || size > 800 {
				size = 200
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return core.ErrNotFound()
			}
			th, mime, err := core.Thumb(data, size, size, false)
			if err != nil {
				return core.ErrNotFound()
			}
			return c.ServeBytes(th, mime, true)
		}
		if img && c.Q("inline") == "1" {
			return c.ServeFile(path, "", f.Type)
		}
		ffiles.Update(f.ID, func(x *FFile) { x.Count++ })
		return c.ServeFile(path, f.Name+"."+f.Ras, "application/octet-stream")
	})

	a.DELETE("/api/forum-file/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		f, ok := ffiles.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		if me.Level < 1 {
			return core.ErrForbidden()
		}
		removeFile(a, f.ID)
		a.AdminLog(me.ID, "Форум", "Файлы", "Удаление вложения '"+f.Name+"."+f.Ras+"'")
		return c.OK()
	}, core.Auth)

	a.POST("/api/forum-file/{id}/rating", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		f, ok := ffiles.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		if me.Balls < 50 || me.Rating < 0 {
			return core.ErrForbidden("Для оценки нужно минимум 50 баллов и неотрицательный рейтинг")
		}
		for _, r := range frating.By("file", i(f.ID)) {
			if r.IDUser == me.ID {
				return core.ErrBad("Вы уже голосовали")
			}
		}
		var in struct {
			Rating string `json:"rating"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		v := int64(1)
		if in.Rating == "down" {
			v = -1
		}
		frating.Insert(&FileRating{IDFile: f.ID, IDUser: me.ID, Rating: v})
		return c.OK()
	}, core.Auth)

	// список всех вложений форума (files_list.php)
	a.GET("/api/forum-files", func(c *core.Ctx) error {
		type row struct {
			f      FFile
			rating int64
			votes  int
		}
		var rows []row
		for _, f := range ffiles.All() {
			p, ok := posts.Get(f.IDPost)
			if !ok {
				continue
			}
			if fr, ok := forums.Get(p.IDForum); ok && !visibleForum(fr, c.User()) {
				continue
			}
			r := row{f: f}
			for _, x := range frating.By("file", i(f.ID)) {
				r.rating += x.Rating
				r.votes++
			}
			rows = append(rows, r)
		}
		for x := 1; x < len(rows); x++ {
			for y := x; y > 0 && rows[y].f.ID > rows[y-1].f.ID; y-- {
				rows[y], rows[y-1] = rows[y-1], rows[y]
			}
		}
		p := c.Page()
		items := []map[string]any{}
		for x := p.Offset; x < min(p.Offset+p.Per, len(rows)); x++ {
			r := rows[x]
			pp, _ := posts.Get(r.f.IDPost)
			t, _ := thems.Get(pp.IDThem)
			items = append(items, map[string]any{"id": r.f.ID, "name": r.f.Name, "ext": r.f.Ras, "size": core.SizeStr(r.f.Size), "count": r.f.Count,
				"rating": r.rating, "votes": r.votes, "url": "/api/forum-file/" + i(r.f.ID), "theme": map[string]any{"id": t.ID, "name": t.Name, "link": themeLink(t) + "?post=" + i(pp.ID)},
				"user": a.Ref(pp.IDUser), "image": strings.Contains(r.f.Type, "image/")})
		}
		return c.List(items, len(rows), p)
	})
}

var _ = store.I
