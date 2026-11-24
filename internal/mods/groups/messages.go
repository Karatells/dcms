package groups

import (
	"sort"
	"strings"
	"unicode/utf8"

	"dcms/internal/core"
	"dcms/internal/store"
)

// GroupMessage — личное сообщение между пользователем и сообществом.
// Сообщество никогда не может написать первым: диалог всегда начинает пользователь (FromGroup=false).
type GroupMessage struct {
	store.Base
	IDGroup   int64  `json:"id_group"`
	IDUser    int64  `json:"id_user"`    // пользователь-собеседник
	IDStaff   int64  `json:"id_staff"`   // id сотрудника сообщества, если FromGroup=true
	FromGroup bool   `json:"from_group"` // false: пользователь -> сообщество, true: сообщество -> пользователь
	Msg       string `json:"msg"`
	Time      int64  `json:"time"`
	ReadByGrp bool   `json:"read_by_grp"`
	ReadByUsr bool   `json:"read_by_usr"`
}

var dmT *store.Table[GroupMessage, *GroupMessage]

func hasUserInitiated(gid, uid int64) bool {
	for _, m := range dmT.By("pair", store.Key(gid, uid)) {
		if !m.FromGroup {
			return true
		}
	}
	return false
}

func unreadGroupMsgs(gid int64) int {
	if dmT == nil {
		return 0
	}
	n := 0
	for _, m := range dmT.By("group", i(gid)) {
		if !m.FromGroup && !m.ReadByGrp {
			n++
		}
	}
	return n
}

func unreadUserGroupMsgs(gid, uid int64) int {
	if dmT == nil || uid == 0 {
		return 0
	}
	n := 0
	for _, m := range dmT.By("pair", store.Key(gid, uid)) {
		if m.FromGroup && !m.ReadByUsr {
			n++
		}
	}
	return n
}

func registerMessages(a *core.App) {
	dmT = store.NewTable[GroupMessage](a.DB, "group_messages").
		Index("group", func(x *GroupMessage) string { return i(x.IDGroup) }).
		Index("pair", func(x *GroupMessage) string { return store.Key(x.IDGroup, x.IDUser) }).
		Index("user", func(x *GroupMessage) string { return i(x.IDUser) })

	a.Describer("group_dm", func(id int64) (core.Desc, bool) {
		g, ok := groupsT.Get(id)
		if !ok {
			return core.Desc{}, false
		}
		return core.Desc{Title: g.Name, Link: "/groups/" + i(g.ID) + "/message", Owner: g.IDCreator}, true
	})

	msgDTO := func(m GroupMessage, showStaff bool) map[string]any {
		out := map[string]any{
			"id":         m.ID,
			"group_id":   m.IDGroup,
			"user_id":    m.IDUser,
			"from_group": m.FromGroup,
			"msg":        a.Text.Render(m.Msg),
			"raw":        m.Msg,
			"time":       m.Time,
		}
		if showStaff && m.FromGroup && m.IDStaff > 0 {
			out["staff"] = a.Ref(m.IDStaff)
		}
		return out
	}

	// ── Пользователь: открыть диалог с сообществом
	a.GET("/api/groups/{id}/dm", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Сообщество не найдено")
		}
		if g.Blocked && !canBlockGroup(me) {
			return core.ErrForbidden("Сообщество заблокировано")
		}
		all := dmT.By("pair", store.Key(g.ID, me.ID))
		if !g.EnableMessages && len(all) == 0 {
			return core.ErrForbidden("Личные сообщения в этом сообществе отключены")
		}
		sort.Slice(all, func(x, y int) bool { return all[x].ID < all[y].ID })
		for _, m := range all {
			if m.FromGroup && !m.ReadByUsr {
				dmT.Update(m.ID, func(x *GroupMessage) { x.ReadByUsr = true })
			}
		}
		if len(all) > 150 {
			all = all[len(all)-150:]
		}
		items := make([]map[string]any, 0, len(all))
		for _, m := range all {
			items = append(items, msgDTO(m, false))
		}
		return c.JSON(map[string]any{
			"items":     items,
			"can_write": g.EnableMessages && !g.Blocked,
		})
	}, core.Auth)

	// ── Пользователь: написать сообщение сообществу (пользователь всегда начинает диалог первым)
	a.POST("/api/groups/{id}/dm", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Сообщество не найдено")
		}
		if g.Blocked {
			return core.ErrForbidden("Сообщество заблокировано")
		}
		if !g.EnableMessages {
			return core.ErrForbidden("Личные сообщения в этом сообществе отключены")
		}
		var in struct {
			Msg string `json:"msg"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		txt := a.Text.Clean(strings.TrimSpace(in.Msg))
		if n := utf8.RuneCountInString(txt); n < 1 || n > 2000 {
			return core.ErrBad("Сообщение: от 1 до 2000 символов")
		}
		id := dmT.Insert(&GroupMessage{
			IDGroup:   g.ID,
			IDUser:    me.ID,
			FromGroup: false,
			Msg:       txt,
			Time:      core.Now(),
			ReadByUsr: true,
			ReadByGrp: false,
		})
		m, _ := dmT.Get(id)
		dto := msgDTO(m, false)
		a.Hub.Publish("group_dm:"+i(g.ID), "group_dm", dto)
		return c.JSON(dto)
	}, core.Auth)

	// ── Сотрудники сообщества: список входящих диалогов (только те пользователи, которые написали сами)
	a.GET("/api/groups/{id}/inbox", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Сообщество не найдено")
		}
		if !permsOf(me, g).ModMessages {
			return core.ErrForbidden("Нет прав на просмотр сообщений сообщества")
		}
		type dlg struct {
			uid       int64
			initiated bool
			lastMsg   string
			lastTime  int64
			fromGroup bool
			unread    int
		}
		byUser := map[int64]*dlg{}
		for _, m := range dmT.By("group", i(g.ID)) {
			d, exists := byUser[m.IDUser]
			if !exists {
				d = &dlg{uid: m.IDUser}
				byUser[m.IDUser] = d
			}
			if !m.FromGroup {
				d.initiated = true
				if !m.ReadByGrp {
					d.unread++
				}
			}
			if m.Time >= d.lastTime {
				d.lastTime = m.Time
				d.lastMsg = m.Msg
				d.fromGroup = m.FromGroup
			}
		}
		list := make([]*dlg, 0, len(byUser))
		for _, d := range byUser {
			if d.initiated {
				list = append(list, d)
			}
		}
		sort.Slice(list, func(x, y int) bool { return list[x].lastTime > list[y].lastTime })
		items := make([]map[string]any, 0, len(list))
		for _, d := range list {
			items = append(items, map[string]any{
				"user":       a.Ref(d.uid),
				"last_msg":   d.lastMsg,
				"last_time":  d.lastTime,
				"from_group": d.fromGroup,
				"unread":     d.unread,
			})
		}
		return c.JSON(map[string]any{"items": items})
	}, core.Auth)

	// ── Сотрудники сообщества: открыть диалог с конкретным пользователем
	a.GET("/api/groups/{id}/inbox/{uid}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Сообщество не найдено")
		}
		if !permsOf(me, g).ModMessages {
			return core.ErrForbidden("Нет прав на просмотр сообщений сообщества")
		}
		targetUID := c.PathInt("uid")
		if !hasUserInitiated(g.ID, targetUID) {
			return core.ErrForbidden("Сообщество не может писать первым — пользователь должен начать диалог сам")
		}
		all := dmT.By("pair", store.Key(g.ID, targetUID))
		sort.Slice(all, func(x, y int) bool { return all[x].ID < all[y].ID })
		for _, m := range all {
			if !m.FromGroup && !m.ReadByGrp {
				dmT.Update(m.ID, func(x *GroupMessage) { x.ReadByGrp = true })
			}
		}
		if len(all) > 150 {
			all = all[len(all)-150:]
		}
		items := make([]map[string]any, 0, len(all))
		for _, m := range all {
			items = append(items, msgDTO(m, true))
		}
		return c.JSON(map[string]any{
			"peer":      a.Ref(targetUID),
			"items":     items,
			"can_write": !g.Blocked,
		})
	}, core.Auth)

	// ── Сотрудники сообщества: ответить пользователю от имени сообщества (ТОЛЬКО если пользователь писал первым!)
	a.POST("/api/groups/{id}/inbox/{uid}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Сообщество не найдено")
		}
		if !permsOf(me, g).ModMessages {
			return core.ErrForbidden("Нет прав на ответы от имени сообщества")
		}
		if g.Blocked {
			return core.ErrForbidden("Сообщество заблокировано")
		}
		targetUID := c.PathInt("uid")
		if !hasUserInitiated(g.ID, targetUID) {
			return core.ErrForbidden("Сообщество не может писать первым — пользователь должен начать диалог сам")
		}
		var in struct {
			Msg string `json:"msg"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		txt := a.Text.Clean(strings.TrimSpace(in.Msg))
		if n := utf8.RuneCountInString(txt); n < 1 || n > 2000 {
			return core.ErrBad("Сообщение: от 1 до 2000 символов")
		}
		id := dmT.Insert(&GroupMessage{
			IDGroup:   g.ID,
			IDUser:    targetUID,
			IDStaff:   me.ID,
			FromGroup: true,
			Msg:       txt,
			Time:      core.Now(),
			ReadByGrp: true,
			ReadByUsr: false,
		})
		m, _ := dmT.Get(id)
		staffDTO := msgDTO(m, true)
		userDTO := msgDTO(m, false)
		a.Hub.Publish("group_dm:"+i(g.ID), "group_dm", staffDTO)
		a.Hub.ToUser(targetUID, "group_dm", userDTO)
		a.Notify(me.ID, targetUID, "group_dm", g.ID, false)
		return c.JSON(staffDTO)
	}, core.Auth)
}
