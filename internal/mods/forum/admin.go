package forum

import (
	"dcms/internal/core"
	"dcms/internal/model"
)

func registerAdmin(a *core.App) {
	a.GET("/api/admin/forum/settings", func(c *core.Ctx) error {
		if err := c.NeedAccess("adm_set_forum"); err != nil {
			return err
		}
		return c.JSON(map[string]int{"show_num_post": a.Cfg.Int("show_num_post"), "echo_rassh_forum": a.Cfg.Int("echo_rassh_forum"), "forum_counter": a.Cfg.Int("forum_counter")})
	}, core.Auth)

	a.PUT("/api/admin/forum/settings", func(c *core.Ctx) error {
		if err := c.NeedAccess("adm_set_forum"); err != nil {
			return err
		}
		var in map[string]int
		if err := c.Bind(&in); err != nil {
			return err
		}
		for _, k := range []string{"show_num_post", "echo_rassh_forum", "forum_counter"} {
			if v, ok := in[k]; ok && (v == 0 || v == 1) {
				a.Cfg.Set(k, map[int]string{0: "0", 1: "1"}[v])
			}
		}
		a.AdminLog(c.UID(), "Панель управления", "Настройки", "Изменены настройки форума")
		return c.OK()
	}, core.Auth)

	// синхронизация: удаление «осиротевших» разделов, тем и сообщений
	a.POST("/api/admin/forum/sync", func(c *core.Ctx) error {
		if err := c.NeedAccess("adm_forum_sinc"); err != nil {
			return err
		}
		var dr, dt, dp int
		for _, r := range razdels.All() {
			if !forums.Has(r.IDForum) {
				razdels.Delete(r.ID)
				dr++
			}
		}
		for _, t := range thems.All() {
			if !razdels.Has(t.IDRazdel) || !a.T.Users.Has(t.IDUser) {
				deleteTheme(a, t)
				dt++
			}
		}
		for _, p := range posts.All() {
			if !thems.Has(p.IDThem) || (p.IDUser != 0 && !a.T.Users.Has(p.IDUser)) {
				deletePostFiles(a, p.ID)
				posts.Delete(p.ID)
				dp++
			}
		}
		_ = model.User{}
		a.AdminLog(c.UID(), "Панель управления", "Форум", "Синхронизация таблиц форума")
		return c.JSON(map[string]int{"razdels": dr, "themes": dt, "posts": dp})
	}, core.Auth)
}
