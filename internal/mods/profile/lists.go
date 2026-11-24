package profile

import (
	"strings"
	"time"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

func userCard(a *core.App, u *model.User, staff bool) map[string]any {
	m := map[string]any{"user": a.Ref(u.ID), "rating": u.Rating, "balls": u.Balls, "city": u.AnkCity, "age": ageOf(u),
		"date_reg": u.DateReg, "date_last": u.DateLast, "about": u.AnkOSebe}
	if staff {
		m["url"], m["ip"], m["ua"] = u.URL, u.IP, u.UA
	}
	return m
}

func startOfDay() int64 {
	n := time.Now()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, n.Location()).Unix()
}

func registerLists(a *core.App) {
	// ---- список пользователей: sort=id|rating|balls|nick|pol|last, order=asc|desc, q=поиск,
	// filter=all|online|today|male|female|banned|staff|birthday
	a.GET("/api/users", func(c *core.Ctx) error {
		p := c.Page()
		q := strings.ToLower(strings.TrimSpace(c.Q("q")))
		now := core.Now()
		day := startOfDay()
		t := time.Now()
		filter := c.Q("filter")
		banned := map[int64]bool{}
		if filter == "banned" {
			for _, b := range a.T.Bans.All() {
				if b.Navsegda == 1 || b.Time > now {
					banned[b.IDUser] = true
				}
			}
		}
		qid := int64(0)
		if q != "" {
			for _, r := range q {
				if r < '0' || r > '9' {
					qid = 0
					break
				}
				qid = qid*10 + int64(r-'0')
			}
		}
		list := a.T.Users.Find(func(u *model.User) bool {
			if u.Activation != "" {
				return false
			}
			if q != "" && !strings.Contains(u.NickLower, q) && u.ID != qid {
				return false
			}
			switch filter {
			case "online":
				return u.DateLast > now-600
			case "today":
				return u.DateLast > day
			case "male":
				return u.Pol == 1
			case "female":
				return u.Pol == 0
			case "banned":
				return banned[u.ID]
			case "staff":
				return u.GroupAccess > 1
			case "birthday":
				return u.AnkDR == int64(t.Day()) && u.AnkMR == int64(t.Month())
			}
			return true
		})
		sortKey := c.Q("sort")
		desc := c.Q("order") != "asc"
		if filter == "online" && sortKey == "" {
			sortKey = "last"
		}
		if filter == "staff" && sortKey == "" {
			sortUsers(list, "id", false)
			// по группе по убыванию
			stable := list
			for i := 1; i < len(stable); i++ {
				for j := i; j > 0 && stable[j].GroupAccess > stable[j-1].GroupAccess; j-- {
					stable[j], stable[j-1] = stable[j-1], stable[j]
				}
			}
		} else {
			sortUsers(list, sortKey, desc)
		}
		total := len(list)
		end := min(p.Offset+p.Per, total)
		items := []map[string]any{}
		if p.Offset < total {
			staff := c.User() != nil && c.User().Level >= 7
			for i := p.Offset; i < end; i++ {
				items = append(items, userCard(a, &list[i], staff && filter == "online"))
			}
		}
		return c.List(items, total, p)
	})

	// ---- сводка (users/other.php)
	a.GET("/api/users/stats", func(c *core.Ctx) error {
		now := core.Now()
		day := startOfDay()
		t := time.Now()
		var all, online, today, male, female, staff, birthday int
		a.T.Users.Scan(func(u *model.User) bool {
			if u.Activation != "" {
				return true
			}
			all++
			if u.DateLast > now-600 {
				online++
			}
			if u.DateLast > day {
				today++
			}
			if u.Pol == 1 {
				male++
			} else {
				female++
			}
			if u.GroupAccess > 1 {
				staff++
			}
			if u.AnkDR == int64(t.Day()) && u.AnkMR == int64(t.Month()) {
				birthday++
			}
			return true
		})
		banned := 0
		seen := map[int64]bool{}
		for _, b := range a.T.Bans.All() {
			if (b.Navsegda == 1 || b.Time > now) && !seen[b.IDUser] {
				seen[b.IDUser] = true
				banned++
			}
		}
		return c.JSON(map[string]int{"all": all, "online": online, "today": today, "male": male, "female": female, "banned": banned, "staff": staff, "birthday": birthday})
	})

	// ---- мои гости
	a.GET("/api/myguest", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		p := c.Page()
		list, total := myGuests.Select(store.Opts[MyGuest]{Index: "ank", Key: store.I(me.ID), Desc: true, Offset: p.Offset, Limit: p.Per})
		items := make([]map[string]any, 0, len(list))
		ids := []int64{}
		for _, g := range list {
			items = append(items, map[string]any{"user": a.Ref(g.IDUser), "time": g.Time, "new": g.Read})
			if g.Read {
				ids = append(ids, g.ID)
			}
		}
		for _, id := range ids {
			myGuests.Update(id, func(x *MyGuest) { x.Read = false })
		}
		return c.List(items, total, p)
	}, core.Auth)
	a.DELETE("/api/myguest", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		myGuests.DeleteBy("ank", store.I(me.ID))
		return c.OK()
	}, core.Auth)

	// ---- друзья пользователя
	a.GET("/api/user/{id}/frends", func(c *core.Ctx) error {
		u, err := userByPath(c)
		if err != nil {
			return err
		}
		me := c.User()
		if st := access(a, me, &u); st != vOK || !a.CanAccessPrivacy(me, u.ID, a.UserSettings(u.ID).PrivatFriends) {
			return core.ErrForbidden("Список друзей недоступен")
		}
		now := core.Now()
		all := a.T.Friends.By("user", store.I(u.ID))
		online := 0
		var rows []model.Frends
		for _, f := range all {
			if f.I != 1 {
				continue
			}
			fu, ok := a.GetUser(f.Frend)
			if !ok {
				continue
			}
			if fu.DateLast > now-600 {
				online++
			}
			if c.Q("online") == "1" && fu.DateLast <= now-600 {
				continue
			}
			rows = append(rows, f)
		}
		// новые сверху
		for i := 1; i < len(rows); i++ {
			for j := i; j > 0 && rows[j].Time > rows[j-1].Time; j-- {
				rows[j], rows[j-1] = rows[j-1], rows[j]
			}
		}
		p := c.Page()
		items := []map[string]any{}
		for i := p.Offset; i < min(p.Offset+p.Per, len(rows)); i++ {
			items = append(items, map[string]any{"user": a.Ref(rows[i].Frend), "time": rows[i].Time})
		}
		out := map[string]any{"items": items, "total": len(rows), "page": p.Page, "per": p.Per, "pages": max(1, (len(rows)+p.Per-1)/p.Per),
			"all": len(all), "online": online, "owner": a.Ref(u.ID)}
		if me != nil && me.ID == u.ID {
			out["requests"] = a.T.FriendReqs.CountBy("to", store.I(u.ID))
		}
		return c.JSON(out)
	})

	// входящие заявки и исходящие
	a.GET("/api/frends/requests", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		in := []map[string]any{}
		for _, r := range a.T.FriendReqs.By("to", store.I(me.ID)) {
			in = append(in, map[string]any{"user": a.Ref(r.User), "time": r.Time})
		}
		out := []map[string]any{}
		for _, r := range a.T.FriendReqs.By("user", store.I(me.ID)) {
			out = append(out, map[string]any{"user": a.Ref(r.To), "time": r.Time})
		}
		return c.JSON(map[string]any{"incoming": in, "outgoing": out})
	}, core.Auth)

	// массовое удаление друзей (delete_marked)
	a.POST("/api/frends/delete", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		var in struct {
			IDs []int64 `json:"ids"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		for _, id := range in.IDs {
			if a.IsFriend(me.ID, id) {
				a.T.Friends.DeleteBy("pair", store.Key(me.ID, id))
				a.T.Friends.DeleteBy("pair", store.Key(id, me.ID))
				a.Notify(me.ID, id, "del_frend", me.ID, false)
			}
		}
		return c.OK()
	}, core.Auth)
}
