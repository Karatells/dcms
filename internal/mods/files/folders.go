package files

import (
	"sort"
	"strings"

	"dcms/internal/core"
	"dcms/internal/model"
)

// rootFolder — корневая папка пользователя (создаётся при первом обращении).
func rootFolder(uid int64) Folder {
	for _, f := range folders.By("user", i(uid)) {
		if f.Osn == 1 {
			return f
		}
	}
	f := Folder{IDUser: uid, Name: "Файлы", Osn: 1, Time: core.Now()}
	f.ID = folders.Insert(&f)
	return f
}

func folderPath(f Folder) []map[string]any {
	out := []map[string]any{}
	for n := 0; n < 8; n++ {
		out = append([]map[string]any{{"id": f.ID, "name": f.Name}}, out...)
		if f.Parent == 0 {
			break
		}
		p, ok := folders.Get(f.Parent)
		if !ok {
			break
		}
		f = p
	}
	return out
}

func folderSubtree(id int64) []int64 {
	res := []int64{id}
	for k := 0; k < len(res) && k < 1000; k++ {
		for _, c := range folders.By("parent", i(res[k])) {
			res = append(res, c.ID)
		}
	}
	return res
}

func registerFolders(a *core.App) {
	a.GET("/api/pfiles/{uid}/{dir}", func(c *core.Ctx) error {
		if err := banErr(a, c); err != nil {
			return err
		}
		owner, ok := a.GetUser(c.PathInt("uid"))
		if !ok {
			return core.ErrNotFound("Пользователь не найден")
		}
		var cur Folder
		if id := c.PathInt("dir"); id == 0 {
			cur = rootFolder(owner.ID)
		} else if f, ok := folders.Get(id); ok && f.IDUser == owner.ID {
			cur = f
		} else {
			return core.ErrNotFound("Папка не найдена")
		}
		me := c.User()
		mine := me != nil && me.ID == owner.ID
		out := map[string]any{"owner": a.Ref(owner.ID), "id": cur.ID, "name": cur.Name, "msg": a.Text.Render(cur.Msg), "path": folderPath(cur), "mine": mine, "osn": cur.Osn, "has_pass": cur.Pass != ""}
		if !folderChainOpen(a, c, cur) {
			out["blocked"] = "password"
			return c.JSON(out)
		}
		if mine || c.Can("obmen_dir_edit") {
			out["raw_msg"], out["pass"] = cur.Msg, cur.Pass
		}
		out["can_edit"] = mine || c.Can("obmen_dir_edit")
		out["can_create"] = mine && cur.Osn < 6
		subs := folders.By("parent", i(cur.ID))
		sort.Slice(subs, func(x, y int) bool { return subs[x].Time > subs[y].Time })
		fl := files.By("mydir", i(cur.ID))
		sort.Slice(fl, func(x, y int) bool { return fl[x].Time > fl[y].Time })
		p := c.Page()
		total := len(subs) + len(fl)
		items := []map[string]any{}
		for k := p.Offset; k < total && k < p.Offset+p.Per; k++ {
			if k < len(subs) {
				s := subs[k]
				nf, nfiles := 0, 0
				for _, x := range folderSubtree(s.ID) {
					if x != s.ID {
						nf++
					}
					nfiles += files.CountBy("mydir", i(x))
				}
				items = append(items, map[string]any{"kind": "dir", "id": s.ID, "name": s.Name, "msg": a.Text.Plain(s.Msg, 80), "lock": s.Pass != "", "folders": nf, "files": nfiles,
					"can_edit": mine || c.Can("obmen_dir_edit")})
			} else {
				f := fl[k-len(subs)]
				d, _ := dirs.Get(f.IDDir)
				m := fileRow(a, f, &d)
				m["kind"] = "file"
				m["published"] = !d.My
				items = append(items, m)
			}
		}
		out["items"], out["total"], out["page"], out["per"], out["pages"] = items, total, p.Page, p.Per, max(1, (total+p.Per-1)/p.Per)
		return c.JSON(out)
	})

	a.POST("/api/pfiles/folder/{id}/unlock", func(c *core.Ctx) error {
		f, ok := folders.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		var in struct {
			Password string `json:"password"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if in.Password != f.Pass {
			return core.ErrBad("Неверный пароль")
		}
		setCookie(c, "pf"+i(f.ID), folderToken(f))
		return c.OK()
	})

	type folderIn struct {
		Parent int64  `json:"parent"`
		Name   string `json:"name"`
		Msg    string `json:"msg"`
		Pass   string `json:"pass"`
	}
	check := func(in *folderIn) error {
		in.Name, in.Msg, in.Pass = strings.TrimSpace(in.Name), strings.TrimSpace(in.Msg), strings.TrimSpace(in.Pass)
		if err := lenOK(in.Name, 3, 30, "Название"); err != nil {
			return err
		}
		if err := lenOK(in.Msg, 0, 256, "Описание"); err != nil {
			return err
		}
		return lenOK(in.Pass, 0, 12, "Пароль")
	}
	a.POST("/api/pfiles/folder", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if err := banErr(a, c); err != nil {
			return err
		}
		var in folderIn
		if err := c.Bind(&in); err != nil {
			return err
		}
		par, ok := folders.Get(in.Parent)
		if !ok || par.IDUser != me.ID {
			return core.ErrForbidden()
		}
		if par.Osn >= 6 {
			return core.ErrBad("Нельзя создавать папки в более чем пяти вложенных директориях")
		}
		if err := check(&in); err != nil {
			return err
		}
		id := folders.Insert(&Folder{IDUser: me.ID, Name: a.Text.Clean(in.Name), Msg: in.Msg, Pass: in.Pass, Parent: par.ID, Osn: par.Osn + 1, Time: core.Now()})
		return c.OK("id", id)
	}, core.Auth)

	a.PUT("/api/pfiles/folder/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		f, ok := folders.Get(c.PathInt("id"))
		if !ok || (f.IDUser != me.ID && !a.Can(me.ID, "obmen_dir_edit")) {
			return core.ErrForbidden()
		}
		var in folderIn
		if err := c.Bind(&in); err != nil {
			return err
		}
		if err := check(&in); err != nil {
			return err
		}
		folders.Update(f.ID, func(x *Folder) { x.Name, x.Msg, x.Pass = a.Text.Clean(in.Name), in.Msg, in.Pass })
		return c.OK()
	}, core.Auth)

	a.DELETE("/api/pfiles/folder/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		f, ok := folders.Get(c.PathInt("id"))
		if !ok || f.Osn == 1 {
			return core.ErrNotFound()
		}
		if f.IDUser != me.ID && !a.Can(me.ID, "obmen_file_delete") {
			return core.ErrForbidden()
		}
		nf, nfiles := 0, 0
		for _, id := range folderSubtree(f.ID) {
			for _, fl := range files.By("mydir", i(id)) {
				deleteFile(a, fl)
				nfiles++
			}
			folders.Delete(id)
			nf++
		}
		return c.OK("folders", nf, "files", nfiles, "parent", f.Parent)
	}, core.Auth)

	// загрузка файла в личную папку (и опционально сразу в зону обмена)
	a.POST("/api/pfiles/upload", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if err := banErr(a, c); err != nil {
			return err
		}
		if err := c.R.ParseMultipartForm(8 << 20); err != nil && c.R.MultipartForm == nil {
			return core.ErrBad("Ошибка при выгрузке файла")
		}
		folderID := atoi64(c.FormValue("folder"))
		fo, ok := folders.Get(folderID)
		if !ok || fo.IDUser != me.ID {
			return core.ErrForbidden("Загружать можно только в свои папки")
		}
		target := myDir()
		if od := atoi64(c.FormValue("obmen_dir")); od != 0 {
			d, ok := dirs.Get(od)
			if !ok || !d.Upload || d.My {
				return core.ErrBad("Эта папка зоны обмена недоступна для загрузки")
			}
			if lim := int64(a.Cfg.Int("obmen_limit_up")); me.Balls < lim {
				return core.ErrBad("Для выгрузки в зону обмена нужно набрать минимум " + itoa(int(lim)) + " баллов")
			}
			target = d
		}
		globalMB := int64(a.Cfg.Int("max_upload_file_mb"))
		if globalMB <= 0 {
			globalMB = 100
		}
		maxSize := target.MaxSize
		if maxSize <= 0 || maxSize > globalMB<<20 {
			maxSize = globalMB << 20
		}
		up, err := c.FormFile("file", maxSize)
		if err != nil {
			return err
		}
		if up == nil || len(up.Data) == 0 {
			return core.ErrBad("Файл не выбран")
		}
		ext := strings.ToLower(up.Ext)
		if ext == "" || len(ext) > 12 {
			return core.ErrBad("У файла нет допустимого расширения")
		}
		if !extAllowed(target, ext) {
			return core.ErrBad("Неверное расширение файла")
		}
		name := strings.TrimSpace(c.FormValue("name"))
		if name == "" {
			name = strings.TrimSuffix(up.Name, "."+up.Ext)
		}
		name = strings.NewReplacer("#", "", "?", "", "/", "", "\\", "").Replace(name)
		if r := []rune(name); len(r) > 128 {
			name = string(r[:128])
		}
		if err := lenOK(name, 1, 128, "Название"); err != nil {
			return err
		}
		opis := strings.TrimSpace(c.FormValue("msg"))
		if err := lenOK(opis, 0, 2048, "Описание"); err != nil {
			return err
		}
		now := core.Now()
		f := File{IDUser: me.ID, IDDir: target.ID, MyDir: fo.ID, Name: a.Text.Clean(name), Ras: ext, Type: up.MIME, Time: now, TimeLast: now, Size: up.Size, Opis: opis, Metka: c.FormValue("metka") == "1"}
		if !target.My {
			f.TimeGo = now
		}
		f.ID = files.Insert(&f)
		if err := core.WriteFile(filePath(a, f.ID), up.Data); err != nil {
			files.Delete(f.ID)
			return core.ErrBad("Ошибка при выгрузке")
		}
		// скриншот
		scrMB := int64(a.Cfg.Int("max_upload_screen_mb"))
		if scrMB <= 0 {
			scrMB = 8
		}
		if sc, err := c.FormFile("screen", scrMB<<20); err == nil && sc != nil {
			if th, _, err := core.Thumb(sc.Data, 320, 320, false); err == nil {
				if core.WriteFile(screenPath(a, f.ID), th) == nil {
					files.Update(f.ID, func(x *File) { x.Screen = true })
				}
			}
		}
		a.AddRatingTmp(me.ID, 3)
		if fo.Pass == "" {
			a.FanTape(me.ID, core.KObmen, "obmen", f.ID, core.TapeOpts{})
		}
		return c.OK("id", f.ID, "folder", fo.ID)
	}, core.Auth)
}

func atoi64(s string) int64 {
	var n int64
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int64(r-'0')
	}
	return n
}

var _ = model.User{}
