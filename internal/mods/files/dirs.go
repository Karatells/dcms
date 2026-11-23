package files

import (
	"sort"
	"strings"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

func dirPath(d Dir) []map[string]any {
	out := []map[string]any{}
	for n := 0; d.ID != 0 && n < 10; n++ {
		out = append([]map[string]any{{"id": d.ID, "name": d.Name}}, out...)
		if d.Parent == 0 {
			break
		}
		p, ok := dirs.Get(d.Parent)
		if !ok {
			break
		}
		d = p
	}
	return out
}

// subtree — id папки и всех вложенных.
func subtree(id int64) []int64 {
	res := []int64{id}
	for k := 0; k < len(res) && k < 500; k++ {
		for _, c := range dirs.By("parent", i(res[k])) {
			res = append(res, c.ID)
		}
	}
	return res
}

func fileRow(a *core.App, f File, d *Dir) map[string]any {
	m := map[string]any{"id": f.ID, "name": f.Name, "ras": f.Ras, "size": f.Size, "size_str": core.SizeStr(f.Size), "time": f.Time, "loads": f.KLoads, "metka": f.Metka,
		"opis": a.Text.Plain(f.Opis, 120), "komm": komms.CountBy("file", i(f.ID)), "owner": a.Ref(f.IDUser), "screen": f.Screen && !f.Metka}
	if f.Screen && !f.Metka {
		m["screen_url"] = "/api/obmen/file/" + i(f.ID) + "/screen?s=128"
	}
	if d != nil {
		m["dir"] = map[string]any{"id": d.ID, "name": d.Name}
	}
	return m
}

func registerDirs(a *core.App) {
	a.GET("/api/obmen/dir/{id}", func(c *core.Ctx) error {
		if err := banErr(a, c); err != nil {
			return err
		}
		me := c.User()
		id := c.PathInt("id")
		var cur Dir
		if id != 0 {
			d, ok := dirs.Get(id)
			if !ok || d.My && !a.Can(c.UID(), "obmen_dir_edit") {
				return core.ErrNotFound("Папка не найдена")
			}
			cur = d
		}
		manage := c.Can("obmen_dir_edit")
		sub := dirs.By("parent", i(id))
		sort.Slice(sub, func(x, y int) bool {
			if sub[x].Name != sub[y].Name {
				return sub[x].Name < sub[y].Name
			}
			return sub[x].Num < sub[y].Num
		})
		var since int64
		if me != nil {
			since = me.DateLast
		}
		my := myDir()
		list := []map[string]any{}
		for _, d := range sub {
			if d.My && !manage {
				continue
			}
			ids := map[int64]bool{}
			for _, x := range subtree(d.ID) {
				ids[x] = true
			}
			total := files.Count(func(f *File) bool { return ids[f.IDDir] })
			news := files.Count(func(f *File) bool { return ids[f.IDDir] && f.TimeGo > since && since > 0 })
			list = append(list, map[string]any{"id": d.ID, "name": d.Name, "files": total, "new": news, "upload": d.Upload, "my": d.My})
		}
		sortMode := c.Q("sort") // new | pop
		var fl []File
		if id != 0 && cur.ID != my.ID {
			fl = files.By("dir", i(id))
		} else if id != 0 && manage {
			fl = files.By("dir", i(id))
		}
		sort.Slice(fl, func(x, y int) bool {
			if sortMode == "pop" {
				return fl[x].KLoads > fl[y].KLoads
			}
			return fl[x].Time > fl[y].Time
		})
		p := c.Page()
		// единый список: сначала папки, потом файлы (как в PHP)
		nd := len(list)
		total := nd + len(fl)
		items := []map[string]any{}
		for k := p.Offset; k < total && k < p.Offset+p.Per; k++ {
			if k < nd {
				m := list[k]
				m["kind"] = "dir"
				items = append(items, m)
			} else {
				m := fileRow(a, fl[k-nd], nil)
				m["kind"] = "file"
				items = append(items, m)
			}
		}
		out := map[string]any{"id": id, "name": cur.Name, "path": dirPath(cur), "items": items, "total": total, "page": p.Page, "per": p.Per, "pages": max(1, (total+p.Per-1)/p.Per),
			"upload": cur.Upload && !cur.My, "can_create": c.Can("obmen_dir_create"), "can_edit": manage && id != 0, "can_delete": c.Can("obmen_dir_delete") && id != 0 && !cur.My,
			"sort": sortMode, "echo_ras": a.Cfg.Int("echo_rassh")}
		if id != 0 {
			out["ras"], out["maxsize"], out["maxsize_str"] = cur.Ras, cur.MaxSize, core.SizeStr(cur.MaxSize)
			out["parent"] = cur.Parent
		}
		if me != nil && cur.Upload && !cur.My {
			out["limit"] = int64(a.Cfg.Int("obmen_limit_up"))
			out["balls"] = me.Balls
		}
		return c.JSON(out)
	})

	type dirIn struct {
		Parent  int64  `json:"parent"`
		Name    string `json:"name"`
		Ras     string `json:"ras"`
		MaxSize int64  `json:"maxsize"`
		Upload  bool   `json:"upload"`
	}
	check := func(in *dirIn) error {
		in.Name = strings.TrimSpace(in.Name)
		if err := lenOK(in.Name, 1, 64, "Название"); err != nil {
			return err
		}
		if !reName.MatchString(in.Name) {
			return core.ErrBad("В названии присутствуют запрещенные символы")
		}
		in.Ras = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(in.Ras), " ", ""))
		if in.Ras == "" {
			in.Upload = false
		}
		if !in.Upload {
			in.MaxSize = 0
		}
		const lim = 1 << 30
		if in.MaxSize > lim {
			in.MaxSize = lim
		}
		return nil
	}
	a.POST("/api/obmen/dir", func(c *core.Ctx) error {
		if err := c.NeedAccess("obmen_dir_create"); err != nil {
			return err
		}
		var in dirIn
		if err := c.Bind(&in); err != nil {
			return err
		}
		if err := check(&in); err != nil {
			return err
		}
		if in.Parent != 0 {
			if p, ok := dirs.Get(in.Parent); !ok || p.My {
				return core.ErrBad("Неверная родительская папка")
			}
		}
		if dirs.Count(func(d *Dir) bool { return d.Parent == in.Parent && strings.EqualFold(d.Name, in.Name) }) > 0 {
			return core.ErrBad("Папка с таким названием уже существует")
		}
		id := dirs.Insert(&Dir{Name: a.Text.Clean(in.Name), Parent: in.Parent, Ras: in.Ras, MaxSize: in.MaxSize, Upload: in.Upload})
		a.AdminLog(c.UID(), "Обменник", "Создание папки", "Создана папка '"+in.Name+"'")
		return c.OK("id", id)
	}, core.Auth)

	a.PUT("/api/obmen/dir/{id}", func(c *core.Ctx) error {
		if err := c.NeedAccess("obmen_dir_edit"); err != nil {
			return err
		}
		d, ok := dirs.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		var in dirIn
		if err := c.Bind(&in); err != nil {
			return err
		}
		if err := check(&in); err != nil {
			return err
		}
		dirs.Update(d.ID, func(x *Dir) {
			if !x.My {
				x.Name = a.Text.Clean(in.Name)
			}
			x.Ras, x.MaxSize, x.Upload = in.Ras, in.MaxSize, in.Upload
		})
		a.AdminLog(c.UID(), "Обменник", "Изменение папки", "Изменены параметры папки '"+d.Name+"'")
		return c.OK()
	}, core.Auth)

	a.POST("/api/obmen/dir/{id}/move", func(c *core.Ctx) error {
		if err := c.NeedAccess("obmen_dir_edit"); err != nil {
			return err
		}
		d, ok := dirs.Get(c.PathInt("id"))
		if !ok || d.My {
			return core.ErrNotFound()
		}
		var in struct {
			Parent int64 `json:"parent"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		for _, x := range subtree(d.ID) {
			if x == in.Parent {
				return core.ErrBad("Нельзя переместить папку в саму себя")
			}
		}
		if in.Parent != 0 {
			if p, ok := dirs.Get(in.Parent); !ok || p.My {
				return core.ErrBad("Неверная папка назначения")
			}
		}
		dirs.Update(d.ID, func(x *Dir) { x.Parent = in.Parent })
		a.AdminLog(c.UID(), "Обменник", "Изменение папки", "Папка '"+d.Name+"' перемещена")
		return c.OK()
	}, core.Auth)

	a.DELETE("/api/obmen/dir/{id}", func(c *core.Ctx) error {
		if err := c.NeedAccess("obmen_dir_delete"); err != nil {
			return err
		}
		d, ok := dirs.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		if d.My {
			return core.ErrBad("Нельзя удалить папку «Личные файлы»")
		}
		for _, x := range subtree(d.ID) {
			for _, f := range files.By("dir", i(x)) {
				deleteFile(a, f)
			}
			dirs.Delete(x)
		}
		a.AdminLog(c.UID(), "Обменник", "Удаление папки", "Папка '"+d.Name+"' удалена")
		return c.OK("parent", d.Parent)
	}, core.Auth)

	a.GET("/api/obmen/tree", func(c *core.Ctx) error {
		out := []map[string]any{}
		var walk func(parent int64, depth int)
		walk = func(parent int64, depth int) {
			l := dirs.By("parent", i(parent))
			sort.Slice(l, func(x, y int) bool { return l[x].Name < l[y].Name })
			for _, d := range l {
				if d.My {
					continue
				}
				out = append(out, map[string]any{"id": d.ID, "name": d.Name, "depth": depth, "upload": d.Upload, "ras": d.Ras, "maxsize": d.MaxSize})
				walk(d.ID, depth+1)
			}
		}
		walk(0, 0)
		return c.JSON(out)
	})

	a.GET("/api/obmen/search", func(c *core.Ctx) error {
		if err := banErr(a, c); err != nil {
			return err
		}
		q := strings.ToLower(strings.TrimSpace(c.Q("q")))
		if len([]rune(q)) < 2 {
			return core.ErrBad("Слишком короткий запрос")
		}
		my := myDir()
		list, total := files.Select(store.Opts[File]{Desc: true, Offset: c.Page().Offset, Limit: c.Page().Per, Less: func(x, y *File) bool { return x.Time < y.Time },
			Where: func(f *File) bool {
				return f.IDDir != my.ID && (strings.Contains(strings.ToLower(f.Name), q) || strings.Contains(strings.ToLower(f.Opis), q))
			}})
		items := make([]map[string]any, 0, len(list))
		for _, f := range list {
			d, _ := dirs.Get(f.IDDir)
			items = append(items, fileRow(a, f, &d))
		}
		return c.List(items, total, c.Page())
	})
}

var _ = model.User{}
