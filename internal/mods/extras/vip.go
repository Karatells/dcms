package extras

import (
	"math"
	"strings"
	"unicode/utf8"

	"dcms/internal/core"
	"dcms/internal/model"
)

func init() {
	core.Defaults["vip_price_month"] = "50"
}

type vipPlan struct {
	Months   int    `json:"months"`
	Days     int64  `json:"days"`
	Label    string `json:"label"`
	Discount int    `json:"discount"`
	Price    int64  `json:"price"`
	OldPrice int64  `json:"old_price"`
}

var (
	vipBadges     = []string{"👑", "💎", "🔥", "⚡", "🌟", "🦋", "🦊", "🚀", "🎯", "💜"}
	vipNickStyles = []string{"", "gold", "neon", "sunset", "ocean", "emerald", "rainbow"}
	vipCovers     = []string{"", "aurora", "sunset", "cyber", "emerald", "royal", "midnight"}
)

func inSlice(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func vipPlans(a *core.App) []vipPlan {
	base := int64(a.Cfg.Int("vip_price_month"))
	if base < 1 {
		base = 50
	}
	calc := func(months int, days int64, label string, disc int) vipPlan {
		oldP := base * int64(months)
		p := int64(math.Round(float64(oldP) * float64(100-disc) / 100.0))
		if p < 1 {
			p = 1
		}
		return vipPlan{Months: months, Days: days, Label: label, Discount: disc, Price: p, OldPrice: oldP}
	}
	return []vipPlan{
		calc(1, 30, "1 месяц", 0),
		calc(3, 90, "3 месяца", 10),
		calc(6, 180, "6 месяцев", 20),
		calc(12, 365, "1 год", 30),
	}
}

func registerVIP(a *core.App) {
	// ── Состояние VIP, тарифы и настройки
	a.GET("/api/vip", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		now := core.Now()
		active := me.VIPUntil > now
		badge := me.VIPBadge
		if badge == "" {
			badge = "👑"
		}
		return c.JSON(map[string]any{
			"active":       active,
			"until":        me.VIPUntil,
			"money":        me.Money,
			"currency":     a.Cfg.Str("money_name"),
			"plans":        vipPlans(a),
			"badges":       vipBadges,
			"nick_styles":  vipNickStyles,
			"covers":       vipCovers,
			"settings": map[string]any{
				"badge":      badge,
				"nick_style": me.VIPNickStyle,
				"cover":      me.VIPCover,
				"stealth":    me.VIPStealth,
				"top":        me.VIPTop,
				"top_msg":    me.VIPTopMsg,
			},
		})
	}, core.Auth)

	// ── Оформить или продлить VIP-подписку
	a.POST("/api/vip/buy", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		var in struct {
			Months int `json:"months"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		var chosen *vipPlan
		for _, p := range vipPlans(a) {
			if p.Months == in.Months {
				cp := p
				chosen = &cp
				break
			}
		}
		if chosen == nil {
			return core.ErrBad("Выберите корректный период подписки (1, 3, 6 или 12 месяцев)")
		}
		if !spend(a, me.ID, chosen.Price) {
			return core.ErrBad("Недостаточно монет на счёте")
		}
		now := core.Now()
		baseTime := now
		if me.VIPUntil > now {
			baseTime = me.VIPUntil
		}
		until := baseTime + chosen.Days*86400
		a.T.Users.Update(me.ID, func(u *model.User) {
			u.VIPUntil = until
			if u.VIPBadge == "" {
				u.VIPBadge = "👑"
			}
		})
		// Автоматически включаем «Оценку 5+» на весь срок VIP
		us := a.UserSettings(me.ID)
		if us.Ocenka < until {
			a.T.UserSet.Update(us.ID, func(x *model.UserSet) { x.Ocenka = until })
		}
		// Если уже включено продвижение в Топе — продлеваем его до новой даты VIP
		if me.VIPTop {
			if ex, ok := liders.OneBy("user", i(me.ID)); ok {
				liders.Update(ex.ID, func(x *Lider) { x.Time = until })
			}
		}
		a.DropUserCache(me.ID)
		return c.OK("until", until)
	}, core.Auth)

	// ── Настройка привилегий VIP (иконка, стиль ника, обложка, невидимка, вывод в Топ/Знакомства)
	a.PUT("/api/vip/settings", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		now := core.Now()
		if me.VIPUntil <= now {
			return core.ErrForbidden("Настройки доступны только с активной VIP-подпиской")
		}
		var in struct {
			Badge     string `json:"badge"`
			NickStyle string `json:"nick_style"`
			Cover     string `json:"cover"`
			Stealth   bool   `json:"stealth"`
			Top       bool   `json:"top"`
			TopMsg    string `json:"top_msg"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if !inSlice(vipBadges, in.Badge) {
			in.Badge = "👑"
		}
		if !inSlice(vipNickStyles, in.NickStyle) {
			in.NickStyle = ""
		}
		if !inSlice(vipCovers, in.Cover) {
			in.Cover = ""
		}
		topMsg := a.Text.Clean(strings.TrimSpace(in.TopMsg))
		if utf8.RuneCountInString(topMsg) > 215 {
			return core.ErrBad("Подпись для Топа не длиннее 215 символов")
		}
		a.T.Users.Update(me.ID, func(u *model.User) {
			u.VIPBadge = in.Badge
			u.VIPNickStyle = in.NickStyle
			u.VIPCover = in.Cover
			u.VIPStealth = in.Stealth
			u.VIPTop = in.Top
			u.VIPTopMsg = topMsg
		})
		// Синхронизируем анкету с разделом «Лидеры» и «Знакомства»
		if in.Top {
			msg := topMsg
			if msg == "" {
				msg = "VIP-участник сайта"
			}
			if ex, ok := liders.OneBy("user", i(me.ID)); ok {
				liders.Update(ex.ID, func(x *Lider) { x.Stav = 7; x.Msg = msg; x.Time = me.VIPUntil; x.TimeP = now })
			} else {
				liders.Insert(&Lider{IDUser: me.ID, Stav: 7, Msg: msg, Time: me.VIPUntil, TimeP: now})
			}
		} else {
			liders.DeleteBy("user", i(me.ID))
		}
		a.DropUserCache(me.ID)
		return c.OK()
	}, core.Auth)
}
