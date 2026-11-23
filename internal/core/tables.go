package core

import (
	"strings"

	"dcms/internal/model"
	"dcms/internal/store"
)

// T — центральные таблицы.
type T struct {
	Users         *store.Table[model.User, *model.User]
	Sessions      *store.Table[model.Session, *model.Session]
	KV            *store.Table[model.KV, *model.KV]
	Groups        *store.Table[model.UserGroup, *model.UserGroup]
	GroupAccess   *store.Table[model.UserGroupAccess, *model.UserGroupAccess]
	AllAccesses   *store.Table[model.AllAccesses, *model.AllAccesses]
	Bans          *store.Table[model.Ban, *model.Ban]
	Friends       *store.Table[model.Frends, *model.Frends]
	FriendReqs    *store.Table[model.FrendsNew, *model.FrendsNew]
	Notifications *store.Table[model.Notification, *model.Notification]
	NotifSet      *store.Table[model.NotificationSet, *model.NotificationSet]
	Tape          *store.Table[model.Tape, *model.Tape]
	TapeSet       *store.Table[model.TapeSet, *model.TapeSet]
	Discussions   *store.Table[model.Discussions, *model.Discussions]
	DiscSet       *store.Table[model.DiscussionsSet, *model.DiscussionsSet]
	Guests        *store.Table[model.Guests, *model.Guests]
	Collisions    *store.Table[model.UserCollision, *model.UserCollision]
	UserLog       *store.Table[model.UserLog, *model.UserLog]
	UserSet       *store.Table[model.UserSet, *model.UserSet]
	Spam          *store.Table[model.Spamus, *model.Spamus]
	Likes         *store.Table[model.LikeObject, *model.LikeObject]
	Bookmarks     *store.Table[model.Bookmarks, *model.Bookmarks]
	AdminLog      *store.Table[model.AdminLog, *model.AdminLog]
	AdminLogMod   *store.Table[model.AdminLogMod, *model.AdminLogMod]
	AdminLogAct   *store.Table[model.AdminLogAct, *model.AdminLogAct]
	VisitDay      *store.Table[model.VisitEveryday, *model.VisitEveryday]
	VisitToday    *store.Table[model.VisitToday, *model.VisitToday]
	BanIP         *store.Table[model.BanIp, *model.BanIp]
	Opsos         *store.Table[model.Opsos, *model.Opsos]
	Menu          *store.Table[model.Menu, *model.Menu]
	SettingsMenu  *store.Table[model.UserSettings, *model.UserSettings]
}

func i64(n int64) string { return store.I(n) }

func newTables(db *store.DB) *T {
	t := &T{}
	t.Users = store.NewTable[model.User](db, "user").
		Index("nick", func(u *model.User) string { return u.NickLower }).
		Index("email", func(u *model.User) string { return strings.ToLower(u.Email) }).
		Index("group", func(u *model.User) string { return i64(u.GroupAccess) })
	t.Sessions = store.NewTable[model.Session](db, "session").
		Index("token", func(s *model.Session) string { return s.Token }).
		Index("user", func(s *model.Session) string { return i64(s.UserID) })
	t.KV = store.NewTable[model.KV](db, "kv").Index("key", func(k *model.KV) string { return k.Key })
	t.Groups = store.NewTable[model.UserGroup](db, "user_group")
	t.GroupAccess = store.NewTable[model.UserGroupAccess](db, "user_group_access").
		Index("group", func(g *model.UserGroupAccess) string { return i64(g.IDGroup) })
	t.AllAccesses = store.NewTable[model.AllAccesses](db, "all_accesses").
		Index("type", func(g *model.AllAccesses) string { return g.Type })
	t.Bans = store.NewTable[model.Ban](db, "ban").Index("user", func(b *model.Ban) string { return i64(b.IDUser) })
	t.Friends = store.NewTable[model.Frends](db, "frends").
		Index("user", func(f *model.Frends) string { return i64(f.User) }).
		Index("pair", func(f *model.Frends) string { return store.Key(f.User, f.Frend) })
	t.FriendReqs = store.NewTable[model.FrendsNew](db, "frends_new").
		Index("to", func(f *model.FrendsNew) string { return i64(f.To) }).
		Index("user", func(f *model.FrendsNew) string { return i64(f.User) })
	t.Notifications = store.NewTable[model.Notification](db, "notification").
		Index("user", func(n *model.Notification) string { return i64(n.IDUser) })
	t.NotifSet = store.NewTable[model.NotificationSet](db, "notification_set").
		Index("user", func(n *model.NotificationSet) string { return i64(n.IDUser) })
	t.Tape = store.NewTable[model.Tape](db, "tape").Index("user", func(n *model.Tape) string { return i64(n.IDUser) })
	t.TapeSet = store.NewTable[model.TapeSet](db, "tape_set").Index("user", func(n *model.TapeSet) string { return i64(n.IDUser) })
	t.Discussions = store.NewTable[model.Discussions](db, "discussions").
		Index("user", func(n *model.Discussions) string { return i64(n.IDUser) }).
		Index("obj", func(n *model.Discussions) string { return store.Key(n.IDUser, n.Type, n.IDSim) })
	t.DiscSet = store.NewTable[model.DiscussionsSet](db, "discussions_set").Index("user", func(n *model.DiscussionsSet) string { return i64(n.IDUser) })
	t.Guests = store.NewTable[model.Guests](db, "guests").Index("key", func(g *model.Guests) string { return store.Key(g.IP, g.Ua) })
	t.Collisions = store.NewTable[model.UserCollision](db, "user_collision")
	t.UserLog = store.NewTable[model.UserLog](db, "user_log").Index("user", func(n *model.UserLog) string { return i64(n.IDUser) })
	t.UserSet = store.NewTable[model.UserSet](db, "user_set").Index("user", func(n *model.UserSet) string { return i64(n.IDUser) })
	t.Spam = store.NewTable[model.Spamus](db, "spamus")
	t.Likes = store.NewTable[model.LikeObject](db, "like_object").
		Index("obj", func(n *model.LikeObject) string { return store.Key(n.Type, n.IDObject) }).
		Index("uobj", func(n *model.LikeObject) string { return store.Key(n.IDUser, n.Type, n.IDObject) })
	t.Bookmarks = store.NewTable[model.Bookmarks](db, "bookmarks").
		Index("user", func(n *model.Bookmarks) string { return store.Key(n.IDUser, n.Type) }).
		Index("uobj", func(n *model.Bookmarks) string { return store.Key(n.IDUser, n.Type, n.IDObject) })
	t.AdminLog = store.NewTable[model.AdminLog](db, "admin_log")
	t.AdminLogMod = store.NewTable[model.AdminLogMod](db, "admin_log_mod")
	t.AdminLogAct = store.NewTable[model.AdminLogAct](db, "admin_log_act")
	t.VisitDay = store.NewTable[model.VisitEveryday](db, "visit_everyday")
	t.VisitToday = store.NewTable[model.VisitToday](db, "visit_today")
	t.BanIP = store.NewTable[model.BanIp](db, "ban_ip")
	t.Opsos = store.NewTable[model.Opsos](db, "opsos")
	t.Menu = store.NewTable[model.Menu](db, "menu")
	t.SettingsMenu = store.NewTable[model.UserSettings](db, "user_settings")
	return t
}
