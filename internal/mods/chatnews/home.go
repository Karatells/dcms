package chatnews

import (
	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

func registerHome(a *core.App) {
	a.GET("/api/home", func(c *core.Ctx) error {
		now := core.Now()
		me := c.User()
		out := map[string]any{
			"users_online":  a.T.Users.Count(func(u *model.User) bool { return u.DateLast > now-600 }),
			"guests_online": a.T.Guests.Count(func(g *model.Guests) bool { return g.DateLast > now-600 && g.Pereh > 0 }),
		}
		// новость-баннер
		if me == nil || me.NewsRead == 0 {
			if l, _ := newsT.Select(store.Opts[News]{Desc: true, Limit: 1, Where: func(n *News) bool { return n.MainTime > now }}); len(l) > 0 {
				out["news"] = newsCard(a, l[0], true)
			}
		}
		out["blocks"] = a.HomeBlocks(me)
		out["admin"] = c.Can("adm_panel_show")
		return c.JSON(out)
	})
}
