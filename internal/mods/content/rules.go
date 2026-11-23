package content

import (
	"sort"
	"strings"

	"dcms/internal/core"
	"dcms/internal/store"
)

func rulesAdmin(c *core.Ctx) error { return c.NeedLevel(3) }

func sortedRules() []Rule {
	l := rules.All()
	sort.Slice(l, func(x, y int) bool {
		if l[x].Pos != l[y].Pos {
			return l[x].Pos < l[y].Pos
		}
		return l[x].ID < l[y].ID
	})
	return l
}

func nextRulePos() int64 {
	var m int64
	rules.Scan(func(r *Rule) bool {
		if r.Pos > m {
			m = r.Pos
		}
		return true
	})
	return m + 1
}

// moveRule меняет позиции соседних элементов (по порядку отсортированного списка).
func moveRule(list []Rule, id int64, dir string) {
	idx := -1
	for k := range list {
		if list[k].ID == id {
			idx = k
		}
	}
	j := idx - 1
	if dir == "down" {
		j = idx + 1
	}
	if idx < 0 || j < 0 || j >= len(list) {
		return
	}
	// нормализуем позиции, чтобы дыры и дубли не мешали
	for k := range list {
		list[k].Pos = int64(k + 1)
	}
	list[idx].Pos, list[j].Pos = list[j].Pos, list[idx].Pos
	for _, r := range list {
		pos := r.Pos
		rules.Update(r.ID, func(x *Rule) { x.Pos = pos })
	}
}

func registerRules(a *core.App) {
	// правила сайта — статический список (sys/add/rules.txt)
	if rules.Len() == 0 {
		if raw, err := seedFS.ReadFile("seed/rules.txt"); err == nil {
			pos := int64(0)
			for _, ln := range strings.Split(string(raw), "\n") {
				ln = strings.TrimSpace(ln)
				if ln == "" {
					continue
				}
				pos++
				rules.Insert(&Rule{Type: "msg", Msg: ln, Time: core.Now(), IDUser: 1, Pos: pos})
			}
		}
	}

	// «Правила сайта» (rules.php): те же пункты, нумерованный список
	a.GET("/api/rules/site", func(c *core.Ctx) error {
		var items []string
		for _, r := range sortedRules() {
			if r.Type == "msg" {
				items = append(items, a.Text.Plain(r.Msg, 5000))
			}
		}
		return c.JSON(map[string]any{"items": items})
	})

	a.GET("/api/rules", func(c *core.Ctx) error {
		me := c.User()
		var items []map[string]any
		for _, r := range sortedRules() {
			m := map[string]any{"id": r.ID, "type": r.Type, "pos": r.Pos, "title": r.Title, "url": r.URL, "name_url": r.NameURL, "msg": r.Msg, "msg_html": a.Text.Render(r.Msg)}
			items = append(items, m)
		}
		return c.JSON(map[string]any{"items": items, "admin": me != nil && me.Level > 2})
	})
	a.POST("/api/rules", func(c *core.Ctx) error {
		if err := rulesAdmin(c); err != nil {
			return err
		}
		var in struct {
			Type    string `json:"type"`
			Title   string `json:"title"`
			Msg     string `json:"msg"`
			URL     string `json:"url"`
			NameURL string `json:"name_url"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		r := Rule{Type: in.Type, Time: core.Now(), IDUser: c.UID(), Pos: nextRulePos()}
		switch in.Type {
		case "razd":
			r.Title = strings.TrimSpace(in.Title)
			if err := lenOK(r.Title, 1, 60, "Название пункта"); err != nil {
				return err
			}
		case "msg":
			r.Msg = a.Text.Clean(strings.TrimSpace(in.Msg))
			if err := lenOK(r.Msg, 2, 1024, "Текст"); err != nil {
				return err
			}
		case "link":
			r.URL, r.NameURL = strings.TrimSpace(in.URL), strings.TrimSpace(in.NameURL)
			if r.URL == "" || r.NameURL == "" {
				return core.ErrBad("Укажите название и адрес ссылки")
			}
		default:
			return core.ErrBad("Неверный тип пункта")
		}
		return c.OK("id", rules.Insert(&r))
	}, core.Auth)
	a.PUT("/api/rules/{id}", func(c *core.Ctx) error {
		if err := rulesAdmin(c); err != nil {
			return err
		}
		r, ok := rules.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		var in struct{ Title, Msg, URL, NameURL string }
		var raw struct {
			Title   string `json:"title"`
			Msg     string `json:"msg"`
			URL     string `json:"url"`
			NameURL string `json:"name_url"`
		}
		_ = in
		if err := c.Bind(&raw); err != nil {
			return err
		}
		rules.Update(r.ID, func(x *Rule) {
			switch x.Type {
			case "razd":
				x.Title = strings.TrimSpace(raw.Title)
			case "msg":
				x.Msg = a.Text.Clean(strings.TrimSpace(raw.Msg))
			case "link":
				x.URL, x.NameURL = strings.TrimSpace(raw.URL), strings.TrimSpace(raw.NameURL)
			}
		})
		return c.OK()
	}, core.Auth)
	a.DELETE("/api/rules/{id}", func(c *core.Ctx) error {
		if err := rulesAdmin(c); err != nil {
			return err
		}
		id := c.PathInt("id")
		if !rules.Delete(id) {
			return core.ErrNotFound()
		}
		rulePosts.DeleteBy("rule", i(id))
		return c.OK()
	}, core.Auth)
	a.POST("/api/rules/{id}/move", func(c *core.Ctx) error {
		if err := rulesAdmin(c); err != nil {
			return err
		}
		var in struct {
			Dir string `json:"dir"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		moveRule(sortedRules(), c.PathInt("id"), in.Dir)
		return c.OK()
	}, core.Auth)

	// раздел с постами
	a.GET("/api/rules/{id}", func(c *core.Ctx) error {
		r, ok := rules.Get(c.PathInt("id"))
		if !ok || r.Type != "razd" {
			return core.ErrNotFound("Пункт не найден")
		}
		me := c.User()
		ps := rulePosts.By("rule", i(r.ID))
		sort.Slice(ps, func(x, y int) bool { return ps[x].Pos < ps[y].Pos || (ps[x].Pos == ps[y].Pos && ps[x].ID < ps[y].ID) })
		items := make([]map[string]any, 0, len(ps))
		for _, p := range ps {
			items = append(items, map[string]any{"id": p.ID, "msg": p.Msg, "msg_html": a.Text.Render(p.Msg), "pos": p.Pos, "time": p.Time, "user": a.Ref(p.IDUser)})
		}
		return c.JSON(map[string]any{"id": r.ID, "title": r.Title, "items": items, "admin": me != nil && me.Level > 2})
	})
	a.POST("/api/rules/{id}/posts", func(c *core.Ctx) error {
		if err := rulesAdmin(c); err != nil {
			return err
		}
		r, ok := rules.Get(c.PathInt("id"))
		if !ok || r.Type != "razd" {
			return core.ErrNotFound()
		}
		var in struct {
			Msg string `json:"msg"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		msg := a.Text.Clean(strings.TrimSpace(in.Msg))
		if err := lenOK(msg, 2, 99999, "Сообщение"); err != nil {
			return err
		}
		if rulePosts.Count(func(p *RulePost) bool { return p.IDNews == r.ID && p.IDUser == c.UID() && p.Msg == msg }) > 0 {
			return core.ErrBad("Ваше сообщение повторяет предыдущее")
		}
		var m int64
		for _, p := range rulePosts.By("rule", i(r.ID)) {
			m = max(m, p.Pos)
		}
		return c.OK("id", rulePosts.Insert(&RulePost{IDUser: c.UID(), Msg: msg, Time: core.Now(), IDNews: r.ID, Pos: m + 1}))
	}, core.Auth)
	a.PUT("/api/rules-post/{id}", func(c *core.Ctx) error {
		if err := rulesAdmin(c); err != nil {
			return err
		}
		var in struct {
			Msg string `json:"msg"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		msg := a.Text.Clean(strings.TrimSpace(in.Msg))
		if err := lenOK(msg, 2, 99999, "Сообщение"); err != nil {
			return err
		}
		if _, ok := rulePosts.Update(c.PathInt("id"), func(p *RulePost) { p.Msg = msg }); !ok {
			return core.ErrNotFound()
		}
		return c.OK()
	}, core.Auth)
	a.DELETE("/api/rules-post/{id}", func(c *core.Ctx) error {
		if err := rulesAdmin(c); err != nil {
			return err
		}
		p, ok := rulePosts.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		if au, ok := a.GetUser(p.IDUser); ok && au.Level >= c.User().Level && au.ID != c.UID() {
			return core.ErrForbidden("Недостаточно прав")
		}
		rulePosts.Delete(p.ID)
		return c.OK()
	}, core.Auth)
	a.POST("/api/rules-post/{id}/move", func(c *core.Ctx) error {
		if err := rulesAdmin(c); err != nil {
			return err
		}
		var in struct {
			Dir string `json:"dir"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		p, ok := rulePosts.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		ps := rulePosts.By("rule", i(p.IDNews))
		sort.Slice(ps, func(x, y int) bool { return ps[x].Pos < ps[y].Pos || (ps[x].Pos == ps[y].Pos && ps[x].ID < ps[y].ID) })
		idx := -1
		for k := range ps {
			ps[k].Pos = int64(k + 1)
			if ps[k].ID == p.ID {
				idx = k
			}
		}
		j := idx - 1
		if in.Dir == "down" {
			j = idx + 1
		}
		if idx >= 0 && j >= 0 && j < len(ps) {
			ps[idx].Pos, ps[j].Pos = ps[j].Pos, ps[idx].Pos
		}
		for _, x := range ps {
			pos := x.Pos
			rulePosts.Update(x.ID, func(y *RulePost) { y.Pos = pos })
		}
		return c.OK()
	}, core.Auth)
	_ = store.Key
}
