package profile

import (
	"strings"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

func registerProfile(a *core.App) {
	// ---------- анкета пользователя
	a.GET("/api/user/{id}", func(c *core.Ctx) error {
		u, err := userByPath(c)
		if err != nil {
			return err
		}
		viewer := c.User()
		now := core.Now()
		out := map[string]any{
			"user":   a.Ref(u.ID),
			"banned": nil,
			"friend": friendState(a, viewer, u.ID),
		}
		if b, ok := a.ActiveBan(u.ID, "all"); ok {
			out["banned"] = map[string]any{"until": b.Time, "forever": b.Navsegda == 1, "reason": b.Prich}
		}
		st := access(a, viewer, &u)
		out["access"] = map[string]any{"ok": st == vOK, "state": []string{"ok", "friends", "closed"}[st]}
		out["self"] = viewer != nil && viewer.ID == u.ID
		out["date_reg"] = u.DateReg
		out["date_last"] = u.DateLast
		out["rating"] = u.Rating
		out["balls"] = u.Balls
		if st != vOK {
			return c.JSON(out)
		}

		// гости
		if viewer != nil && viewer.ID != u.ID {
			if g, ok := myGuests.OneBy("pair", store.Key(u.ID, viewer.ID)); !ok {
				myGuests.Insert(&MyGuest{IDAnk: u.ID, IDUser: viewer.ID, Time: now, Read: true})
				a.AddBalls(u.ID, 1, 1)
			} else if now-g.Time > 3600 {
				myGuests.Update(g.ID, func(x *MyGuest) { x.Time = now; x.Read = true })
				a.AddRatingTmp(u.ID, 1)
			}
		}

		info := map[string]any{
			"name": u.AnkName, "city": u.AnkCity, "about": u.AnkOSebe, "job": u.AnkZan,
			"day": u.AnkDR, "month": u.AnkMR, "year": u.AnkGR, "age": ageOf(&u),
			"marital": label(maritalLabels, u.AnkMaritalStatus, u.Pol), "baby": label(babyLabels, u.AnkBaby, u.Pol),
			"smok": label(smokLabels, u.AnkSmok, u.Pol), "alko": label(alkoLabels, u.AnkAlkoN, u.Pol), "meet": lovList(&u),
			"skype": u.AnkSkype, "telegram": u.AnkTelegram, "phone": u.AnkNTel,
		}
		if u.ShowMail {
			info["email"] = u.Email
		}
		out["info"] = info
		if u.ShowURL || (viewer != nil && viewer.Level >= 7) {
			out["url"] = u.URL
		}
		if viewer != nil && viewer.Level >= 7 {
			out["staff"] = map[string]any{"ip": u.IP, "ua": u.UA, "browser": u.Browser, "email": u.Email, "money": u.Money, "level": u.Level, "group_id": u.GroupAccess}
		}
		if viewer != nil && viewer.ID == u.ID {
			out["money"] = u.Money
			out["email"] = u.Email
		}
		out["time_on_site"] = u.Time

		// статус
		if ss := statusOf(u.ID); ss != nil {
			sm := map[string]any{"id": ss.ID, "msg": a.Text.Render(ss.Msg), "raw": ss.Msg, "time": ss.Time,
				"likes": stLikes.CountBy("st", store.I(ss.ID)), "komm": stKomm.CountBy("st", store.I(ss.ID))}
			if viewer != nil {
				for _, l := range stLikes.By("st", store.I(ss.ID)) {
					if l.IDUser == viewer.ID {
						sm["my_like"] = l.Like + 1 // 1 не нравится, 2 нравится
					}
				}
			}
			out["status"] = sm
		}
		// стена
		stenaCnt := stena.CountBy("wall", store.I(u.ID))
		items := a.ProfileItems(viewer, &u)
		friendsCnt := a.T.Friends.CountBy("user", store.I(u.ID))
		out["stena_count"] = stenaCnt
		out["items"] = items
		out["friends_count"] = friendsCnt
		out["achievements"] = computeAchievements(&u, items, friendsCnt, stenaCnt)
		// закладка
		if viewer != nil && viewer.ID != u.ID {
			_, fav := a.T.Bookmarks.OneBy("uobj", store.Key(viewer.ID, "people", u.ID))
			out["favorite"] = fav
			// оценка анкеты
			vv, _ := voices.OneBy("pair", store.Key(viewer.ID, u.ID))
			out["my_vote"] = vv.Rating
			out["can_vote"] = viewer.Balls >= 50 && myVoteSum(viewer.ID) >= 0
		}
		out["votes"] = voteStats(u.ID)
		return c.JSON(out)
	})

	// ---------- избранные пользователи (people)
	a.POST("/api/user/{id}/favorite", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		u, err := userByPath(c)
		if err != nil {
			return err
		}
		if u.ID == me.ID {
			return core.ErrBad("Нельзя добавить себя")
		}
		var in struct {
			On bool `json:"on"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		key := store.Key(me.ID, "people", u.ID)
		ex, has := a.T.Bookmarks.OneBy("uobj", key)
		if in.On && !has {
			a.T.Bookmarks.Insert(&model.Bookmarks{IDUser: me.ID, IDObject: u.ID, Type: "people", Time: core.Now()})
		} else if !in.On && has {
			a.T.Bookmarks.Delete(ex.ID)
		}
		return c.OK()
	}, core.Auth)

	// ---------- оценка анкеты (-2..2)
	a.POST("/api/user/{id}/rating", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		u, err := userByPath(c)
		if err != nil {
			return err
		}
		if u.ID == me.ID {
			return core.ErrBad("Нельзя оценивать себя")
		}
		if me.Balls < 50 {
			return core.ErrForbidden("Для оценки нужно минимум 50 баллов")
		}
		if myVoteSum(me.ID) < 0 {
			return core.ErrForbidden("Ваш рейтинг слишком низкий для оценок")
		}
		var in struct {
			Rating int64 `json:"rating"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		r := min(max(in.Rating, -2), 2)
		voices.DeleteBy("pair", store.Key(me.ID, u.ID))
		if r != 0 {
			voices.Insert(&Voice{IDUser: me.ID, IDKont: u.ID, Rating: r, Time: core.Now()})
		}
		sum := myVoteSum(u.ID)
		a.T.Users.Update(u.ID, func(x *model.User) { x.Rating = sum })
		a.ForgetUser(u.ID)
		if r != 0 {
			a.Notify(me.ID, u.ID, "user_rating", me.ID, false)
		}
		return c.OK("rating", sum)
	}, core.Auth)

	// кто оценил (who_rating.php)
	a.GET("/api/user/{id}/voters", func(c *core.Ctx) error {
		u, err := userByPath(c)
		if err != nil {
			return err
		}
		p := c.Page()
		list, total := voices.Select(store.Opts[Voice]{Index: "kont", Key: store.I(u.ID), Desc: true, Offset: p.Offset, Limit: p.Per})
		items := make([]map[string]any, 0, len(list))
		for _, v := range list {
			items = append(items, map[string]any{"user": a.Ref(v.IDUser), "rating": v.Rating, "time": v.Time})
		}
		return c.List(items, total, p)
	})

	// ---------- редактирование анкеты
	a.GET("/api/user/edit", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		return c.JSON(editView(me))
	}, core.Auth)

	a.PUT("/api/user/edit", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		var in struct {
			Name, City, About, Job, Skype, Telegram, Vk, Phone string
			Day, Month, Year, Pol                              *int64
			ShowMail                                           *bool `json:"show_mail"`
			Marital, Baby, Smok, Alko                          *int64
			Meet                                               []int64
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		var errs []string
		if !reName.MatchString(in.Name) || strLen(in.Name) > 32 {
			errs = append(errs, "Некорректное имя")
		}
		if !reCity.MatchString(in.City) || strLen(in.City) > 32 {
			errs = append(errs, "Некорректное название города")
		}
		if strLen(in.About) > 512 {
			errs = append(errs, "Поле «О себе» не длиннее 512 символов")
		}
		if strLen(in.Job) > 215 {
			errs = append(errs, "Поле «Занятие» не длиннее 215 символов")
		}
		if !reSkype.MatchString(in.Skype) || strLen(in.Skype) > 32 {
			errs = append(errs, "Некорректный Skype")
		}
		if !reSkype.MatchString(in.Telegram) || strLen(in.Telegram) > 32 {
			errs = append(errs, "Некорректный Telegram")
		}
		if !reLink.MatchString(in.Vk) || strLen(in.Vk) > 64 {
			errs = append(errs, "Некорректная ссылка VK")
		}
		if !rePhone.MatchString(in.Phone) || strLen(in.Phone) > 16 {
			errs = append(errs, "Некорректный номер телефона")
		}
		okRange := func(p *int64, lo, hi int64) bool { return p == nil || *p == 0 || (*p >= lo && *p <= hi) }
		if !okRange(in.Day, 1, 31) || !okRange(in.Month, 1, 12) || !okRange(in.Year, 1900, int64(timeYear())) {
			errs = append(errs, "Некорректная дата рождения")
		}
		if len(errs) > 0 {
			return core.ErrBad(strings.Join(errs, ". "))
		}
		a.T.Users.Update(me.ID, func(u *model.User) {
			u.AnkName, u.AnkCity = in.Name, in.City
			u.AnkOSebe, u.AnkZan = a.Text.Censor(in.About), a.Text.Censor(in.Job)
			u.AnkSkype, u.AnkTelegram, u.AnkVk, u.AnkNTel = in.Skype, in.Telegram, in.Vk, in.Phone
			if in.Day != nil {
				u.AnkDR = *in.Day
			}
			if in.Month != nil {
				u.AnkMR = *in.Month
			}
			if in.Year != nil {
				u.AnkGR = *in.Year
			}
			if in.Pol != nil && (*in.Pol == 0 || *in.Pol == 1) {
				u.Pol = *in.Pol
			}
			if in.ShowMail != nil {
				u.ShowMail = *in.ShowMail
			}
			if in.Marital != nil && *in.Marital >= 0 && int(*in.Marital) < len(maritalLabels) {
				u.AnkMaritalStatus = *in.Marital
			}
			if in.Baby != nil && *in.Baby >= 0 && int(*in.Baby) < len(babyLabels) {
				u.AnkBaby = *in.Baby
			}
			if in.Smok != nil && *in.Smok >= 0 && int(*in.Smok) < len(smokLabels) {
				u.AnkSmok = *in.Smok
			}
			if in.Alko != nil && *in.Alko >= 0 && int(*in.Alko) < len(alkoLabels) {
				u.AnkAlkoN = *in.Alko
			}
			if in.Meet != nil {
				flags := [14]int64{}
				for _, i := range in.Meet {
					if i >= 1 && i <= 14 {
						flags[i-1] = 1
					}
				}
				u.AnkLov1, u.AnkLov2, u.AnkLov3, u.AnkLov4, u.AnkLov5, u.AnkLov6, u.AnkLov7 = flags[0], flags[1], flags[2], flags[3], flags[4], flags[5], flags[6]
				u.AnkLov8, u.AnkLov9, u.AnkLov10, u.AnkLov11, u.AnkLov12, u.AnkLov13, u.AnkLov14 = flags[7], flags[8], flags[9], flags[10], flags[11], flags[12], flags[13]
			}
		})
		a.ForgetUser(me.ID)
		nu, _ := a.GetUser(me.ID)
		return c.JSON(editView(&nu))
	}, core.Auth)

	a.PUT("/api/user/wall", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		var in struct {
			Wall int64 `json:"wall"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		a.T.Users.Update(me.ID, func(u *model.User) { u.Wall = min(max(in.Wall, 0), 1) })
		return c.OK()
	}, core.Auth)
}

func editView(u *model.User) map[string]any {
	meet := []int64{}
	flags := []int64{u.AnkLov1, u.AnkLov2, u.AnkLov3, u.AnkLov4, u.AnkLov5, u.AnkLov6, u.AnkLov7, u.AnkLov8, u.AnkLov9, u.AnkLov10, u.AnkLov11, u.AnkLov12, u.AnkLov13, u.AnkLov14}
	for i, f := range flags {
		if f == 1 {
			meet = append(meet, int64(i+1))
		}
	}
	return map[string]any{
		"name": u.AnkName, "city": u.AnkCity, "about": u.AnkOSebe, "job": u.AnkZan, "skype": u.AnkSkype, "telegram": u.AnkTelegram,
		"phone": u.AnkNTel, "day": u.AnkDR, "month": u.AnkMR, "year": u.AnkGR, "pol": u.Pol, "show_mail": u.ShowMail,
		"marital": u.AnkMaritalStatus, "baby": u.AnkBaby, "smok": u.AnkSmok, "alko": u.AnkAlkoN, "meet": meet, "email": u.Email,
		"options": map[string]any{"marital": maritalLabels, "baby": babyLabels, "smok": smokLabels, "alko": alkoLabels, "meet": lovLabels},
	}
}

func statusOf(uid int64) *Status {
	for _, s := range statuses.By("user", store.I(uid)) {
		if s.Pokaz {
			x := s
			return &x
		}
	}
	return nil
}

func myVoteSum(uid int64) int64 {
	var s int64
	for _, v := range voices.By("kont", store.I(uid)) {
		s += v.Rating
	}
	return s
}

func voteStats(uid int64) map[string]int {
	m := map[string]int{"plus": 0, "minus": 0, "neutral": 0}
	for _, v := range voices.By("kont", store.I(uid)) {
		switch {
		case v.Rating > 0:
			m["plus"]++
		case v.Rating < 0:
			m["minus"]++
		default:
			m["neutral"]++
		}
	}
	return m
}

type achievement struct {
	Key   string `json:"key"`
	Emoji string `json:"emoji"`
	Title string `json:"title"`
	Desc  string `json:"desc"`
}

func computeAchievements(u *model.User, items []core.ProfileItem, friendsCnt, stenaCnt int) []achievement {
	var out []achievement
	add := func(key, emoji, title, desc string) {
		out = append(out, achievement{Key: key, Emoji: emoji, Title: title, Desc: desc})
	}
	if u.Verified {
		add("verified", "✅", "Верифицирован", "Подтверждённый аккаунт")
	}
	if u.Level >= 2 {
		add("staff", "🛡️", "Страж порядка", "Представитель администрации или модерации сайта")
	}
	if u.TOTPEnabled {
		add("2fa", "🔐", "Под защитой", "Включена двухфакторная аутентификация (2FA)")
	}
	if u.ID <= 100 || (u.DateReg > 0 && core.Now()-u.DateReg >= 30*86400) {
		add("veteran", "🚀", "Первопроходец", "В числе первых участников или на сайте больше месяца")
	}
	if u.AvatarID > 0 {
		add("avatar", "📸", "Фотогеничный", "Установлен собственный аватар")
	}
	if friendsCnt >= 20 {
		add("popular", "🌟", "Популярный", "Более 20 друзей на сайте")
	} else if friendsCnt >= 5 {
		add("friendly", "🤝", "Душа компании", "5 и более друзей на сайте")
	}
	if u.Rating >= 20 {
		add("rating", "🏆", "Звезда рейтинга", "Высокий рейтинг профиля")
	}
	if stenaCnt >= 10 || u.Balls >= 100 {
		add("active", "💬", "Общительный", "Активное участие в жизни сайта")
	}
	for _, it := range items {
		if it.Key == "shorts" && it.Count > 0 {
			add("director", "🎬", "Кинорежиссёр", "Опубликован хотя бы один клип")
		}
		if it.Key == "groups" && it.Count > 0 {
			add("community", "👥", "Активист сообществ", "Участник сообществ сайта")
		}
	}
	return out
}
