package forum

import (
	"strings"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

type pollIn struct {
	Text    string   `json:"text"`
	Options []string `json:"options"`
	Close   bool     `json:"close"`
}

func cleanOptions(in *pollIn) []string {
	out := []string{}
	for _, o := range in.Options {
		if o = strings.TrimSpace(o); o != "" {
			out = append(out, o)
		}
	}
	return out
}

func validatePoll(a *core.App, in *pollIn) error {
	in.Text = a.Text.Clean(strings.TrimSpace(in.Text))
	if err := lenOK(in.Text, 3, 42, "Тема опроса"); err != nil {
		return err
	}
	opts := cleanOptions(in)
	if len(opts) < 2 {
		return core.ErrBad("Заполните минимум два варианта опроса")
	}
	if len(opts) > 6 {
		return core.ErrBad("Не больше шести вариантов")
	}
	for n, o := range opts {
		if core.RuneLen(o) > 23 {
			return core.ErrBad("Вариант № " + store.I(int64(n+1)) + " слишком длинный (максимум 23 символа)")
		}
		if a.Text.Censor(o) != o {
			return core.ErrBad("В варианте № " + store.I(int64(n+1)) + " обнаружен мат")
		}
	}
	return nil
}

// savePoll заменяет варианты, сохраняя голоса за варианты с теми же номерами.
func savePoll(t *Them, in *pollIn) {
	opts := cleanOptions(in)
	cur := voteOpts.By("them", i(t.ID))
	for n, o := range opts {
		num := int64(n + 1)
		found := false
		for _, c := range cur {
			if c.Num == num {
				voteOpts.Update(c.ID, func(x *VoteOpt) { x.Var = o })
				found = true
			}
		}
		if !found {
			voteOpts.Insert(&VoteOpt{Them: t.ID, Var: o, Num: num})
		}
	}
	for _, c := range cur {
		if int(c.Num) > len(opts) {
			voteOpts.Delete(c.ID)
			voteUser.DeleteWhere(func(v *VoteUser) bool { return v.Them == t.ID && v.Var == c.Num })
		}
	}
	thems.Update(t.ID, func(x *Them) { x.Vote = in.Text; x.VoteClose = in.Close })
	t.Vote, t.VoteClose = in.Text, in.Close
}

func pollView(a *core.App, t Them, me *model.User) map[string]any {
	if t.Vote == "" {
		return nil
	}
	opts := voteOpts.By("them", i(t.ID))
	for x := 1; x < len(opts); x++ {
		for y := x; y > 0 && opts[y].Num < opts[y-1].Num; y-- {
			opts[y], opts[y-1] = opts[y-1], opts[y]
		}
	}
	votes := voteUser.By("them", i(t.ID))
	total := len(votes)
	var mine int64
	counts := map[int64]int{}
	for _, v := range votes {
		counts[v.Var]++
		if me != nil && v.IDUser == me.ID {
			mine = v.Var
		}
	}
	vs := []map[string]any{}
	for _, o := range opts {
		pct := 0
		if total > 0 {
			pct = counts[o.Num] * 100 / total
		}
		vs = append(vs, map[string]any{"num": o.Num, "text": a.Text.Render(o.Var), "plain": o.Var, "count": counts[o.Num], "percent": pct})
	}
	return map[string]any{"text": a.Text.Render(t.Vote), "plain": t.Vote, "closed": t.VoteClose, "total": total, "my": mine, "options": vs,
		"can_vote": me != nil && mine == 0 && !t.VoteClose}
}

func registerPoll(a *core.App) {
	theme := func(c *core.Ctx) (Them, error) {
		t, ok := thems.Get(c.PathInt("t"))
		if !ok {
			return t, core.ErrNotFound("Тема не найдена")
		}
		if f, ok := forums.Get(t.IDForum); !ok || !visibleForum(f, c.User()) {
			return t, core.ErrNotFound("Тема не найдена")
		}
		return t, nil
	}

	a.PUT("/api/forum-theme/{t}/poll", func(c *core.Ctx) error {
		if _, err := c.Need(); err != nil {
			return err
		}
		t, err := theme(c)
		if err != nil {
			return err
		}
		if !canEditTheme(c, t) {
			return core.ErrForbidden()
		}
		var in pollIn
		if err := c.Bind(&in); err != nil {
			return err
		}
		if err := validatePoll(a, &in); err != nil {
			return err
		}
		savePoll(&t, &in)
		return c.OK()
	}, core.Auth)

	a.DELETE("/api/forum-theme/{t}/poll", func(c *core.Ctx) error {
		if _, err := c.Need(); err != nil {
			return err
		}
		t, err := theme(c)
		if err != nil {
			return err
		}
		if !canEditTheme(c, t) {
			return core.ErrForbidden()
		}
		thems.Update(t.ID, func(x *Them) { x.Vote, x.VoteClose = "", false })
		voteOpts.DeleteBy("them", i(t.ID))
		voteUser.DeleteBy("them", i(t.ID))
		return c.OK()
	}, core.Auth)

	a.POST("/api/forum-theme/{t}/vote/{num}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if err := forumBan(a, me); err != nil {
			return err
		}
		t, err := theme(c)
		if err != nil {
			return err
		}
		if t.Vote == "" || t.VoteClose {
			return core.ErrBad("Голосование недоступно")
		}
		num := c.PathInt("num")
		ok := false
		for _, o := range voteOpts.By("them", i(t.ID)) {
			if o.Num == num {
				ok = true
			}
		}
		if !ok {
			return core.ErrBad("Нет такого варианта")
		}
		for _, v := range voteUser.By("them", i(t.ID)) {
			if v.IDUser == me.ID {
				return core.ErrBad("Вы уже голосовали")
			}
		}
		voteUser.Insert(&VoteUser{Them: t.ID, Var: num, IDUser: me.ID, Time: core.Now()})
		return c.JSON(pollView(a, t, me))
	}, core.Auth)

	a.GET("/api/forum-theme/{t}/vote/{num}/users", func(c *core.Ctx) error {
		t, err := theme(c)
		if err != nil {
			return err
		}
		num := c.PathInt("num")
		p := c.Page()
		list, total := voteUser.Select(store.Opts[VoteUser]{Index: "them", Key: i(t.ID), Where: func(v *VoteUser) bool { return v.Var == num }, Desc: false, Offset: p.Offset, Limit: p.Per})
		items := make([]map[string]any, 0, len(list))
		for _, v := range list {
			items = append(items, map[string]any{"user": a.Ref(v.IDUser), "time": v.Time})
		}
		return c.List(items, total, p)
	})
}
