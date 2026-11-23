package extras

import (
	"sort"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

var bmTypes = []struct{ Type, Name, Describer string }{
	{"people", "Люди", ""}, {"file", "Файлы", "obmen"}, {"foto", "Фотографии", "foto"}, {"forum", "Форум", "them"}, {"notes", "Блоги", "notes"},
}

func bmDescriber(t string) (string, bool) {
	for _, x := range bmTypes {
		if x.Type == t {
			return x.Describer, true
		}
	}
	return "", false
}

func registerBookmarks(a *core.App) {
	a.POST("/api/bookmark/toggle", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		var in struct {
			Type string `json:"type"`
			ID   int64  `json:"id"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		desc, ok := bmDescriber(in.Type)
		if !ok {
			return core.ErrBad("Неверный тип закладки")
		}
		if in.Type == "people" {
			if _, ok := a.GetUser(in.ID); !ok || in.ID == me.ID {
				return core.ErrNotFound()
			}
		} else if _, ok := a.Describe(desc, in.ID); !ok {
			return core.ErrNotFound("Объект не найден")
		}
		key := store.Key(me.ID, in.Type, in.ID)
		if ex, has := a.T.Bookmarks.OneBy("uobj", key); has {
			a.T.Bookmarks.Delete(ex.ID)
			return c.OK("on", false)
		}
		a.T.Bookmarks.Insert(&model.Bookmarks{IDUser: me.ID, IDObject: in.ID, Type: in.Type, Time: core.Now(), TimeObn: core.Now()})
		return c.OK("on", true)
	}, core.Auth)

	// категории закладок пользователя с количеством
	a.GET("/api/bookmarks/{uid}", func(c *core.Ctx) error {
		uid := c.PathInt("uid")
		u, ok := a.GetUser(uid)
		if !ok {
			return core.ErrNotFound("Пользователь не найден")
		}
		var cats []map[string]any
		for _, t := range bmTypes {
			n := a.T.Bookmarks.Count(func(b *model.Bookmarks) bool { return b.IDUser == uid && b.Type == t.Type })
			cats = append(cats, map[string]any{"type": t.Type, "name": t.Name, "count": n})
		}
		return c.JSON(map[string]any{"owner": a.Ref(u.ID), "items": cats, "mine": c.UID() == uid})
	})
	a.GET("/api/bookmarks/{uid}/{type}", func(c *core.Ctx) error {
		uid, typ := c.PathInt("uid"), c.R.PathValue("type")
		desc, ok := bmDescriber(typ)
		if !ok {
			return core.ErrNotFound()
		}
		u, ok := a.GetUser(uid)
		if !ok {
			return core.ErrNotFound("Пользователь не найден")
		}
		p := c.Page()
		list, total := a.T.Bookmarks.Select(store.Opts[model.Bookmarks]{
			Where:  func(b *model.Bookmarks) bool { return b.IDUser == uid && b.Type == typ },
			Offset: p.Offset, Limit: p.Per, Desc: true,
			Less: func(x, y *model.Bookmarks) bool { return x.Time < y.Time },
		})
		items := make([]map[string]any, 0, len(list))
		for _, b := range list {
			m := map[string]any{"id": b.IDObject, "time": b.Time}
			if typ == "people" {
				m["user"] = a.Ref(b.IDObject)
			} else {
				d, ok := a.Describe(desc, b.IDObject)
				if !ok {
					continue
				}
				m["title"], m["text"], m["link"], m["image"], m["owner"] = d.Title, d.Text, d.Link, d.Image, a.Ref(d.Owner)
			}
			items = append(items, m)
		}
		sort.SliceStable(items, func(x, y int) bool { return items[x]["time"].(int64) > items[y]["time"].(int64) })
		return c.JSON(map[string]any{"items": items, "total": total, "page": p.Page, "per": p.Per, "pages": max(1, (total+p.Per-1)/p.Per), "owner": a.Ref(u.ID), "mine": c.UID() == uid, "type": typ})
	})
}
