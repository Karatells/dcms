// Package profile — анкета, стена, статусы, друзья, гости, список пользователей, оценки анкеты.
package profile

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

// ---- таблицы модуля

type Stena struct {
	store.Base
	IDUser  int64  `json:"id_user"`  // автор
	IDStena int64  `json:"id_stena"` // чья стена
	Time    int64  `json:"time"`
	Msg     string `json:"msg"`
}
type StenaLike struct {
	store.Base
	IDUser  int64 `json:"id_user"`
	IDStena int64 `json:"id_stena"`
}
type Status struct {
	store.Base
	IDUser int64  `json:"id_user"`
	Time   int64  `json:"time"`
	Msg    string `json:"msg"`
	Pokaz  bool   `json:"pokaz"`
}
type StatusLike struct {
	store.Base
	IDUser   int64 `json:"id_user"`
	IDStatus int64 `json:"id_status"`
	Time     int64 `json:"time"`
	Like     int64 `json:"like"` // 1 нравится, 0 не нравится
}
type StatusKomm struct {
	store.Base
	IDUser   int64  `json:"id_user"`
	IDStatus int64  `json:"id_status"`
	Time     int64  `json:"time"`
	Msg      string `json:"msg"`
}
type MyGuest struct {
	store.Base
	IDAnk  int64 `json:"id_ank"`
	IDUser int64 `json:"id_user"`
	Time   int64 `json:"time"`
	Read   bool  `json:"read"`
}
type Voice struct {
	store.Base
	IDUser int64 `json:"id_user"`
	IDKont int64 `json:"id_kont"`
	Rating int64 `json:"rating"`
	Time   int64 `json:"time"`
}

var (
	stena     *store.Table[Stena, *Stena]
	stenaLike *store.Table[StenaLike, *StenaLike]
	statuses  *store.Table[Status, *Status]
	stLikes   *store.Table[StatusLike, *StatusLike]
	stKomm    *store.Table[StatusKomm, *StatusKomm]
	myGuests  *store.Table[MyGuest, *MyGuest]
	voices    *store.Table[Voice, *Voice]
)

var (
	maritalLabels = []string{"Не указано", "Без пары", "Встречается", "Помолвлен/Помолвлена", "В браке", "В гражданском браке",
		"В домашнем партнерстве", "В свободных отношениях", "Все сложно", "Раздельное проживание", "В разводе", "Вдовец/Вдова", "Влюблен/Влюблена"}
	babyLabels = []string{"Не указано", "Нет", "Нет, но хотелось бы", "Есть, живем вместе", "Есть, живем порознь"}
	smokLabels = []string{"Не указано", "Не курю", "Курю", "Редко", "Бросаю", "Успешно бросил"}
	alkoLabels = []string{"Не указано", "Да, выпиваю", "Редко, по праздникам", "Нет, категорически не приемлю"}
	lovLabels  = []string{"Дружба и общение", "Переписка", "Любовь, отношения", "Регулярный секс вдвоем", "Секс на один-два раза",
		"Групповой секс", "Виртуальный секс", "Предлагаю интим за деньги", "Ищу интим за деньги", "Брак, создание семьи",
		"Рождение, воспитание ребенка", "Брак для вида", "Совместная аренда жилья", "Занятия спортом"}
	reName  = regexp.MustCompile(`^[A-Za-zА-Яа-яЁё \-]*$`)
	reCity  = regexp.MustCompile(`^[A-Za-zА-Яа-яЁё \-]*$`)
	reSkype = regexp.MustCompile(`^[A-Za-z0-9_.\-:@]*$`)
	rePhone = regexp.MustCompile(`^[0-9+\-() ]*$`)
	reLink  = regexp.MustCompile(`^[A-Za-z0-9_./:\-]*$`)
)

func Register(a *core.App) {
	db := a.DB
	stena = store.NewTable[Stena](db, "stena").Index("wall", func(s *Stena) string { return store.I(s.IDStena) })
	stenaLike = store.NewTable[StenaLike](db, "stena_like").Index("post", func(s *StenaLike) string { return store.I(s.IDStena) })
	statuses = store.NewTable[Status](db, "status").Index("user", func(s *Status) string { return store.I(s.IDUser) })
	stLikes = store.NewTable[StatusLike](db, "status_like").Index("st", func(s *StatusLike) string { return store.I(s.IDStatus) })
	stKomm = store.NewTable[StatusKomm](db, "status_komm").Index("st", func(s *StatusKomm) string { return store.I(s.IDStatus) })
	myGuests = store.NewTable[MyGuest](db, "my_guests").
		Index("ank", func(s *MyGuest) string { return store.I(s.IDAnk) }).
		Index("pair", func(s *MyGuest) string { return store.Key(s.IDAnk, s.IDUser) })
	voices = store.NewTable[Voice](db, "user_voice2").
		Index("kont", func(s *Voice) string { return store.I(s.IDKont) }).
		Index("pair", func(s *Voice) string { return store.Key(s.IDUser, s.IDKont) })

	a.OnUserDelete(func(uid int64) {
		stena.DeleteWhere(func(s *Stena) bool { return s.IDUser == uid || s.IDStena == uid })
		stenaLike.DeleteWhere(func(s *StenaLike) bool { return s.IDUser == uid })
		for _, s := range statuses.By("user", store.I(uid)) {
			stLikes.DeleteBy("st", store.I(s.ID))
			stKomm.DeleteBy("st", store.I(s.ID))
		}
		statuses.DeleteBy("user", store.I(uid))
		stLikes.DeleteWhere(func(s *StatusLike) bool { return s.IDUser == uid })
		stKomm.DeleteWhere(func(s *StatusKomm) bool { return s.IDUser == uid })
		myGuests.DeleteWhere(func(s *MyGuest) bool { return s.IDUser == uid || s.IDAnk == uid })
		voices.DeleteWhere(func(s *Voice) bool { return s.IDUser == uid || s.IDKont == uid })
		a.T.Friends.DeleteWhere(func(f *model.Frends) bool { return f.User == uid || f.Frend == uid })
		a.T.FriendReqs.DeleteWhere(func(f *model.FrendsNew) bool { return f.User == uid || f.To == uid })
	})

	a.Describer("user", func(id int64) (core.Desc, bool) {
		u, ok := a.GetUser(id)
		if !ok {
			return core.Desc{}, false
		}
		return core.Desc{Title: u.Nick, Link: "/id" + store.I(u.ID), Image: core.AvatarURL(&u, 100), Owner: u.ID}, true
	})
	a.Describer("status", func(id int64) (core.Desc, bool) {
		s, ok := statuses.Get(id)
		if !ok {
			return core.Desc{}, false
		}
		return core.Desc{Title: "Статус", Text: a.Text.Plain(s.Msg, 200), Link: "/user/status/" + store.I(s.ID), Owner: s.IDUser}, true
	})
	a.Describer("stena", func(id int64) (core.Desc, bool) {
		s, ok := stena.Get(id)
		if !ok {
			return core.Desc{}, false
		}
		return core.Desc{Title: "Сообщение на стене", Text: a.Text.Plain(s.Msg, 200), Link: "/user/stena/" + store.I(s.IDStena), Owner: s.IDUser}, true
	})

	a.Counter("myguest", func(a *core.App, u *model.User) string {
		return strconv.Itoa(myGuests.Count(func(g *MyGuest) bool { return g.IDAnk == u.ID && g.Read }))
	})
	a.Counter("frends", func(a *core.App, u *model.User) string {
		n := a.T.FriendReqs.CountBy("to", store.I(u.ID))
		if n == 0 {
			return ""
		}
		return strconv.Itoa(n)
	})
	a.Counter("users", func(a *core.App, u *model.User) string {
		return strconv.Itoa(a.T.Users.Count(func(x *model.User) bool { return x.DateLast > core.Now()-600 }))
	})

	a.ProfileStat(func(a *core.App, viewer, target *model.User) []core.ProfileItem {
		var out []core.ProfileItem
		if n := stena.CountBy("wall", store.I(target.ID)); n > 0 {
			out = append(out, core.ProfileItem{Key: "stena", Title: "Стена", Icon: "comment-text", Link: "/user/stena/" + store.I(target.ID), Count: int64(n)})
		}
		if n := a.T.Friends.CountBy("user", store.I(target.ID)); n > 0 {
			out = append(out, core.ProfileItem{Key: "frends", Title: "Друзья", Icon: "account-group", Link: "/user/frends/" + store.I(target.ID), Count: int64(n)})
		}
		return out
	})

	registerProfile(a)
	registerSocial(a)
	registerLists(a)
}

// GetWallPosts возвращает записи со стены пользователя (используется публичным API v1).
func GetWallPosts(a *core.App, targetID int64, offset, per int) ([]map[string]any, int) {
	all := stena.By("wall", store.I(targetID))
	sort.Slice(all, func(i, j int) bool { return all[i].ID > all[j].ID })
	total := len(all)
	if offset < 0 {
		offset = 0
	}
	end := offset + per
	if end > total {
		end = total
	}
	var sl []Stena
	if offset < total {
		sl = all[offset:end]
	}
	items := make([]map[string]any, 0, len(sl))
	for _, s := range sl {
		items = append(items, map[string]any{
			"id":        s.ID,
			"wall_id":   s.IDStena,
			"author":    a.Ref(s.IDUser),
			"text":      s.Msg,
			"html":      a.Text.Render(s.Msg),
			"likes":     stenaLike.CountBy("post", store.I(s.ID)),
			"time":      s.Time,
		})
	}
	return items, total
}

// CreateWallPost добавляет запись на стену пользователя (используется публичным API v1).
func CreateWallPost(a *core.App, authorID, targetID int64, msg string) (Stena, error) {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return Stena{}, core.ErrBad("Текст записи не может быть пустым")
	}
	if utf8.RuneCountInString(msg) > 1024 {
		return Stena{}, core.ErrBad("Максимум 1024 символа")
	}
	id := stena.Insert(&Stena{IDUser: authorID, IDStena: targetID, Time: core.Now(), Msg: msg})
	p, _ := stena.Get(id)
	return p, nil
}

// ---- вспомогательные

type viewState int

const (
	vOK viewState = iota
	vFriends
	vClosed
)

// access определяет, может ли viewer видеть анкету/стену target (privat_str: 1 все, 2 друзья, 0 только я).
func access(a *core.App, viewer *model.User, target *model.User) viewState {
	if viewer != nil && (viewer.ID == target.ID || viewer.Level >= 7) {
		return vOK
	}
	switch a.UserSettings(target.ID).PrivatStr {
	case 0:
		return vClosed
	case 2:
		if viewer != nil && a.IsFriend(viewer.ID, target.ID) {
			return vOK
		}
		return vFriends
	}
	return vOK
}

func friendState(a *core.App, viewer *model.User, id int64) string {
	if viewer == nil || viewer.ID == id {
		return ""
	}
	if a.IsFriend(viewer.ID, id) {
		return "friend"
	}
	for _, r := range a.T.FriendReqs.By("user", store.I(viewer.ID)) {
		if r.To == id {
			return "sent"
		}
	}
	for _, r := range a.T.FriendReqs.By("to", store.I(viewer.ID)) {
		if r.User == id {
			return "incoming"
		}
	}
	return "none"
}

func strLen(s string) int { return utf8.RuneCountInString(s) }

func userByPath(c *core.Ctx) (model.User, error) {
	u, ok := c.App.GetUser(c.PathInt("id"))
	if !ok {
		return u, core.ErrNotFound("Пользователь не найден")
	}
	return u, nil
}

func ageOf(u *model.User) int {
	if u.AnkGR == 0 {
		return 0
	}
	t := time.Now()
	age := t.Year() - int(u.AnkGR)
	if u.AnkMR > 0 && (int(u.AnkMR) > int(t.Month()) || (int(u.AnkMR) == int(t.Month()) && int(u.AnkDR) > t.Day())) {
		age--
	}
	return age
}

func label(list []string, i int64, pol int64) string {
	if i <= 0 || int(i) >= len(list) {
		return ""
	}
	s := list[i]
	if p := strings.Split(s, "/"); len(p) == 2 {
		if pol == 1 {
			return p[0]
		}
		return p[1]
	}
	return s
}

func lovList(u *model.User) []string {
	flags := []int64{u.AnkLov1, u.AnkLov2, u.AnkLov3, u.AnkLov4, u.AnkLov5, u.AnkLov6, u.AnkLov7, u.AnkLov8, u.AnkLov9, u.AnkLov10, u.AnkLov11, u.AnkLov12, u.AnkLov13, u.AnkLov14}
	var out []string
	for i, f := range flags {
		if f == 1 {
			out = append(out, lovLabels[i])
		}
	}
	return out
}

func sortUsers(list []model.User, key string, desc bool) {
	less := func(a, b *model.User) bool { return a.ID < b.ID }
	switch key {
	case "rating":
		less = func(a, b *model.User) bool { return a.Rating < b.Rating }
	case "balls":
		less = func(a, b *model.User) bool { return a.Balls < b.Balls }
	case "nick":
		less = func(a, b *model.User) bool { return a.NickLower < b.NickLower }
	case "pol":
		less = func(a, b *model.User) bool { return a.Pol < b.Pol }
	case "last":
		less = func(a, b *model.User) bool { return a.DateLast < b.DateLast }
	}
	sort.SliceStable(list, func(i, j int) bool {
		if desc {
			return less(&list[j], &list[i])
		}
		return less(&list[i], &list[j])
	})
}

func timeYear() int { return time.Now().Year() }
