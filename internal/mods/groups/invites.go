package groups

import (
	"sort"
	"strings"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

// GroupRequest — заявка на вступление в приватное сообщество.
type GroupRequest struct {
	store.Base
	IDGroup int64 `json:"id_group"`
	IDUser  int64 `json:"id_user"`
	Time    int64 `json:"time"`
}

// GroupLink — уникальная ссылка для автоматического вступления в сообщество.
type GroupLink struct {
	store.Base
	IDGroup   int64  `json:"id_group"`
	IDUser    int64  `json:"id_user"` // кто создал
	Token     string `json:"token"`
	Note      string `json:"note"`
	MaxUses   int64  `json:"max_uses"`   // 0 — без ограничений
	Uses      int64  `json:"uses"`       // сколько уже вступило
	ExpiresAt int64  `json:"expires_at"` // 0 — бессрочная
	Time      int64  `json:"time"`
}

// GroupInvite — приглашение друга в сообщество.
type GroupInvite struct {
	store.Base
	IDGroup int64 `json:"id_group"`
	IDFrom  int64 `json:"id_from"`
	IDTo    int64 `json:"id_to"`
	Time    int64 `json:"time"`
}

var (
	requestsT *store.Table[GroupRequest, *GroupRequest]
	linksT    *store.Table[GroupLink, *GroupLink]
	invitesT  *store.Table[GroupInvite, *GroupInvite]
)

func canViewGroup(u *model.User, g Group) bool {
	if !g.Private {
		return true
	}
	if u == nil {
		return false
	}
	return u.Level >= 3 || permsOf(u, g).IsMember
}

func registerInvites(a *core.App) {
	requestsT = store.NewTable[GroupRequest](a.DB, "group_requests").
		Index("group", func(r *GroupRequest) string { return i(r.IDGroup) }).
		Index("user", func(r *GroupRequest) string { return i(r.IDUser) }).
		Index("pair", func(r *GroupRequest) string { return store.Key(r.IDGroup, r.IDUser) })
	linksT = store.NewTable[GroupLink](a.DB, "group_links").
		Index("group", func(l *GroupLink) string { return i(l.IDGroup) }).
		Index("token", func(l *GroupLink) string { return l.Token })
	invitesT = store.NewTable[GroupInvite](a.DB, "group_invites").
		Index("group", func(v *GroupInvite) string { return i(v.IDGroup) }).
		Index("to", func(v *GroupInvite) string { return i(v.IDTo) }).
		Index("pair", func(v *GroupInvite) string { return store.Key(v.IDGroup, v.IDTo) })

	a.Describer("group_invite", func(id int64) (core.Desc, bool) {
		inv, ok := invitesT.Get(id)
		if !ok {
			return core.Desc{Title: "Приглашение обработано", Text: "Приглашение уже принято или отклонено", Link: "/groups", Extra: map[string]any{"pending": false}}, true
		}
		g, ok := groupsT.Get(inv.IDGroup)
		if !ok {
			return core.Desc{}, false
		}
		img := ""
		if g.AvatarExt != "" {
			img = "/api/groups/" + i(g.ID) + "/avatar"
		}
		return core.Desc{
			Title: g.Name,
			Text:  a.Text.Plain(g.Opis, 120),
			Link:  "/groups/" + i(g.ID),
			Image: img,
			Owner: inv.IDFrom,
			Extra: map[string]any{"invite_id": inv.ID, "group_id": g.ID, "pending": true},
		}, true
	})

	// ── Заявки на вступление в приватное сообщество
	a.GET("/api/groups/{id}/requests", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		if !permsOf(me, g).Manage {
			return core.ErrForbidden()
		}
		all := requestsT.By("group", i(g.ID))
		sort.Slice(all, func(x, y int) bool { return all[x].Time > all[y].Time })
		items := make([]map[string]any, 0, len(all))
		for _, r := range all {
			items = append(items, map[string]any{"id": r.ID, "user": a.Ref(r.IDUser), "time": r.Time})
		}
		return c.JSON(map[string]any{"items": items})
	}, core.Auth)

	a.POST("/api/groups/{id}/requests/{uid}/approve", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		if !permsOf(me, g).Manage {
			return core.ErrForbidden()
		}
		uid := c.PathInt("uid")
		req, ok := requestsT.OneBy("pair", store.Key(g.ID, uid))
		if !ok {
			return core.ErrNotFound("Заявка не найдена")
		}
		requestsT.Delete(req.ID)
		if _, exists := memberOf(g.ID, uid); !exists {
			membersT.Insert(&GroupMember{IDGroup: g.ID, IDUser: uid, Role: "member", Time: core.Now()})
		}
		a.Notify(me.ID, uid, "group_req_ok", g.ID, false)
		return c.OK()
	}, core.Auth)

	a.POST("/api/groups/{id}/requests/{uid}/reject", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		if !permsOf(me, g).Manage {
			return core.ErrForbidden()
		}
		uid := c.PathInt("uid")
		if req, ok := requestsT.OneBy("pair", store.Key(g.ID, uid)); ok {
			requestsT.Delete(req.ID)
		}
		return c.OK()
	}, core.Auth)

	// ── Уникальные ссылки для автовступления
	a.GET("/api/groups/{id}/links", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		if !permsOf(me, g).Manage {
			return core.ErrForbidden()
		}
		all := linksT.By("group", i(g.ID))
		sort.Slice(all, func(x, y int) bool { return all[x].ID > all[y].ID })
		now := core.Now()
		items := make([]map[string]any, 0, len(all))
		for _, l := range all {
			expired := l.ExpiresAt > 0 && l.ExpiresAt <= now
			exhausted := l.MaxUses > 0 && l.Uses >= l.MaxUses
			items = append(items, map[string]any{
				"id":         l.ID,
				"token":      l.Token,
				"note":       l.Note,
				"max_uses":   l.MaxUses,
				"uses":       l.Uses,
				"expires_at": l.ExpiresAt,
				"time":       l.Time,
				"active":     !expired && !exhausted,
				"expired":    expired,
				"exhausted":  exhausted,
			})
		}
		return c.JSON(map[string]any{"items": items})
	}, core.Auth)

	a.POST("/api/groups/{id}/links", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		if !permsOf(me, g).Manage {
			return core.ErrForbidden()
		}
		var in struct {
			Note         string `json:"note"`
			MaxUses      int64  `json:"max_uses"`      // 0 — безлимит
			ExpiresHours int64  `json:"expires_hours"` // 0 — бессрочная
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if in.MaxUses < 0 {
			in.MaxUses = 0
		}
		if in.ExpiresHours < 0 {
			in.ExpiresHours = 0
		}
		now := core.Now()
		var exp int64
		if in.ExpiresHours > 0 {
			exp = now + in.ExpiresHours*3600
		}
		note := a.Text.Clean(strings.TrimSpace(in.Note))
		if len([]rune(note)) > 64 {
			note = core.Trunc(note, 64)
		}
		tok := core.RandToken(12)
		id := linksT.Insert(&GroupLink{
			IDGroup: g.ID, IDUser: me.ID, Token: tok, Note: note,
			MaxUses: in.MaxUses, ExpiresAt: exp, Time: now,
		})
		return c.OK("id", id, "token", tok)
	}, core.Auth)

	a.DELETE("/api/group-link/{lid}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		l, ok := linksT.Get(c.PathInt("lid"))
		if !ok {
			return core.ErrNotFound()
		}
		g, _ := groupsT.Get(l.IDGroup)
		if !permsOf(me, g).Manage {
			return core.ErrForbidden()
		}
		linksT.Delete(l.ID)
		return c.OK()
	}, core.Auth)

	// Предпросмотр и автовступление по уникальной ссылке
	a.GET("/api/group-join/{token}", func(c *core.Ctx) error {
		l, ok := linksT.OneBy("token", c.Path("token"))
		if !ok {
			return core.ErrNotFound("Ссылка-приглашение недействительна")
		}
		g, ok := groupsT.Get(l.IDGroup)
		if !ok {
			return core.ErrNotFound("Сообщество не найдено")
		}
		now := core.Now()
		if l.ExpiresAt > 0 && l.ExpiresAt <= now {
			return core.ErrBad("Срок действия ссылки истёк")
		}
		if l.MaxUses > 0 && l.Uses >= l.MaxUses {
			return core.ErrBad("Лимит вступлений по этой ссылке исчерпан")
		}
		return c.JSON(map[string]any{"group": groupCard(a, g, c.User()), "note": l.Note})
	})

	a.POST("/api/group-join/{token}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		l, ok := linksT.OneBy("token", c.Path("token"))
		if !ok {
			return core.ErrNotFound("Ссылка-приглашение недействительна")
		}
		g, ok := groupsT.Get(l.IDGroup)
		if !ok {
			return core.ErrNotFound("Сообщество не найдено")
		}
		if _, exists := memberOf(g.ID, me.ID); exists {
			return c.OK("group_id", g.ID, "already", true)
		}
		now := core.Now()
		if l.ExpiresAt > 0 && l.ExpiresAt <= now {
			return core.ErrBad("Срок действия ссылки истёк")
		}
		if l.MaxUses > 0 && l.Uses >= l.MaxUses {
			return core.ErrBad("Лимит вступлений по этой ссылке исчерпан")
		}
		membersT.Insert(&GroupMember{IDGroup: g.ID, IDUser: me.ID, Role: "member", Time: now})
		linksT.Update(l.ID, func(x *GroupLink) { x.Uses++ })
		if req, ok := requestsT.OneBy("pair", store.Key(g.ID, me.ID)); ok {
			requestsT.Delete(req.ID)
		}
		if inv, ok := invitesT.OneBy("pair", store.Key(g.ID, me.ID)); ok {
			invitesT.Delete(inv.ID)
		}
		return c.OK("group_id", g.ID)
	}, core.Auth)

	// ── Приглашения друзей в сообщество
	a.GET("/api/groups/{id}/invitable-friends", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		if !permsOf(me, g).IsMember {
			return core.ErrForbidden("Вступите в сообщество, чтобы приглашать друзей")
		}
		out := []map[string]any{}
		for _, f := range a.T.Friends.By("user", i(me.ID)) {
			if f.I != 0 {
				continue
			}
			fid := f.Frend
			if _, isMem := memberOf(g.ID, fid); isMem {
				continue
			}
			if _, already := invitesT.OneBy("pair", store.Key(g.ID, fid)); already {
				continue
			}
			if a.UserSettings(fid).BlockGroupInvites {
				continue
			}
			out = append(out, map[string]any{"user": a.Ref(fid)})
		}
		return c.JSON(map[string]any{"items": out})
	}, core.Auth)

	a.POST("/api/groups/{id}/invite", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Сообщество не найдено")
		}
		if !permsOf(me, g).IsMember {
			return core.ErrForbidden("Вступите в сообщество, чтобы приглашать друзей")
		}
		var in struct {
			UserID int64 `json:"user_id"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if in.UserID == me.ID || !a.T.Users.Has(in.UserID) {
			return core.ErrBad("Пользователь не найден")
		}
		if !a.IsFriend(me.ID, in.UserID) {
			return core.ErrForbidden("Приглашать в сообщество можно только своих друзей")
		}
		if a.UserSettings(in.UserID).BlockGroupInvites {
			return core.ErrForbidden("Пользователь запретил приглашения в сообщества")
		}
		if _, isMem := memberOf(g.ID, in.UserID); isMem {
			return core.ErrBad("Пользователь уже состоит в этом сообществе")
		}
		if _, already := invitesT.OneBy("pair", store.Key(g.ID, in.UserID)); already {
			return core.ErrBad("Приглашение этому другу уже отправлено")
		}
		invID := invitesT.Insert(&GroupInvite{IDGroup: g.ID, IDFrom: me.ID, IDTo: in.UserID, Time: core.Now()})
		a.Notify(me.ID, in.UserID, "group_invite", invID, false)
		return c.OK("id", invID)
	}, core.Auth)

	a.POST("/api/group-invite/{invId}/accept", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		inv, ok := invitesT.Get(c.PathInt("invId"))
		if !ok || inv.IDTo != me.ID {
			return core.ErrNotFound("Приглашение не найдено")
		}
		g, ok := groupsT.Get(inv.IDGroup)
		if !ok {
			invitesT.Delete(inv.ID)
			return core.ErrNotFound("Сообщество удалено")
		}
		if _, isMem := memberOf(g.ID, me.ID); !isMem {
			membersT.Insert(&GroupMember{IDGroup: g.ID, IDUser: me.ID, Role: "member", Time: core.Now()})
		}
		if req, ok := requestsT.OneBy("pair", store.Key(g.ID, me.ID)); ok {
			requestsT.Delete(req.ID)
		}
		invitesT.Delete(inv.ID)
		return c.OK("group_id", g.ID)
	}, core.Auth)

	a.POST("/api/group-invite/{invId}/decline", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		inv, ok := invitesT.Get(c.PathInt("invId"))
		if !ok || inv.IDTo != me.ID {
			return core.ErrNotFound("Приглашение не найдено")
		}
		invitesT.Delete(inv.ID)
		return c.OK()
	}, core.Auth)
}
