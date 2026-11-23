package content

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"dcms/internal/core"
)

func smilePath(a *core.App, id int64) string {
	return filepath.Join(a.UploadDir("smiles"), i(id)+".gif")
}

var seedSmiles = []struct {
	ID   int64
	Code string
}{{1184, ".сердит."}, {1185, ":)"}, {331, "=)"}, {1168, ".че."}, {1169, ":("}, {1170, ":D"}, {1186, ".язык."}, {1173, ".миг."}, {1174, ".крут."}, {1182, ".ах."}, {1183, ".кисс."}}

func registerSmiles(a *core.App) {
	if smileDirs.Len() == 0 {
		dir := SmileDir{Name: "Общие"}
		dir.ID = 21
		smileDirs.Put(dir)
		for _, s := range seedSmiles {
			raw, err := seedFS.ReadFile("seed/" + i(s.ID) + ".gif")
			if err != nil {
				continue
			}
			_ = core.WriteFile(smilePath(a, s.ID), raw)
			sm := Smile{Smile: s.Code, Dir: 21}
			sm.ID = s.ID
			smiles.Put(sm)
		}
	}
	a.Text.SetSmiles(func() []core.Smile {
		all := smiles.All()
		var out []core.Smile
		for _, s := range all {
			for _, code := range strings.Split(s.Smile, "|") {
				if code = strings.TrimSpace(code); code != "" {
					out = append(out, core.Smile{Code: code, URL: "/api/smiles/img/" + i(s.ID)})
				}
			}
		}
		// длинные коды первыми, чтобы «:-))» не превращалось в «:-)» + «)»
		sort.SliceStable(out, func(x, y int) bool { return len(out[x].Code) > len(out[y].Code) })
		return out
	})

	a.GET("/api/smiles/all", func(c *core.Ctx) error {
		var out []map[string]string
		for _, s := range smiles.All() {
			code := strings.Split(s.Smile, "|")[0]
			out = append(out, map[string]string{"code": code, "url": "/api/smiles/img/" + i(s.ID)})
		}
		return c.JSON(out)
	})
	a.GET("/api/smiles/img/{id}", func(c *core.Ctx) error {
		raw, err := os.ReadFile(smilePath(a, c.PathInt("id")))
		if err != nil {
			return core.ErrNotFound()
		}
		return c.ServeBytes(raw, http.DetectContentType(raw), true)
	})
	a.GET("/api/smiles", func(c *core.Ctx) error {
		ds := smileDirs.All()
		sort.Slice(ds, func(x, y int) bool { return ds[x].ID < ds[y].ID })
		items := make([]map[string]any, 0, len(ds))
		for _, d := range ds {
			items = append(items, map[string]any{"id": d.ID, "name": d.Name, "opis": d.Opis, "count": smiles.CountBy("dir", i(d.ID))})
		}
		me := c.User()
		return c.JSON(map[string]any{"items": items, "admin": me != nil && me.Level > 3})
	})
	a.GET("/api/smiles/dir/{id}", func(c *core.Ctx) error {
		d, ok := smileDirs.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Категория не найдена")
		}
		p := c.Page()
		l := smiles.By("dir", i(d.ID))
		sort.Slice(l, func(x, y int) bool { return l[x].ID < l[y].ID })
		total := len(l)
		l = l[min(p.Offset, total):min(p.Offset+p.Per, total)]
		items := make([]map[string]any, 0, len(l))
		for _, s := range l {
			items = append(items, map[string]any{"id": s.ID, "smile": s.Smile, "url": "/api/smiles/img/" + i(s.ID)})
		}
		me := c.User()
		return c.JSON(map[string]any{"items": items, "total": total, "page": p.Page, "per": p.Per, "pages": max(1, (total+p.Per-1)/p.Per), "dir": map[string]any{"id": d.ID, "name": d.Name, "opis": d.Opis}, "admin": me != nil && me.Level > 2})
	})

	// ── админка (only_level(3))
	adm := func(c *core.Ctx) error { return c.NeedLevel(3) }
	a.POST("/api/smiles/dir", func(c *core.Ctx) error {
		if err := adm(c); err != nil {
			return err
		}
		var in struct {
			Name string `json:"name"`
			Opis string `json:"opis"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		name := strings.TrimSpace(in.Name)
		if err := lenOK(name, 1, 32, "Название"); err != nil {
			return err
		}
		return c.OK("id", smileDirs.Insert(&SmileDir{Name: name, Opis: strings.TrimSpace(in.Opis)}))
	}, core.Auth)
	a.DELETE("/api/smiles/dir/{id}", func(c *core.Ctx) error {
		if err := adm(c); err != nil {
			return err
		}
		id := c.PathInt("id")
		if !smileDirs.Has(id) {
			return core.ErrNotFound()
		}
		for _, s := range smiles.By("dir", i(id)) {
			core.RemoveFile(smilePath(a, s.ID))
			smiles.Delete(s.ID)
		}
		smileDirs.Delete(id)
		a.Text.ResetSmiles()
		return c.OK()
	}, core.Auth)
	a.POST("/api/smiles/dir/{id}/upload", func(c *core.Ctx) error {
		if err := adm(c); err != nil {
			return err
		}
		id := c.PathInt("id")
		if !smileDirs.Has(id) {
			return core.ErrNotFound()
		}
		forms := int(c.QInt("forms"))
		if forms < 1 || forms > 20 {
			forms = 10
		}
		var okN, bad int
		var errs []string
		for k := 0; k < forms; k++ {
			up, err := c.FormFile("file_"+itoa(k), 2<<20)
			if err != nil {
				return err
			}
			code := strings.TrimSpace(c.FormValue("smile_" + itoa(k)))
			if up == nil {
				continue
			}
			if code == "" || len([]rune(code)) > 64 || !(up.Ext == "gif" || up.Ext == "png" || up.Ext == "jpg" || up.Ext == "jpeg") || len(up.Data) == 0 {
				bad++
				errs = append(errs, "Файл ("+itoa(k)+") не выгружен")
				continue
			}
			sid := smiles.Insert(&Smile{Smile: code, Dir: id})
			if err := core.WriteFile(smilePath(a, sid), up.Data); err != nil {
				smiles.Delete(sid)
				bad++
				continue
			}
			okN++
		}
		a.Text.ResetSmiles()
		return c.OK("uploaded", okN, "failed", bad, "errors", errs)
	}, core.Auth)
	a.PUT("/api/smiles/{id}", func(c *core.Ctx) error {
		if err := adm(c); err != nil {
			return err
		}
		var in struct {
			Smile string `json:"smile"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		code := strings.TrimSpace(in.Smile)
		if err := lenOK(code, 1, 64, "Название"); err != nil {
			return err
		}
		if _, ok := smiles.Update(c.PathInt("id"), func(s *Smile) { s.Smile = code }); !ok {
			return core.ErrNotFound()
		}
		a.Text.ResetSmiles()
		return c.OK()
	}, core.Auth)
	a.DELETE("/api/smiles/{id}", func(c *core.Ctx) error {
		if err := adm(c); err != nil {
			return err
		}
		id := c.PathInt("id")
		if !smiles.Delete(id) {
			return core.ErrNotFound()
		}
		core.RemoveFile(smilePath(a, id))
		a.Text.ResetSmiles()
		return c.OK()
	}, core.Auth)
}

func itoa(n int) string { return i(int64(n)) }
