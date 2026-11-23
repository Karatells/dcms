package forum

import (
	"sort"
	"strings"

	"dcms/internal/core"
	"dcms/internal/model"
)

func visibleThems(c *core.Ctx, pred func(*Them) bool) []Them {
	hidden := map[int64]bool{}
	if !isStaff(c.User()) {
		for _, f := range forums.Find(func(f *Forum) bool { return f.Adm }) {
			hidden[f.ID] = true
		}
	}
	return thems.Find(func(t *Them) bool { return !hidden[t.IDForum] && pred(t) })
}

func themeRow(a *core.App, t Them) map[string]any {
	m := themeSummary(a, t)
	if f, ok := forums.Get(t.IDForum); ok {
		m["forum"] = map[string]any{"id": f.ID, "name": f.Name}
	}
	if r, ok := razdels.Get(t.IDRazdel); ok {
		m["razdel"] = map[string]any{"id": r.ID, "name": r.Name}
	}
	m["link"] = themeLink(t)
	return m
}

func pageThemes(a *core.App, c *core.Ctx, list []Them) error {
	p := c.Page()
	items := []map[string]any{}
	for x := p.Offset; x < min(p.Offset+p.Per, len(list)); x++ {
		items = append(items, themeRow(a, list[x]))
	}
	return c.List(items, len(list), p)
}

func registerLists(a *core.App) {
	// мои темы
	a.GET("/api/forum-my", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if err := forumBan(a, me); err != nil {
			return err
		}
		list := visibleThems(c, func(t *Them) bool { return t.IDUser == me.ID })
		sort.Slice(list, func(x, y int) bool { return list[x].Time > list[y].Time })
		return pageThemes(a, c, list)
	}, core.Auth)

	// новые: kind=posts (по последней активности) | themes (по дате создания)
	a.GET("/api/forum-new", func(c *core.Ctx) error {
		if err := forumBan(a, c.User()); err != nil {
			return err
		}
		list := visibleThems(c, func(t *Them) bool { return true })
		if c.Q("kind") == "themes" {
			sort.Slice(list, func(x, y int) bool { return list[x].TimeCreate > list[y].TimeCreate })
		} else {
			sort.Slice(list, func(x, y int) bool { return list[x].Time > list[y].Time })
		}
		return pageThemes(a, c, list)
	})

	// кто на форуме
	a.GET("/api/forum-online", func(c *core.Ctx) error {
		now := core.Now()
		list := a.T.Users.Find(func(u *model.User) bool { return u.DateLast > now-600 && strings.HasPrefix(u.URL, "/forum") })
		sort.Slice(list, func(x, y int) bool { return list[x].DateLast > list[y].DateLast })
		p := c.Page()
		items := []map[string]any{}
		for x := p.Offset; x < min(p.Offset+p.Per, len(list)); x++ {
			items = append(items, map[string]any{"user": a.Ref(list[x].ID), "date_last": list[x].DateLast})
		}
		return c.List(items, len(list), p)
	})

	// поиск: text, in = "f<id>" | "r<id>" | ""
	a.POST("/api/forum-search", func(c *core.Ctx) error {
		if err := forumBan(a, c.User()); err != nil {
			return err
		}
		var in struct{ Text, In string }
		if err := c.Bind(&in); err != nil {
			return err
		}
		words := searchWords(in.Text)
		if len(words) == 0 {
			return core.ErrBad("Слишком короткий запрос (минимум 3 символа)")
		}
		var fid, rid int64
		if len(in.In) > 1 {
			n := int64(atoi(in.In[1:]))
			if in.In[0] == 'f' {
				fid = n
			} else if in.In[0] == 'r' {
				rid = n
			}
		}
		scope := func(t *Them) bool {
			return (fid == 0 || t.IDForum == fid) && (rid == 0 || t.IDRazdel == rid)
		}
		cands := visibleThems(c, scope)
		type hit struct {
			t     Them
			score int
			post  int64
			text  string
		}
		var hits []hit
		for _, t := range cands {
			h := hit{t: t}
			low := strings.ToLower(t.Name + " " + t.Text)
			for _, w := range words {
				if strings.Contains(low, w) {
					h.score += 3
				}
			}
			for _, p := range posts.By("them", i(t.ID)) {
				pl := strings.ToLower(p.Msg)
				n := 0
				for _, w := range words {
					if strings.Contains(pl, w) {
						n++
					}
				}
				if n > 0 {
					h.score += n
					if h.post == 0 || n == len(words) {
						h.post, h.text = p.ID, p.Msg
					}
				}
			}
			if h.score > 0 {
				hits = append(hits, h)
			}
		}
		sort.Slice(hits, func(x, y int) bool {
			return hits[x].score > hits[y].score || hits[x].score == hits[y].score && hits[x].t.Time > hits[y].t.Time
		})
		if len(hits) > 100 {
			hits = hits[:100]
		}
		p := c.Page()
		items := []map[string]any{}
		for x := p.Offset; x < min(p.Offset+p.Per, len(hits)); x++ {
			h := hits[x]
			m := themeRow(a, h.t)
			if h.post > 0 {
				m["post"] = h.post
				m["snippet"] = a.Text.Plain(h.text, 160)
			}
			items = append(items, m)
		}
		return c.List(items, len(hits), p)
	})
}

// searchWords — слова запроса в нижнем регистре; у длинных слов отсекается окончание (упрощённое стеммирование).
func searchWords(q string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(strings.ToLower(q), func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '.' }) {
		r := []rune(w)
		if len(r) < 3 {
			continue
		}
		if len(r) > 5 {
			r = r[:len(r)-2]
		} else if len(r) > 4 {
			r = r[:len(r)-1]
		}
		out = append(out, string(r))
	}
	return out
}
