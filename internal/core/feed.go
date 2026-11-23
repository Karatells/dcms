package core

import (
	"dcms/internal/model"
)

// Ленты, обсуждения и уведомления — общие примитивы для всех модулей (в старом движке были разбросаны SQL-вставками).

// FeedKind — тип контента для фильтров друзей/настроек ленты и обсуждений.
type FeedKind string

const (
	KForum      FeedKind = "forum"
	KObmen      FeedKind = "obmen" // в tape_set / discussions_set это "files"
	KFoto       FeedKind = "foto"
	KNotes      FeedKind = "notes"
	KAvatar     FeedKind = "avatar"
	KFrends     FeedKind = "frends"
	KStatus     FeedKind = "status"
	KStatusLike FeedKind = "status_like"
)

func tapeFlags(f *model.Frends, k FeedKind) bool {
	switch k {
	case KForum:
		return f.LentaForum == 1
	case KObmen:
		return f.LentaObmen == 1
	case KFoto:
		return f.LentaFoto == 1
	case KNotes:
		return f.LentaNotes == 1
	case KAvatar:
		return f.LentaAvatar == 1
	case KFrends:
		return f.LentaFrends == 1
	case KStatus:
		return f.LentaStatus == 1
	case KStatusLike:
		return f.LentaStatusLike == 1
	}
	return true
}

func discFlags(f *model.Frends, k FeedKind) bool {
	switch k {
	case KForum:
		return f.DiscForum == 1
	case KObmen:
		return f.DiscObmen == 1
	case KFoto:
		return f.DiscFoto == 1
	case KNotes:
		return f.DiscNotes == 1
	case KFrends:
		return f.DiscFrends == 1
	case KStatus:
		return f.DiscStatus == 1
	}
	return true
}

// TapeSettings — общие настройки ленты пользователя (создаёт по умолчанию, все включены).
func (a *App) TapeSettings(uid int64) model.TapeSet {
	s, ok := a.T.TapeSet.OneBy("user", i64(uid))
	if !ok {
		s = model.TapeSet{IDUser: uid, LentaStatusLike: 1, LentaStatus: 1, LentaFoto: 1, LentaFiles: 1, LentaForum: 1, LentaNotes: 1, LentaAvatar: 1, LentaFrends: 1}
		a.T.TapeSet.Insert(&s)
	}
	return s
}

// DiscSettings — общие настройки обсуждений.
func (a *App) DiscSettings(uid int64) model.DiscussionsSet {
	s, ok := a.T.DiscSet.OneBy("user", i64(uid))
	if !ok {
		s = model.DiscussionsSet{IDUser: uid, DiscStatus: 1, DiscFoto: 1, DiscFiles: 1, DiscForum: 1, DiscNotes: 1}
		a.T.DiscSet.Insert(&s)
	}
	return s
}

// NotifSettings — настройки уведомлений (komm: уведомлять об ответах).
func (a *App) NotifSettings(uid int64) model.NotificationSet {
	s, ok := a.T.NotifSet.OneBy("user", i64(uid))
	if !ok {
		s = model.NotificationSet{IDUser: uid, Komm: 1}
		a.T.NotifSet.Insert(&s)
	}
	return s
}

func tapeSetOn(s model.TapeSet, k FeedKind) bool {
	switch k {
	case KForum:
		return s.LentaForum == 1
	case KObmen:
		return s.LentaFiles == 1
	case KFoto:
		return s.LentaFoto == 1
	case KNotes:
		return s.LentaNotes == 1
	case KAvatar:
		return s.LentaAvatar == 1
	case KFrends:
		return s.LentaFrends == 1
	case KStatus:
		return s.LentaStatus == 1
	case KStatusLike:
		return s.LentaStatusLike == 1
	}
	return true
}

func discSetOn(s model.DiscussionsSet, k FeedKind) bool {
	switch k {
	case KForum:
		return s.DiscForum == 1
	case KObmen:
		return s.DiscFiles == 1
	case KFoto:
		return s.DiscFoto == 1
	case KNotes:
		return s.DiscNotes == 1
	case KStatus:
		return s.DiscStatus == 1
	}
	return true
}

// TapeOpts — доп. поля записи ленты.
type TapeOpts struct {
	OtKogo int64 // кто совершил действие (для status_like/status: ot_kogo)
	Avatar int64 // id фото-аватара (type=avatar)
}

// FanTape рассылает запись ленты друзьям автора (author), у которых включён фильтр kind.
// typ — значение поля type (например "them","obmen","avatar","album","frends","status","status_like","notes"),
// fileID — id объекта (поле id_file). Рассылка идёт тем, кто у автора в друзьях (frends.user=author, i=1)
// и у кого в личных настройках ленты включён kind — как в старом движке.
func (a *App) FanTape(author int64, kind FeedKind, typ string, fileID int64, o TapeOpts) {
	now := Now()
	for _, f := range a.T.Friends.By("user", i64(author)) {
		if f.I != 1 || !tapeFlags(&f, kind) {
			continue
		}
		if !tapeSetOn(a.TapeSettings(f.Frend), kind) {
			continue
		}
		a.T.Tape.Insert(&model.Tape{IDUser: f.Frend, Avtor: author, Type: typ, Time: now, IDFile: fileID, Count: 1, OtKogo: o.OtKogo, Avatar: o.Avatar})
	}
	a.pushCounters(author)
}

// TapeTo — запись ленты конкретному пользователю (без фильтров).
func (a *App) TapeTo(to, author int64, typ string, fileID int64, o TapeOpts) {
	a.T.Tape.Insert(&model.Tape{IDUser: to, Avtor: author, Type: typ, Time: Now(), IDFile: fileID, Count: 1, OtKogo: o.OtKogo, Avatar: o.Avatar})
	a.Hub.ToUser(to, "counters", map[string]any{"tape": true})
}

// Discuss регистрирует активность в обсуждениях: друзьям владельца объекта (owner) с включённым kind
// и самому владельцу (если он не actor). Если запись уже есть — увеличивает счётчик и поднимает время.
// typ — тип обсуждения ("them","obmen","foto","notes","status"), objID — id объекта.
func (a *App) Discuss(owner, actor int64, kind FeedKind, typ string, objID int64) {
	now := Now()
	upsert := func(to int64) {
		key := i64(to) + "|" + typ + "|" + i64(objID)
		if d, ok := a.T.Discussions.OneBy("obj", key); ok {
			a.T.Discussions.Update(d.ID, func(x *model.Discussions) { x.Count++; x.Time = now; x.Avtor = owner })
		} else {
			a.T.Discussions.Insert(&model.Discussions{IDUser: to, Avtor: owner, Type: typ, Time: now, IDSim: objID, Count: 1})
		}
		a.Hub.ToUser(to, "counters", map[string]any{"discussions": true})
	}
	for _, f := range a.T.Friends.By("user", i64(owner)) {
		if f.I != 1 || !discFlags(&f, kind) || !discSetOn(a.DiscSettings(f.Frend), kind) {
			continue
		}
		if f.Frend != actor {
			upsert(f.Frend)
		}
	}
	if owner != actor {
		upsert(owner)
	}
}

// DiscussAdd создаёт запись обсуждения для самого актёра, чтобы он видел ветку, где участвует (комментатор).
func (a *App) DiscussFor(to int64, owner int64, typ string, objID int64) {
	now := Now()
	key := i64(to) + "|" + typ + "|" + i64(objID)
	if _, ok := a.T.Discussions.OneBy("obj", key); !ok {
		a.T.Discussions.Insert(&model.Discussions{IDUser: to, Avtor: owner, Type: typ, Time: now, IDSim: objID, Count: 0})
	}
}

// Notify создаёт уведомление пользователю to от avtor. typ — тип (например "stena_komm","them_komm","new_gift");
// objectID — id объекта. Для типов-ответов (komm=true) уважает настройку notification_set.komm получателя.
// Не уведомляет самого себя. Возвращает true, если уведомление создано.
func (a *App) Notify(avtor, to int64, typ string, objectID int64, komm bool) bool {
	if to == 0 || to == avtor {
		return false
	}
	if komm && a.NotifSettings(to).Komm != 1 {
		return false
	}
	a.T.Notifications.Insert(&model.Notification{IDUser: to, Time: Now(), IDObject: objectID, Type: typ, Avtor: avtor})
	a.Hub.ToUser(to, "notify", map[string]any{"type": typ, "from": a.Ref(avtor)})
	return true
}

func (a *App) pushCounters(uid int64) {}

// ReportSpam — жалоба (таблица spamus); razdel — модуль; idPost — id сообщения, idObject — id объекта/темы.
func (a *App) ReportSpam(reporter, spammer int64, razdel string, idPost, idObject int64, text, msg string) {
	a.T.Spam.Insert(&model.Spamus{IDUser: spammer, IDSpam: reporter, Razdel: razdel, IDPost: idPost, IDObject: idObject, Spam: text, Msg: msg, Time: Now()})
}

// AdminLog пишет действие администрации (admin_log); mod и act — названия, создаются при отсутствии.
func (a *App) AdminLog(uid int64, mod, act, desc string) {
	m, ok := a.T.AdminLogMod.FindOne(func(x *model.AdminLogMod) bool { return x.Name == mod })
	if !ok {
		m.Name = mod
		a.T.AdminLogMod.Insert(&m)
	}
	ac, ok := a.T.AdminLogAct.FindOne(func(x *model.AdminLogAct) bool { return x.IDMod == m.ID && x.Name == act })
	if !ok {
		ac = model.AdminLogAct{IDMod: m.ID, Name: act}
		a.T.AdminLogAct.Insert(&ac)
	}
	a.T.AdminLog.Insert(&model.AdminLog{IDUser: uid, Time: Now(), Mod: m.ID, Act: ac.ID, Opis: desc})
}
