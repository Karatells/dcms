package core

import (
	"fmt"
	"strings"
	"time"

	"dcms/internal/model"
)

type cachedRef struct {
	ref model.UserRef
	at  int64
}

// Medal — уровень медали по рейтингу (как medal.php).
func Medal(r int64) int {
	switch {
	case r >= 60:
		return 7
	case r >= 48:
		return 6
	case r >= 38:
		return 5
	case r >= 28:
		return 4
	case r >= 20:
		return 3
	case r >= 12:
		return 2
	case r >= 6:
		return 1
	}
	return 0
}

func (a *App) groupLevel(gid int64) int64 {
	if g, ok := a.T.Groups.Get(gid); ok {
		return g.Level
	}
	return 0
}

// AvatarURL — url аватара пользователя ("" если нет).
func AvatarURL(u *model.User, size int) string {
	if u.AvatarID > 0 {
		return fmt.Sprintf("/api/foto/img/%d.%s?s=%d", u.AvatarID, u.AvatarExt, size)
	}
	return ""
}

func (a *App) dropRef(id int64) {
	a.mu.Lock()
	delete(a.userCache, id)
	a.mu.Unlock()
}

// DropUserCache сбрасывает кеш карточки пользователя (UserRef).
func (a *App) DropUserCache(id int64) { a.dropRef(id) }

// Ref возвращает краткую карточку пользователя (кешируется на 30 с). id=0 — «Система», несуществующий — «[Удален]».
func (a *App) Ref(id int64) model.UserRef {
	if id == 0 {
		return model.UserRef{ID: 0, Nick: "Система", Pol: 1}
	}
	now := time.Now().Unix()
	a.mu.RLock()
	c := a.userCache[id]
	a.mu.RUnlock()
	if c != nil && now-c.at < 30 {
		r := c.ref
		r.Online = a.isOnline(id)
		return r
	}
	u, ok := a.T.Users.Get(id)
	if !ok {
		return model.UserRef{ID: 0, Nick: "[Удален]", Pol: 1, Deleted: true}
	}
	r := a.refOf(&u)
	a.mu.Lock()
	a.userCache[id] = &cachedRef{ref: r, at: now}
	a.mu.Unlock()
	return r
}

func (a *App) isOnline(id int64) bool {
	u, ok := a.T.Users.Get(id)
	if !ok || u.DateLast <= Now()-600 {
		return false
	}
	if s, ok := a.T.UserSet.OneBy("user", i64(id)); ok && s.HideOnline {
		return false
	}
	return true
}

// CanAccessPrivacy проверяет расширенную настройку приватности (0=все, 1=только друзья, 2=только я).
func (a *App) CanAccessPrivacy(viewer *model.User, targetID, mode int64) bool {
	if viewer != nil && (viewer.ID == targetID || viewer.Level >= 7) {
		return true
	}
	switch mode {
	case 2:
		return false
	case 1:
		return viewer != nil && a.IsFriend(viewer.ID, targetID)
	default:
		return true
	}
}

func (a *App) refOf(u *model.User) model.UserRef {
	g, _ := a.T.Groups.Get(u.GroupAccess)
	isVIP := u.VIPUntil > Now()
	vipBadge, vipNick := "", ""
	if isVIP {
		vipBadge = u.VIPBadge
		if vipBadge == "" {
			vipBadge = "👑"
		}
		vipNick = u.VIPNickStyle
	}
	return model.UserRef{ID: u.ID, Nick: u.Nick, Pol: u.Pol, Online: a.isOnline(u.ID), Mobile: u.Browser == "wap",
		Avatar: AvatarURL(u, 50), Group: g.Name, Level: g.Level, Medal: Medal(u.Rating), Banned: a.IsBanned(u.ID), Verified: u.Verified, Accent: u.Accent,
		VIP: isVIP, VIPBadge: vipBadge, VIPNickStyle: vipNick}
}

// Refs — карточки списка id (порядок сохраняется).
func (a *App) Refs(ids ...int64) []model.UserRef {
	out := make([]model.UserRef, len(ids))
	for i, id := range ids {
		out[i] = a.Ref(id)
	}
	return out
}

// Can — проверка привилегии группы (user_access).
func (a *App) Can(uid int64, access string) bool {
	u, ok := a.T.Users.Get(uid)
	if !ok || u.GroupAccess == 0 {
		return false
	}
	a.mu.RLock()
	m, ok := a.accessCache[u.GroupAccess]
	a.mu.RUnlock()
	if !ok {
		m = map[string]bool{}
		for _, ga := range a.T.GroupAccess.By("group", i64(u.GroupAccess)) {
			m[ga.IDAccess] = true
		}
		a.mu.Lock()
		a.accessCache[u.GroupAccess] = m
		a.mu.Unlock()
	}
	return m[access]
}

// ResetAccessCache вызывается после изменения прав групп.
func (a *App) ResetAccessCache() {
	a.mu.Lock()
	a.accessCache = map[int64]map[string]bool{}
	a.userCache = map[int64]*cachedRef{}
	a.mu.Unlock()
}

// ForgetUser сбрасывает кеш карточки (после смены ника/аватара/группы).
func (a *App) ForgetUser(id int64) { a.dropRef(id) }

// AddBalls меняет баллы (и опц. rating_tmp) пользователя.
func (a *App) AddBalls(uid, balls, ratingTmp int64) {
	a.T.Users.Update(uid, func(u *model.User) { u.Balls += balls; u.RatingTmp += ratingTmp })
}

// AddRatingTmp меняет rating_tmp.
func (a *App) AddRatingTmp(uid, d int64) {
	a.T.Users.Update(uid, func(u *model.User) { u.RatingTmp += d })
}

// AddMoney атомарно меняет баланс; при delta<0 и недостатке средств возвращает false.
func (a *App) AddMoney(uid, delta int64) bool {
	ok := false
	a.T.Users.Update(uid, func(u *model.User) {
		if u.Money+delta >= 0 {
			u.Money += delta
			ok = true
		}
	})
	return ok
}

// GetUser — пользователь по id.
func (a *App) GetUser(id int64) (model.User, bool) { return a.T.Users.Get(id) }

// UserByNick — поиск по нику без учёта регистра.
func (a *App) UserByNick(nick string) (model.User, bool) {
	return a.T.Users.OneBy("nick", strings.ToLower(strings.TrimSpace(nick)))
}

// IsFriend — взаимные ли друзья (запись frends с i=true у user→frend).
func (a *App) IsFriend(user, friend int64) bool {
	f, ok := a.T.Friends.OneBy("pair", fmt.Sprintf("%d|%d", user, friend))
	return ok && f.I == 1
}

// DeleteUser удаляет пользователя и вызывает очистку во всех модулях.
func (a *App) DeleteUser(uid int64) {
	for _, fn := range a.onDelete {
		fn(uid)
	}
	a.T.Users.Delete(uid)
	a.LogoutAll(uid, 0)
	a.T.Bans.DeleteBy("user", i64(uid))
	a.T.Friends.DeleteWhere(func(f *model.Frends) bool { return f.User == uid || f.Frend == uid })
	a.T.FriendReqs.DeleteWhere(func(f *model.FrendsNew) bool { return f.User == uid || f.To == uid })
	a.T.Notifications.DeleteWhere(func(n *model.Notification) bool { return n.IDUser == uid || n.Avtor == uid })
	a.T.Tape.DeleteWhere(func(n *model.Tape) bool { return n.IDUser == uid || n.Avtor == uid })
	a.T.Discussions.DeleteWhere(func(n *model.Discussions) bool { return n.IDUser == uid || n.Avtor == uid })
	for _, t := range []struct{ n int }{{0}} {
		_ = t
	}
	a.T.NotifSet.DeleteBy("user", i64(uid))
	a.T.TapeSet.DeleteBy("user", i64(uid))
	a.T.DiscSet.DeleteBy("user", i64(uid))
	a.T.UserSet.DeleteBy("user", i64(uid))
	a.T.UserLog.DeleteBy("user", i64(uid))
	a.T.Likes.DeleteWhere(func(l *model.LikeObject) bool { return l.IDUser == uid })
	a.T.Bookmarks.DeleteWhere(func(l *model.Bookmarks) bool { return l.IDUser == uid })
	a.dropRef(uid)
}

// UserSettings — настройки приватности пользователя (создаёт по умолчанию).
func (a *App) UserSettings(uid int64) model.UserSet {
	s, ok := a.T.UserSet.OneBy("user", i64(uid))
	if !ok {
		s = model.UserSet{IDUser: uid, PrivatStr: 1, PrivatMail: 1, Ocenka: 1}
		a.T.UserSet.Insert(&s)
	}
	return s
}

// ResetUserCache сбрасывает кеш карточки пользователя (после правки из админки).
func (a *App) ResetUserCache(id int64) { a.dropRef(id) }
