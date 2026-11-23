package extras

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

type GiftCat struct {
	store.Base
	Name string `json:"name"`
	Icon string `json:"icon"`
}
type Gift struct {
	store.Base
	IDCategory int64  `json:"id_category"`
	Name       string `json:"name"`
	Money      int64  `json:"money"`
	Emoji      string `json:"emoji"`
	HasImg     bool   `json:"has_img"`
}
type GiftUser struct {
	store.Base
	IDUser int64  `json:"id_user"` // получатель
	IDAnk  int64  `json:"id_ank"`  // отправитель
	IDGift int64  `json:"id_gift"`
	Time   int64  `json:"time"`
	Coment string `json:"coment"`
	Status int64  `json:"status"`
}

var (
	giftCats  *store.Table[GiftCat, *GiftCat]
	giftList  *store.Table[Gift, *Gift]
	giftsUser *store.Table[GiftUser, *GiftUser]
)

func giftImg(a *core.App, id int64) string { return filepath.Join(a.UploadDir("gifts"), i(id)+".png") }

func giftView(g Gift) map[string]any {
	m := map[string]any{"id": g.ID, "name": g.Name, "money": g.Money, "emoji": g.Emoji, "id_category": g.IDCategory}
	if g.HasImg {
		m["img"] = "/api/gifts/img/" + i(g.ID)
	}
	return m
}

func registerGifts(a *core.App) {
	giftCats = store.NewTable[GiftCat](a.DB, "gift_categories")
	giftList = store.NewTable[Gift](a.DB, "gift_list").Index("cat", func(g *Gift) string { return i(g.IDCategory) })
	giftsUser = store.NewTable[GiftUser](a.DB, "gifts_user").Index("user", func(g *GiftUser) string { return i(g.IDUser) }).Index("gift", func(g *GiftUser) string { return i(g.IDGift) })

	if giftCats.Len() == 0 {
		seed := []struct {
			Cat, Icon string
			Items     [][3]string
		}{
			{"Праздничные", "gift-outline", [][3]string{{"Торт", "🎂", "5"}, {"Шампанское", "🍾", "8"}, {"Подарок", "🎁", "3"}, {"Воздушные шарики", "🎈", "2"}, {"Салют", "🎆", "6"}}},
			{"Милые", "heart-outline", [][3]string{{"Сердечко", "❤️", "2"}, {"Плюшевый мишка", "🧸", "6"}, {"Поцелуй", "💋", "3"}, {"Котёнок", "🐱", "4"}}},
			{"Цветы", "flower-outline", [][3]string{{"Роза", "🌹", "4"}, {"Букет", "💐", "7"}, {"Тюльпан", "🌷", "3"}, {"Подсолнух", "🌻", "3"}}},
		}
		for _, s := range seed {
			cid := giftCats.Insert(&GiftCat{Name: s.Cat, Icon: s.Icon})
			for _, it := range s.Items {
				var price int64
				for _, ch := range it[2] {
					price = price*10 + int64(ch-'0')
				}
				giftList.Insert(&Gift{IDCategory: cid, Name: it[0], Emoji: it[1], Money: price})
			}
		}
	}

	a.ProfileStat(func(a *core.App, viewer, target *model.User) []core.ProfileItem {
		n := giftsUser.Count(func(g *GiftUser) bool { return g.IDUser == target.ID && g.Status == 1 })
		return []core.ProfileItem{{Key: "gifts", Title: "Подарки", Icon: "gift", Link: "/user/gift/" + i(target.ID), Count: int64(n)}}
	})
	a.OnUserDelete(func(uid int64) {
		giftsUser.DeleteWhere(func(g *GiftUser) bool { return g.IDUser == uid || g.IDAnk == uid })
	})

	a.GET("/api/gifts/img/{id}", func(c *core.Ctx) error {
		raw, err := os.ReadFile(giftImg(a, c.PathInt("id")))
		if err != nil {
			return core.ErrNotFound()
		}
		return c.ServeBytes(raw, http.DetectContentType(raw), true)
	})

	// подарки пользователя
	a.GET("/api/gifts/user/{uid}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		uid := c.PathInt("uid")
		u, ok := a.GetUser(uid)
		if !ok {
			return core.ErrNotFound("Пользователь не найден")
		}
		mine := uid == me.ID
		p := c.Page()
		list, total := giftsUser.Select(store.Opts[GiftUser]{Index: "user", Key: i(uid), Desc: true, Offset: p.Offset, Limit: p.Per,
			Where: func(g *GiftUser) bool { return mine || g.Status == 1 }, Less: func(x, y *GiftUser) bool { return x.Time < y.Time }})
		items := make([]map[string]any, 0, len(list))
		for _, g := range list {
			gf, _ := giftList.Get(g.IDGift)
			v := giftView(gf)
			items = append(items, map[string]any{"id": g.ID, "gift": v, "from": a.Ref(g.IDAnk), "time": g.Time, "status": g.Status, "can_reply": g.IDAnk != me.ID})
		}
		return c.JSON(map[string]any{"items": items, "total": total, "page": p.Page, "per": p.Per, "pages": max(1, (total+p.Per-1)/p.Per), "owner": a.Ref(u.ID), "mine": mine})
	}, core.Auth)

	// каталог
	a.GET("/api/gifts/catalog", func(c *core.Ctx) error {
		cs := giftCats.All()
		sort.Slice(cs, func(x, y int) bool { return cs[x].ID < cs[y].ID })
		items := make([]map[string]any, 0, len(cs))
		for _, k := range cs {
			items = append(items, map[string]any{"id": k.ID, "name": k.Name, "icon": k.Icon, "count": giftList.CountBy("cat", i(k.ID))})
		}
		me := c.User()
		return c.JSON(map[string]any{"items": items, "admin": me != nil && me.Level > 2, "currency": a.Cfg.Str("money_name")})
	})
	a.GET("/api/gifts/catalog/{cat}", func(c *core.Ctx) error {
		k, ok := giftCats.Get(c.PathInt("cat"))
		if !ok {
			return core.ErrNotFound("Нет такой категории")
		}
		gs := giftList.By("cat", i(k.ID))
		sort.Slice(gs, func(x, y int) bool { return gs[x].ID < gs[y].ID })
		items := make([]map[string]any, 0, len(gs))
		for _, g := range gs {
			items = append(items, giftView(g))
		}
		me := c.User()
		return c.JSON(map[string]any{"items": items, "category": map[string]any{"id": k.ID, "name": k.Name}, "admin": me != nil && me.Level > 2, "currency": a.Cfg.Str("money_name")})
	})

	// отправить
	a.POST("/api/gifts/send", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		var in struct {
			To   int64  `json:"to"`
			Gift int64  `json:"gift"`
			Msg  string `json:"msg"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if _, ok := a.GetUser(in.To); !ok || in.To == me.ID {
			return core.ErrNotFound("Получатель не найден")
		}
		g, ok := giftList.Get(in.Gift)
		if !ok {
			return core.ErrNotFound("Подарок не найден")
		}
		msg := a.Text.Clean(strings.TrimSpace(in.Msg))
		if len([]rune(msg)) > 150 {
			return core.ErrBad("Комментарий не длиннее 150 символов")
		}
		if !spend(a, me.ID, g.Money) {
			return core.ErrBad("У вас недостаточно средств на счету")
		}
		id := giftsUser.Insert(&GiftUser{IDUser: in.To, IDAnk: me.ID, IDGift: g.ID, Coment: msg, Time: core.Now(), Status: 1})
		a.Notify(me.ID, in.To, "new_gift", id, false)
		return c.OK("id", id)
	}, core.Auth)

	// конкретный подарок
	a.GET("/api/gifts/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		g, ok := giftsUser.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Подарок не найден")
		}
		if g.Status == 0 && g.IDUser != me.ID && g.IDAnk != me.ID {
			return core.ErrForbidden()
		}
		gf, _ := giftList.Get(g.IDGift)
		return c.JSON(map[string]any{"id": g.ID, "gift": giftView(gf), "from": a.Ref(g.IDAnk), "to": a.Ref(g.IDUser), "time": g.Time, "msg": a.Text.Render(g.Coment), "status": g.Status,
			"mine": g.IDUser == me.ID, "can_delete": g.IDUser == me.ID || me.Level > 2})
	}, core.Auth)
	answer := func(ok bool) core.Handler {
		return func(c *core.Ctx) error {
			me, err := c.Need()
			if err != nil {
				return err
			}
			g, found := giftsUser.Get(c.PathInt("id"))
			if !found || g.IDUser != me.ID || g.Status != 0 {
				return core.ErrNotFound()
			}
			if ok {
				giftsUser.Update(g.ID, func(x *GiftUser) { x.Status = 1 })
				a.Notify(me.ID, g.IDAnk, "ok_gift", g.IDGift, false)
			} else {
				giftsUser.Delete(g.ID)
				a.Notify(me.ID, g.IDAnk, "no_gift", g.IDGift, false)
			}
			return c.OK()
		}
	}
	a.POST("/api/gifts/{id}/accept", answer(true), core.Auth)
	a.POST("/api/gifts/{id}/decline", answer(false), core.Auth)
	a.DELETE("/api/gifts/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		g, ok := giftsUser.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		if g.IDUser != me.ID && me.Level <= 2 {
			return core.ErrForbidden()
		}
		giftsUser.Delete(g.ID)
		return c.OK()
	}, core.Auth)

	// ── управление (only_level(3))
	adm := func(c *core.Ctx) error { return c.NeedLevel(3) }
	a.POST("/api/gifts/cat", func(c *core.Ctx) error {
		if err := adm(c); err != nil {
			return err
		}
		var in struct{ Name, Icon string }
		var b struct {
			Name string `json:"name"`
			Icon string `json:"icon"`
		}
		_ = in
		if err := c.Bind(&b); err != nil {
			return err
		}
		b.Name = strings.TrimSpace(b.Name)
		if n := len([]rune(b.Name)); n < 2 || n > 128 {
			return core.ErrBad("Название от 2 до 128 символов")
		}
		if b.Icon == "" {
			b.Icon = "folder-outline"
		}
		return c.OK("id", giftCats.Insert(&GiftCat{Name: b.Name, Icon: b.Icon}))
	}, core.Auth)
	a.PUT("/api/gifts/cat/{id}", func(c *core.Ctx) error {
		if err := adm(c); err != nil {
			return err
		}
		var b struct {
			Name string `json:"name"`
			Icon string `json:"icon"`
		}
		if err := c.Bind(&b); err != nil {
			return err
		}
		b.Name = strings.TrimSpace(b.Name)
		if n := len([]rune(b.Name)); n < 2 || n > 128 {
			return core.ErrBad("Название от 2 до 128 символов")
		}
		if _, ok := giftCats.Update(c.PathInt("id"), func(x *GiftCat) {
			x.Name = b.Name
			if b.Icon != "" {
				x.Icon = b.Icon
			}
		}); !ok {
			return core.ErrNotFound()
		}
		return c.OK()
	}, core.Auth)
	a.DELETE("/api/gifts/cat/{id}", func(c *core.Ctx) error {
		if err := adm(c); err != nil {
			return err
		}
		id := c.PathInt("id")
		for _, g := range giftList.By("cat", i(id)) {
			delGift(a, g.ID)
		}
		if !giftCats.Delete(id) {
			return core.ErrNotFound()
		}
		return c.OK()
	}, core.Auth)
	// создание подарка: multipart (name, money, emoji, category, gift=png) или JSON-подобные поля формы
	a.POST("/api/gifts/item", func(c *core.Ctx) error {
		if err := adm(c); err != nil {
			return err
		}
		gMB := int64(a.Cfg.Int("max_upload_gift_mb"))
		if gMB <= 0 {
			gMB = 2
		}
		up, err := c.FormFile("gift", gMB<<20)
		if err != nil {
			return err
		}
		cat := formInt(c, "category")
		if _, ok := giftCats.Get(cat); !ok {
			return core.ErrNotFound("Нет такой категории")
		}
		name, emoji := strings.TrimSpace(c.FormValue("name")), strings.TrimSpace(c.FormValue("emoji"))
		money := formInt(c, "money")
		if money < 1 {
			return core.ErrBad("Укажите стоимость подарка")
		}
		if n := len([]rune(name)); n < 2 || n > 128 {
			return core.ErrBad("Название от 2 до 128 символов")
		}
		if up == nil && emoji == "" {
			return core.ErrBad("Загрузите картинку или укажите эмодзи")
		}
		g := Gift{IDCategory: cat, Name: name, Money: money, Emoji: emoji}
		if up != nil {
			if up.Ext != "png" && up.Ext != "gif" && up.Ext != "jpg" && up.Ext != "jpeg" {
				return core.ErrBad("Неверный формат изображения")
			}
			g.HasImg = true
		}
		id := giftList.Insert(&g)
		if up != nil {
			if err := core.WriteFile(giftImg(a, id), up.Data); err != nil {
				giftList.Delete(id)
				return core.ErrBad("Не удалось сохранить файл")
			}
		}
		return c.OK("id", id)
	}, core.Auth)
	a.PUT("/api/gifts/item/{id}", func(c *core.Ctx) error {
		if err := adm(c); err != nil {
			return err
		}
		var b struct {
			Name  string `json:"name"`
			Money int64  `json:"money"`
			Emoji string `json:"emoji"`
			Cat   int64  `json:"category"`
		}
		if err := c.Bind(&b); err != nil {
			return err
		}
		b.Name = strings.TrimSpace(b.Name)
		if n := len([]rune(b.Name)); n < 2 || n > 128 {
			return core.ErrBad("Название от 2 до 128 символов")
		}
		if b.Money < 1 {
			return core.ErrBad("Укажите стоимость подарка")
		}
		if _, ok := giftList.Update(c.PathInt("id"), func(x *Gift) {
			x.Name, x.Money, x.Emoji = b.Name, b.Money, b.Emoji
			if _, ok := giftCats.Get(b.Cat); ok {
				x.IDCategory = b.Cat
			}
		}); !ok {
			return core.ErrNotFound()
		}
		return c.OK()
	}, core.Auth)
	a.DELETE("/api/gifts/item/{id}", func(c *core.Ctx) error {
		if err := adm(c); err != nil {
			return err
		}
		if !giftList.Has(c.PathInt("id")) {
			return core.ErrNotFound()
		}
		delGift(a, c.PathInt("id"))
		return c.OK()
	}, core.Auth)
}

func formInt(c *core.Ctx, name string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(c.FormValue(name)), 10, 64)
	return n
}

func delGift(a *core.App, id int64) {
	core.RemoveFile(giftImg(a, id))
	giftsUser.DeleteBy("gift", i(id))
	giftList.Delete(id)
}
