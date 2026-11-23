package extras

import (
	"math/rand"
	"sort"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

func hasGoal(u *model.User) bool {
	return u.AnkLov1 == 1 || u.AnkLov2 == 1 || u.AnkLov3 == 1 || u.AnkLov4 == 1 || u.AnkLov5 == 1 || u.AnkLov6 == 1 || u.AnkLov7 == 1 ||
		u.AnkLov8 == 1 || u.AnkLov9 == 1 || u.AnkLov10 == 1 || u.AnkLov11 == 1 || u.AnkLov12 == 1 || u.AnkLov13 == 1 || u.AnkLov14 == 1
}

func registerLove(a *core.App) {
	a.Counter("love", func(a *core.App, u *model.User) string {
		since := core.Now() - 259200
		return i(int64(a.T.Users.Count(func(x *model.User) bool { return hasGoal(x) && x.AnkOSebe != "" && x.DateLast > since })))
	})
	a.GET("/api/love", func(c *core.Ctx) error {
		since := core.Now() - 259200
		pol := c.Q("orders")
		p := c.Page()
		list, total := a.T.Users.Select(store.Opts[model.User]{Desc: true, Offset: p.Offset, Limit: p.Per,
			Where: func(u *model.User) bool {
				if !hasGoal(u) || u.AnkOSebe == "" || u.DateLast <= since {
					return false
				}
				return pol == "" || pol == "all" || (pol == "man" && u.Pol == 1) || (pol == "woman" && u.Pol == 0)
			},
			Less: func(x, y *model.User) bool { return x.DateLast < y.DateLast }})
		items := make([]map[string]any, 0, len(list))
		for _, u := range list {
			items = append(items, map[string]any{"user": a.Ref(u.ID), "msg": a.Text.Render(u.AnkOSebe)})
		}
		out := map[string]any{"items": items, "total": total, "page": p.Page, "per": p.Per, "pages": max(1, (total+p.Per-1)/p.Per), "orders": pol}
		now := core.Now()
		act := liders.Find(func(l *Lider) bool { return l.Time > now })
		sort.Slice(act, func(x, y int) bool { return act[x].ID < act[y].ID })
		if len(act) > 0 {
			l := act[rand.Intn(len(act))]
			out["lider"] = map[string]any{"user": a.Ref(l.IDUser), "msg": a.Text.Render(l.Msg), "total": len(act)}
		}
		return c.JSON(out)
	})
}
