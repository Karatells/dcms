// Package extras — закладки, лидеры, магазин услуг (деньги), подарки, знакомства.
package extras

import (
	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

type Lider struct {
	store.Base
	IDUser int64  `json:"id_user"`
	Stav   int64  `json:"stav"`
	Msg    string `json:"msg"`
	Time   int64  `json:"time"`   // до какого времени активен
	TimeP  int64  `json:"time_p"` // когда куплен
}
type Card struct {
	store.Base
	IDUser int64  `json:"id_user"`
	Number string `json:"number"`
}

var (
	liders *store.Table[Lider, *Lider]
	cards  *store.Table[Card, *Card]
)

func i(n int64) string { return store.I(n) }

func init() {
	core.Defaults["money_name"] = "монет"
}

// spend списывает валюту, если хватает.
func spend(a *core.App, uid, n int64) bool {
	ok := false
	a.T.Users.Update(uid, func(u *model.User) {
		if u.Money >= n {
			u.Money -= n
			ok = true
		}
	})
	return ok
}

func Register(a *core.App) {
	liders = store.NewTable[Lider](a.DB, "liders").Index("user", func(l *Lider) string { return i(l.IDUser) })
	cards = store.NewTable[Card](a.DB, "user_cards").Index("user", func(c *Card) string { return i(c.IDUser) }).Index("num", func(c *Card) string { return c.Number })
	registerBookmarks(a)
	registerMoney(a)
	registerGifts(a)
	registerLove(a)
	a.OnUserDelete(func(uid int64) {
		liders.DeleteBy("user", i(uid))
		cards.DeleteBy("user", i(uid))
		a.T.Bookmarks.DeleteWhere(func(b *model.Bookmarks) bool { return b.IDUser == uid || (b.Type == "people" && b.IDObject == uid) })
	})
}
