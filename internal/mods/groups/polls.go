package groups

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

// GroupPoll — опрос в сообществе.
type GroupPoll struct {
	store.Base
	IDGroup  int64  `json:"id_group"`
	IDUser   int64  `json:"id_user"`
	Question string `json:"question"`
	Options  string `json:"options"` // JSON []string
	Multi    bool   `json:"multi"`
	Anon     bool   `json:"anon"`
	Closed   bool   `json:"closed"`
	Time     int64  `json:"time"`
}

// GroupPollVote — голос пользователя в опросе.
type GroupPollVote struct {
	store.Base
	IDPoll    int64 `json:"id_poll"`
	IDUser    int64 `json:"id_user"`
	OptionIdx int   `json:"option_idx"`
	Time      int64 `json:"time"`
}

var (
	pollsT     *store.Table[GroupPoll, *GroupPoll]
	pollVotesT *store.Table[GroupPollVote, *GroupPollVote]
)

func registerPollsAndStats(a *core.App) {
	pollsT = store.NewTable[GroupPoll](a.DB, "group_polls").
		Index("group", func(x *GroupPoll) string { return i(x.IDGroup) })
	pollVotesT = store.NewTable[GroupPollVote](a.DB, "group_poll_votes").
		Index("poll", func(x *GroupPollVote) string { return i(x.IDPoll) }).
		Index("pair", func(x *GroupPollVote) string { return store.Key(x.IDPoll, x.IDUser) })

	pollDTO := func(p GroupPoll, viewer *model.User, pr perms) map[string]any {
		var opts []string
		_ = json.Unmarshal([]byte(p.Options), &opts)
		votes := pollVotesT.By("poll", i(p.ID))
		counts := make([]int, len(opts))
		voters := map[int64]bool{}
		myVotes := map[int]bool{}
		for _, v := range votes {
			if v.OptionIdx >= 0 && v.OptionIdx < len(opts) {
				counts[v.OptionIdx]++
				voters[v.IDUser] = true
				if viewer != nil && v.IDUser == viewer.ID {
					myVotes[v.OptionIdx] = true
				}
			}
		}
		totalVotes := len(votes)
		optList := make([]map[string]any, 0, len(opts))
		for idx, txt := range opts {
			pct := 0
			if totalVotes > 0 {
				pct = (counts[idx] * 100) / totalVotes
			}
			optList = append(optList, map[string]any{
				"idx":   idx,
				"text":  txt,
				"votes": counts[idx],
				"pct":   pct,
				"voted": myVotes[idx],
			})
		}
		return map[string]any{
			"id":          p.ID,
			"group_id":    p.IDGroup,
			"author":      a.Ref(p.IDUser),
			"question":    p.Question,
			"options":     optList,
			"multi":       p.Multi,
			"anon":        p.Anon,
			"closed":      p.Closed,
			"voters":      len(voters),
			"total_votes": totalVotes,
			"time":        p.Time,
			"can_mod":     pr.ModBlog || pr.Manage,
		}
	}

	// ── Список опросов сообщества
	a.GET("/api/groups/{id}/polls", func(c *core.Ctx) error {
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Сообщество не найдено")
		}
		if !canViewGroup(c.User(), g) {
			return core.ErrForbidden("Это закрытое сообщество")
		}
		pr := permsOf(c.User(), g)
		all := pollsT.By("group", i(g.ID))
		sort.Slice(all, func(x, y int) bool { return all[x].ID > all[y].ID })
		items := make([]map[string]any, 0, len(all))
		for _, p := range all {
			items = append(items, pollDTO(p, c.User(), pr))
		}
		return c.JSON(map[string]any{"items": items, "can_create": pr.ModBlog || pr.Manage})
	})

	// ── Создать опрос
	a.POST("/api/groups/{id}/polls", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok || g.Blocked {
			return core.ErrNotFound("Сообщество не найдено")
		}
		pr := permsOf(me, g)
		if !pr.ModBlog && !pr.Manage {
			return core.ErrForbidden("Создавать опросы могут администраторы и модераторы блога")
		}
		var in struct {
			Question string   `json:"question"`
			Options  []string `json:"options"`
			Multi    bool     `json:"multi"`
			Anon     bool     `json:"anon"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		q := a.Text.Clean(strings.TrimSpace(in.Question))
		if utf8.RuneCountInString(q) < 3 || utf8.RuneCountInString(q) > 200 {
			return core.ErrBad("Вопрос опроса: от 3 до 200 символов")
		}
		var opts []string
		for _, o := range in.Options {
			o = a.Text.Clean(strings.TrimSpace(o))
			if o != "" {
				if utf8.RuneCountInString(o) > 100 {
					o = core.Trunc(o, 100)
				}
				opts = append(opts, o)
			}
		}
		if len(opts) < 2 || len(opts) > 10 {
			return core.ErrBad("Укажите от 2 до 10 вариантов ответа")
		}
		b, _ := json.Marshal(opts)
		id := pollsT.Insert(&GroupPoll{
			IDGroup: g.ID, IDUser: me.ID, Question: q, Options: string(b),
			Multi: in.Multi, Anon: in.Anon, Time: core.Now(),
		})
		p, _ := pollsT.Get(id)
		return c.JSON(pollDTO(p, me, pr))
	}, core.Auth)

	// ── Проголосовать в опросе
	a.POST("/api/group-polls/{pid}/vote", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		p, ok := pollsT.Get(c.PathInt("pid"))
		if !ok {
			return core.ErrNotFound("Опрос не найден")
		}
		g, ok := groupsT.Get(p.IDGroup)
		if !ok || !canViewGroup(me, g) {
			return core.ErrForbidden()
		}
		if p.Closed {
			return core.ErrBad("Голосование закрыто")
		}
		var opts []string
		_ = json.Unmarshal([]byte(p.Options), &opts)
		var in struct {
			OptionIdx int `json:"option_idx"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if in.OptionIdx < 0 || in.OptionIdx >= len(opts) {
			return core.ErrBad("Некорректный вариант ответа")
		}
		existing := pollVotesT.By("pair", store.Key(p.ID, me.ID))
		if p.Multi {
			removed := false
			for _, v := range existing {
				if v.OptionIdx == in.OptionIdx {
					pollVotesT.Delete(v.ID)
					removed = true
				}
			}
			if !removed {
				pollVotesT.Insert(&GroupPollVote{IDPoll: p.ID, IDUser: me.ID, OptionIdx: in.OptionIdx, Time: core.Now()})
			}
		} else {
			same := false
			for _, v := range existing {
				if v.OptionIdx == in.OptionIdx {
					same = true
				}
				pollVotesT.Delete(v.ID)
			}
			if !same {
				pollVotesT.Insert(&GroupPollVote{IDPoll: p.ID, IDUser: me.ID, OptionIdx: in.OptionIdx, Time: core.Now()})
			}
		}
		return c.JSON(pollDTO(p, me, permsOf(me, g)))
	}, core.Auth)

	// ── Закрыть / открыть опрос
	a.POST("/api/group-polls/{pid}/close", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		p, ok := pollsT.Get(c.PathInt("pid"))
		if !ok {
			return core.ErrNotFound()
		}
		g, ok := groupsT.Get(p.IDGroup)
		if !ok {
			return core.ErrNotFound()
		}
		pr := permsOf(me, g)
		if !pr.ModBlog && !pr.Manage {
			return core.ErrForbidden()
		}
		pollsT.Update(p.ID, func(x *GroupPoll) { x.Closed = !x.Closed })
		upd, _ := pollsT.Get(p.ID)
		return c.JSON(pollDTO(upd, me, pr))
	}, core.Auth)

	// ── Удалить опрос
	a.DELETE("/api/group-polls/{pid}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		p, ok := pollsT.Get(c.PathInt("pid"))
		if !ok {
			return core.ErrNotFound()
		}
		g, ok := groupsT.Get(p.IDGroup)
		if !ok {
			return core.ErrNotFound()
		}
		pr := permsOf(me, g)
		if !pr.ModBlog && !pr.Manage {
			return core.ErrForbidden()
		}
		pollVotesT.DeleteBy("poll", i(p.ID))
		pollsT.Delete(p.ID)
		return c.OK()
	}, core.Auth)

	// ── Статистика сообщества (за 7 дней + общие показатели)
	a.GET("/api/groups/{id}/stats", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		if !permsOf(me, g).Manage {
			return core.ErrForbidden("Статистика доступна только создателю и администраторам")
		}
		now := time.Now()
		days := make([]map[string]any, 7)
		dayKeys := make([]string, 7)
		dayIdx := map[string]int{}
		for d := 0; d < 7; d++ {
			dt := now.AddDate(0, 0, -(6 - d))
			k := dt.Format("2006-01-02")
			dayKeys[d] = k
			dayIdx[k] = d
			days[d] = map[string]any{
				"date":    dt.Format("02.01"),
				"members": 0,
				"chat":    0,
				"forum":   0,
				"blog":    0,
				"dm":      0,
			}
		}
		bump := func(ts int64, field string) {
			k := time.Unix(ts, 0).Format("2006-01-02")
			if idx, ok := dayIdx[k]; ok {
				days[idx][field] = days[idx][field].(int) + 1
			}
		}
		members := membersT.By("group", i(g.ID))
		for _, m := range members {
			bump(m.Time, "members")
		}
		chats := chatT.By("group", i(g.ID))
		for _, m := range chats {
			bump(m.Time, "chat")
		}
		blogs := blogT.By("group", i(g.ID))
		for _, p := range blogs {
			bump(p.Time, "blog")
		}
		topics := topicsT.By("group", i(g.ID))
		for _, t := range topics {
			bump(t.Time, "forum")
		}
		tposts := tpostsT.By("group", i(g.ID))
		for _, p := range tposts {
			bump(p.Time, "forum")
		}
		dms := dmT.By("group", i(g.ID))
		for _, m := range dms {
			bump(m.Time, "dm")
		}
		files := filesT.By("group", i(g.ID))
		var totalLoads int64
		for _, f := range files {
			totalLoads += f.Loads
		}
		return c.JSON(map[string]any{
			"summary": map[string]any{
				"members":     len(members),
				"blog_posts":  len(blogs),
				"topics":      len(topics),
				"forum_posts": len(tposts),
				"chat_msgs":   len(chats),
				"files":       len(files),
				"file_loads":  totalLoads,
				"dm_msgs":     len(dms),
				"polls":       pollsT.CountBy("group", i(g.ID)),
			},
			"days": days,
		})
	}, core.Auth)

	// ── Верификация пользователя администратором сайта (синяя галочка)
	a.POST("/api/admin/user/{id}/verify", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if me.Level < 3 && !a.Can(me.ID, "adm_set_user") && !a.Can(me.ID, "user_prof_edit") {
			return core.ErrForbidden()
		}
		uid := c.PathInt("id")
		u, ok := a.GetUser(uid)
		if !ok {
			return core.ErrNotFound("Пользователь не найден")
		}
		want := !u.Verified
		a.T.Users.Update(u.ID, func(x *model.User) { x.Verified = want })
		a.DropUserCache(u.ID)
		act := "Снятие верификации"
		if want {
			act = "Верификация"
		}
		a.AdminLog(me.ID, "Пользователи", act, "Пользователь #"+i(u.ID)+" ("+u.Nick+")")
		return c.OK("verified", want)
	}, core.Auth)
}
