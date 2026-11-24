package socialplus

import (
	"sort"
	"strconv"
	"strings"

	"dcms/internal/core"
	"dcms/internal/mods/profile"
	"dcms/internal/store"
)

// ContentReaction — универсальная эмодзи-реакция на пост/комментарий.
type ContentReaction struct {
	store.Base
	TargetType string `json:"target_type"`
	TargetID   int64  `json:"target_id"`
	UserID     int64  `json:"user_id"`
	Emoji      string `json:"emoji"`
}

var (
	reactsT      *store.Table[ContentReaction, *ContentReaction]
	allowedEmoji = map[string]bool{"🔥": true, "❤️": true, "😂": true, "👍": true, "😮": true}
)

func tKey(typ string, id int64) string { return typ + ":" + store.I(id) }
func uKey(typ string, id, uid int64) string {
	return typ + ":" + store.I(id) + ":" + store.I(uid)
}

func summarize(typ string, id, meID int64) []map[string]any {
	list := reactsT.By("target", tKey(typ, id))
	if len(list) == 0 {
		return []map[string]any{}
	}
	type agg struct {
		cnt  int
		mine bool
	}
	m := map[string]*agg{}
	var order []string
	for _, r := range list {
		a, ok := m[r.Emoji]
		if !ok {
			a = &agg{}
			m[r.Emoji] = a
			order = append(order, r.Emoji)
		}
		a.cnt++
		if meID > 0 && r.UserID == meID {
			a.mine = true
		}
	}
	out := make([]map[string]any, 0, len(order))
	for _, e := range order {
		out = append(out, map[string]any{"emoji": e, "count": m[e].cnt, "mine": m[e].mine})
	}
	return out
}

// Register регистрирует универсальные реакции, автодополнение @ник и умную ленту.
func Register(a *core.App) {
	reactsT = store.NewTable[ContentReaction](a.DB, "content_reactions").
		Index("target", func(x *ContentReaction) string { return tKey(x.TargetType, x.TargetID) }).
		Index("pair", func(x *ContentReaction) string { return uKey(x.TargetType, x.TargetID, x.UserID) })

	a.OnUserDelete(func(uid int64) {
		reactsT.DeleteWhere(func(x *ContentReaction) bool { return x.UserID == uid })
	})

	// Пакетное получение реакций
	a.GET("/api/reactions", func(c *core.Ctx) error {
		typ := strings.TrimSpace(c.Q("type"))
		if typ == "" {
			return c.JSON(map[string]any{"items": map[string]any{}})
		}
		var meID int64
		if u := c.User(); u != nil {
			meID = u.ID
		}
		res := map[string]any{}
		for _, part := range strings.Split(c.Q("ids"), ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
			if err == nil && id > 0 {
				res[store.I(id)] = summarize(typ, id, meID)
			}
		}
		return c.JSON(map[string]any{"items": res})
	})

	// Переключить эмодзи-реакцию
	a.POST("/api/reactions/toggle", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		var in struct {
			Type  string `json:"type"`
			ID    int64  `json:"id"`
			Emoji string `json:"emoji"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if in.Type == "" || in.ID <= 0 || !allowedEmoji[in.Emoji] {
			return core.ErrBad("Некорректная реакция")
		}
		if old, ok := reactsT.OneBy("pair", uKey(in.Type, in.ID, me.ID)); ok {
			if old.Emoji == in.Emoji {
				reactsT.Delete(old.ID)
			} else {
				reactsT.Update(old.ID, func(x *ContentReaction) { x.Emoji = in.Emoji })
			}
		} else {
			reactsT.Insert(&ContentReaction{TargetType: in.Type, TargetID: in.ID, UserID: me.ID, Emoji: in.Emoji})
		}
		return c.OK("reactions", summarize(in.Type, in.ID, me.ID))
	}, core.Auth)

	// Быстрый поиск пользователей для автодополнения @ник
	a.GET("/api/users/mention", func(c *core.Ctx) error {
		q := strings.ToLower(strings.TrimSpace(c.Q("q")))
		if len(q) < 1 {
			return c.JSON(map[string]any{"items": []any{}})
		}
		var out []map[string]any
		for _, u := range a.T.Users.All() {
			if strings.HasPrefix(strings.ToLower(u.Nick), q) || strings.Contains(strings.ToLower(u.Nick), q) {
				r := a.Ref(u.ID)
				out = append(out, map[string]any{"id": r.ID, "nick": r.Nick, "avatar": r.Avatar})
				if len(out) >= 6 {
					break
				}
			}
		}
		return c.JSON(map[string]any{"items": out})
	})

	// Умная главная лента (Популярное / Подписки)
	a.GET("/api/smart-feed", func(c *core.Ctx) error {
		me := c.User()
		tab := c.Q("tab")
		if tab == "" {
			if me != nil {
				tab = "following"
			} else {
				tab = "popular"
			}
		}
		friendSet := map[int64]bool{}
		if me != nil {
			friendSet[me.ID] = true
			for _, f := range a.T.Friends.By("user", store.I(me.ID)) {
				if f.I == 1 {
					friendSet[f.Frend] = true
				}
			}
		}
		var meID int64
		if me != nil {
			meID = me.ID
		}
		posts := profile.RecentWallPosts(a, 120)
		type entry struct {
			id      int64
			wallID  int64
			author  int64
			time    int64
			msg     string
			reacts  int
			score   float64
		}
		now := core.Now()
		var list []entry
		for _, p := range posts {
			if tab == "following" && me != nil && !friendSet[p.IDUser] && !friendSet[p.IDStena] {
				continue
			}
			rc := reactsT.CountBy("target", tKey("stena", p.ID))
			ageH := float64(max(1, now-p.Time)) / 3600.0
			sc := float64(rc*3+1) / (ageH + 2.0)
			list = append(list, entry{id: p.ID, wallID: p.IDStena, author: p.IDUser, time: p.Time, msg: p.Msg, reacts: rc, score: sc})
		}
		if tab == "popular" {
			sort.Slice(list, func(x, y int) bool {
				if list[x].score == list[y].score {
					return list[x].time > list[y].time
				}
				return list[x].score > list[y].score
			})
		} else {
			sort.Slice(list, func(x, y int) bool { return list[x].time > list[y].time })
		}
		pg := c.Page()
		total := len(list)
		end := min(pg.Offset+pg.Per, total)
		var slice []entry
		if pg.Offset < total {
			slice = list[pg.Offset:end]
		}
		items := make([]map[string]any, 0, len(slice))
		for _, e := range slice {
			items = append(items, map[string]any{
				"id":        e.id,
				"type":      "stena",
				"wall_id":   e.wallID,
				"wall_user": a.Ref(e.wallID),
				"user":      a.Ref(e.author),
				"time":      e.time,
				"html":      a.Text.Render(e.msg),
				"reactions": summarize("stena", e.id, meID),
				"link":      "/user/stena/" + store.I(e.wallID),
			})
		}
		return c.List(items, total, pg)
	})
}
