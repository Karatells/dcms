package extras

import (
	"math/rand"
	"strings"
	"unicode"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

const ballsPerCoin = 500

func cardOf(uid int64) string {
	if c, ok := cards.OneBy("user", i(uid)); ok {
		return c.Number
	}
	for {
		num := "4444"
		for k := 0; k < 12; k++ {
			num += string(rune('0' + rand.Intn(10)))
		}
		if _, dup := cards.OneBy("num", num); dup {
			continue
		}
		cards.Insert(&Card{IDUser: uid, Number: num})
		return num
	}
}

func registerMoney(a *core.App) {
	a.Counter("liders", func(a *core.App, u *model.User) string {
		now := core.Now()
		total := liders.Count(func(l *Lider) bool { return l.Time > now })
		since := now - 86400
		if u != nil && u.DateLast > 0 {
			since = u.DateLast
		}
		n := liders.Count(func(l *Lider) bool { return l.Time > now && l.TimeP > since })
		if n > 0 {
			return i(int64(total)) + " +" + i(int64(n))
		}
		return i(int64(total))
	})

	a.GET("/api/liders", func(c *core.Ctx) error {
		now := core.Now()
		p := c.Page()
		list, total := liders.Select(store.Opts[Lider]{Where: func(l *Lider) bool { return l.Time > now }, Desc: true, Offset: p.Offset, Limit: p.Per,
			Less: func(x, y *Lider) bool { return x.Stav < y.Stav || (x.Stav == y.Stav && x.TimeP < y.TimeP) }})
		items := make([]map[string]any, 0, len(list))
		for _, l := range list {
			items = append(items, map[string]any{"user": a.Ref(l.IDUser), "stav": l.Stav, "msg": a.Text.Render(l.Msg), "time": l.Time, "bought": l.TimeP})
		}
		me := c.User()
		return c.JSON(map[string]any{"items": items, "total": total, "page": p.Page, "per": p.Per, "pages": max(1, (total+p.Per-1)/p.Per), "currency": a.Cfg.Str("money_name"), "admin": me != nil && me.Level > 2})
	})
	a.DELETE("/api/liders/{uid}", func(c *core.Ctx) error {
		if err := c.NeedLevel(3); err != nil {
			return err
		}
		if liders.DeleteBy("user", i(c.PathInt("uid"))) == 0 {
			return core.ErrNotFound()
		}
		return c.OK()
	}, core.Auth)

	a.GET("/api/money", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		now := core.Now()
		isVIP := me.VIPUntil > now
		rate := int64(ballsPerCoin)
		if isVIP {
			rate = 400
		}
		oc := a.UserSettings(me.ID).Ocenka
		out := map[string]any{
			"balls":          me.Balls,
			"money":          me.Money,
			"currency":       a.Cfg.Str("money_name"),
			"balls_per_coin": rate,
			"is_vip":         isVIP,
			"vip_until":      me.VIPUntil,
			"is_plus5":       oc > now,
			"plus5_until":    oc,
			"card":           cardOf(me.ID),
		}
		return c.JSON(out)
	}, core.Auth)

	// баллы → валюта (для VIP скидка 20%: 400 баллов вместо 500)
	a.POST("/api/money/exchange", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		var in struct {
			N int64 `json:"n"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if in.N < 1 || in.N > 20 {
			return core.ErrBad("Перевод баллов возможен не более чем на 20 за одну операцию")
		}
		rate := int64(ballsPerCoin)
		if me.VIPUntil > core.Now() {
			rate = 400
		}
		cost := in.N * rate
		ok := false
		a.T.Users.Update(me.ID, func(u *model.User) {
			if u.Balls >= cost {
				u.Balls -= cost
				u.Money += in.N
				ok = true
			}
		})
		if !ok {
			return core.ErrBad("Недостаточно баллов")
		}
		return c.OK()
	}, core.Auth)

	registerVIP(a)

	// Оценка 5+
	a.POST("/api/money/plus5", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		var in struct {
			Stav int64 `json:"stav"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if in.Stav < 1 || in.Stav > 7 {
			return core.ErrBad("Ставка от 1 до 7")
		}
		s := a.UserSettings(me.ID)
		if s.Ocenka > core.Now() {
			return core.ErrBad("Услуга уже подключена")
		}
		if !spend(a, me.ID, in.Stav) {
			return core.ErrBad("У вас недостаточно средств")
		}
		until := core.Now() + in.Stav*86400
		a.T.UserSet.Update(s.ID, func(x *model.UserSet) { x.Ocenka = until })
		return c.OK()
	}, core.Auth)

	// перевод по номеру карты
	a.POST("/api/money/transfer", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		var in struct {
			Card   string `json:"card"`
			Amount int64  `json:"amount"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		num := strings.Map(func(r rune) rune {
			if unicode.IsDigit(r) {
				return r
			}
			return -1
		}, in.Card)
		if len(num) < 10 {
			return core.ErrBad("Введите корректный номер карты (16 цифр)")
		}
		dst, ok := cards.OneBy("num", num)
		if !ok {
			return core.ErrBad("Получатель с такой картой не найден")
		}
		if dst.IDUser == me.ID {
			return core.ErrBad("Нельзя переводить монеты самому себе")
		}
		if in.Amount <= 0 {
			return core.ErrBad("Введите сумму больше нуля")
		}
		if !spend(a, me.ID, in.Amount) {
			return core.ErrBad("Недостаточно средств на вашем счету")
		}
		a.T.Users.Update(dst.IDUser, func(u *model.User) { u.Money += in.Amount })
		a.Notify(me.ID, dst.IDUser, "money_in", in.Amount, false)
		return c.OK()
	}, core.Auth)
}
