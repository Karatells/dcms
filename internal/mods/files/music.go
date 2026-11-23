package files

import (
	"dcms/internal/core"
	"dcms/internal/store"
)

// Music — запись плейлиста пользователя (user_music).
type Music struct {
	store.Base
	IDUser int64  `json:"id_user"`
	Dir    string `json:"dir"`
	IDFile int64  `json:"id_file"`
}

func isAudio(ras string) bool { return ras == "mp3" || ras == "wav" || ras == "ogg" }

func registerMusic(a *core.App) {
	// плейлист пользователя
	a.GET("/api/music/{uid}", func(c *core.Ctx) error {
		uid := c.PathInt("uid")
		owner, ok := a.GetUser(uid)
		if !ok {
			return core.ErrNotFound("Пользователь не найден")
		}
		if b, ok := a.ActiveBan(c.UID(), "files"); ok && c.UID() != 0 {
			_ = b
			return core.ErrForbidden("Вам запрещён доступ к файлам")
		}
		p := c.Page()
		list, total := music.Select(store.Opts[Music]{Index: "user", Key: i(uid), Desc: true, Offset: p.Offset, Limit: p.Per})
		items := make([]map[string]any, 0, len(list))
		for _, m := range list {
			f, ok := files.Get(m.IDFile)
			if !ok {
				music.Delete(m.ID)
				continue
			}
			items = append(items, map[string]any{"id": m.ID, "file": f.ID, "name": f.Name, "size": f.Size, "size_str": core.SizeStr(f.Size), "ras": f.Ras,
				"src": "/api/obmen/file/" + i(f.ID) + "/download?inline=1", "download": "/api/obmen/file/" + i(f.ID) + "/download", "link": "/obmen/file/" + i(f.ID)})
		}
		return c.JSON(map[string]any{"items": items, "total": total, "page": p.Page, "per": p.Per, "pages": max(1, (total+p.Per-1)/p.Per), "owner": a.Ref(owner.ID), "mine": c.UID() == uid})
	})
	// добавить/убрать трек
	a.POST("/api/music/toggle/{file}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		f, ok := files.Get(c.PathInt("file"))
		if !ok || !isAudio(f.Ras) {
			return core.ErrBad("Это не аудиофайл")
		}
		if ex, has := music.Find(func(m *Music) bool { return m.IDUser == me.ID && m.IDFile == f.ID && m.Dir == "obmen" }), false; len(ex) > 0 {
			_ = has
			music.Delete(ex[0].ID)
			a.AddRatingTmp(f.IDUser, -1)
			return c.OK("on", false)
		}
		music.Insert(&Music{IDUser: me.ID, IDFile: f.ID, Dir: "obmen"})
		a.AddBalls(f.IDUser, 1, 1)
		return c.OK("on", true)
	}, core.Auth)
}
