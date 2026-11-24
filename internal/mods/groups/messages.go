package groups

import (
	"encoding/json"
	"sort"
	"strings"
	"unicode/utf8"

	"dcms/internal/core"
	"dcms/internal/mods/feeds"
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
	ReplyID   int64  `json:"reply_id"`
	AttType   string `json:"att_type"` // foto | files | shorts
	AttID     int64  `json:"att_id"`
	Pinned    bool   `json:"pinned"`
	InlineKB  string `json:"inline_kb"` // JSON [][]BotButton
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

func totalUnreadUserGroupMsgs(uid int64) int {
	if dmT == nil || uid == 0 {
		return 0
	}
	n := 0
	for _, m := range dmT.By("user", i(uid)) {
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

	feeds.RegisterExtraUnread(totalUnreadUserGroupMsgs)

	a.Describer("group_dm", func(id int64) (core.Desc, bool) {
		g, ok := groupsT.Get(id)
		if !ok {
			return core.Desc{}, false
		}
		return core.Desc{Title: g.Name, Link: "/mail/group/" + i(g.ID), Owner: g.IDCreator}, true
	})

	attSummary := func(attType string, attID int64) string {
		if attType == "" || attID == 0 {
			return ""
		}
		switch attType {
		case "foto":
			return "📷 Фотография"
		case "shorts":
			return "🎬 Клип"
		default:
			return "📎 Файл"
		}
	}

	msgDTO := func(g Group, m GroupMessage, showStaff bool) map[string]any {
		out := map[string]any{
			"id":         m.ID,
			"group_id":   m.IDGroup,
			"user_id":    m.IDUser,
			"from_group": m.FromGroup,
			"msg":        a.Text.Render(m.Msg),
			"raw":        m.Msg,
			"time":       m.Time,
			"pinned":     m.Pinned,
			"read":       (m.FromGroup && m.ReadByUsr) || (!m.FromGroup && m.ReadByGrp),
		}
		if showStaff && m.FromGroup && m.IDStaff > 0 {
			out["staff"] = a.Ref(m.IDStaff)
		}
		if m.ReplyID > 0 {
			if rm, ok := dmT.Get(m.ReplyID); ok && rm.IDGroup == m.IDGroup && rm.IDUser == m.IDUser {
				nick := g.Name
				if !rm.FromGroup {
					nick = a.Ref(rm.IDUser).Nick
				}
				txt := a.Text.Plain(rm.Msg, 80)
				if txt == "" {
					txt = attSummary(rm.AttType, rm.AttID)
				}
				out["reply"] = map[string]any{"id": rm.ID, "nick": nick, "text": txt}
			}
		}
		if m.AttType != "" && m.AttID > 0 {
			if desc, ok := a.Describe(m.AttType, m.AttID); ok {
				out["attach"] = map[string]any{
					"type":  m.AttType,
					"id":    m.AttID,
					"title": desc.Title,
					"text":  desc.Text,
					"link":  desc.Link,
					"image": desc.Image,
				}
			}
		}
		if m.InlineKB != "" {
			var kb [][]BotButton
			if err := json.Unmarshal([]byte(m.InlineKB), &kb); err == nil && len(kb) > 0 {
				out["inline_keyboard"] = kb
			}
		}
		return out
	}

	pinnedOf := func(g Group, all []GroupMessage) map[string]any {
		for idx := len(all) - 1; idx >= 0; idx-- {
			m := all[idx]
			if m.Pinned {
				nick := g.Name
				if !m.FromGroup {
					nick = a.Ref(m.IDUser).Nick
				}
				txt := a.Text.Plain(m.Msg, 80)
				if txt == "" {
					txt = attSummary(m.AttType, m.AttID)
				}
				return map[string]any{"id": m.ID, "nick": nick, "text": txt}
			}
		}
		return nil
	}

	validateSend := func(senderID int64, rawMsg, attType string, attID int64) (string, error) {
		msg := strings.TrimSpace(rawMsg)
		hasAtt := attType != "" && attID > 0
		if hasAtt {
			if attType != "foto" && attType != "files" && attType != "shorts" {
				return "", core.ErrBad("Неподдерживаемый тип вложения")
			}
			desc, ok := a.Describe(attType, attID)
			if !ok || desc.Owner != senderID {
				return "", core.ErrForbidden("Можно прикреплять только свои фото, файлы и клипы")
			}
		}
		if n := utf8.RuneCountInString(msg); n < 1 && !hasAtt {
			return "", core.ErrBad("Введите сообщение или прикрепите вложение")
		} else if n > 2000 {
			return "", core.ErrBad("Сообщение слишком длинное (максимум 2000 символов)")
		}
		if msg == "" {
			return "", nil
		}
		return a.Text.Clean(msg), nil
	}

	// ── Список диалогов пользователя с сообществами (для отображения прямо в основной Почте /mail)
	a.GET("/api/my-group-dialogs", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		type agg struct {
			g      Group
			last   GroupMessage
			unread int
		}
		byGrp := map[int64]*agg{}
		for _, m := range dmT.By("user", i(me.ID)) {
			g, ok := groupsT.Get(m.IDGroup)
			if !ok {
				continue
			}
			aItem, exists := byGrp[g.ID]
			if !exists {
				aItem = &agg{g: g, last: m}
				byGrp[g.ID] = aItem
			}
			if m.ID > aItem.last.ID {
				aItem.last = m
			}
			if m.FromGroup && !m.ReadByUsr {
				aItem.unread++
			}
		}
		list := make([]*agg, 0, len(byGrp))
		for _, v := range byGrp {
			list = append(list, v)
		}
		sort.Slice(list, func(x, y int) bool { return list[x].last.Time > list[y].last.Time })
		items := make([]map[string]any, 0, len(list))
		for _, v := range list {
			ava := ""
			if v.g.AvatarExt != "" {
				ava = "/api/groups/" + i(v.g.ID) + "/avatar?v=" + i(v.g.Time)
			}
			txt := a.Text.Plain(v.last.Msg, 100)
			if txt == "" {
				txt = attSummary(v.last.AttType, v.last.AttID)
			}
			items = append(items, map[string]any{
				"kind": "group",
				"group": map[string]any{
					"id":     v.g.ID,
					"name":   v.g.Name,
					"avatar": ava,
				},
				"new":  v.unread,
				"time": v.last.Time,
				"last": map[string]any{
					"text": txt,
					"time": v.last.Time,
					"mine": !v.last.FromGroup,
				},
			})
		}
		return c.JSON(map[string]any{"items": items})
	}, core.Auth)

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
		pinned := pinnedOf(g, all)
		if len(all) > 150 {
			all = all[len(all)-150:]
		}
		items := make([]map[string]any, 0, len(all))
		for _, m := range all {
			items = append(items, msgDTO(g, m, false))
		}
		return c.JSON(map[string]any{
			"items":        items,
			"pinned":       pinned,
			"can_write":    g.EnableMessages && !g.Blocked,
			"keyboard":     getUserBotKeyboard(g.ID, me.ID),
			"bot_commands": BotGetCommands(g),
		})
	}, core.Auth)

	// ── Пользователь нажал inline-кнопку под конкретным сообщением бота
	a.POST("/api/groups/{id}/dm/callback", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok || g.Blocked || !g.EnableMessages {
			return core.ErrForbidden("Сообщения сообщества недоступны")
		}
		var in struct {
			MessageID    int64  `json:"message_id"`
			CallbackData string `json:"callback_data"`
			Text         string `json:"text"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		m, ok := dmT.Get(in.MessageID)
		if !ok || m.IDGroup != g.ID || m.IDUser != me.ID {
			return core.ErrNotFound("Сообщение не найдено")
		}
		EmitUpdate(g.ID, "inline_button_click", "messages", map[string]any{
			"message_id":    m.ID,
			"from":          a.Ref(me.ID),
			"callback_data": in.CallbackData,
			"text":          in.Text,
			"time":          core.Now(),
		})
		return c.OK()
	}, core.Auth)

	// ── Пользователь нажал кнопку в нижней клавиатуре под полем ввода
	a.POST("/api/groups/{id}/dm/keyboard-click", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok || g.Blocked || !g.EnableMessages {
			return core.ErrForbidden("Сообщения сообщества недоступны")
		}
		var in struct {
			CallbackData string `json:"callback_data"`
			Text         string `json:"text"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		removed := false
		if kb, ok := botKBT.OneBy("pair", store.Key(g.ID, me.ID)); ok && kb.OneTime {
			botKBT.Delete(kb.ID)
			removed = true
		}
		EmitUpdate(g.ID, "keyboard_button_click", "messages", map[string]any{
			"from":          a.Ref(me.ID),
			"callback_data": in.CallbackData,
			"text":          in.Text,
			"time":          core.Now(),
		})
		return c.OK("keyboard_removed", removed)
	}, core.Auth)

	// ── Пометить входящие от сообщества прочитанными (вызывается при живом SSE-сообщении в открытом диалоге)
	a.POST("/api/groups/{id}/dm/read", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		gid := c.PathInt("id")
		for _, m := range dmT.By("pair", store.Key(gid, me.ID)) {
			if m.FromGroup && !m.ReadByUsr {
				dmT.Update(m.ID, func(x *GroupMessage) { x.ReadByUsr = true })
			}
		}
		return c.OK()
	}, core.Auth)

	// ── Пометить входящие от пользователя прочитанными (для сотрудников сообщества в открытом диалоге)
	a.POST("/api/groups/{id}/inbox/{uid}/read", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		if !permsOf(me, g).ModMessages {
			return core.ErrForbidden()
		}
		uid := c.PathInt("uid")
		for _, m := range dmT.By("pair", store.Key(g.ID, uid)) {
			if !m.FromGroup && !m.ReadByGrp {
				dmT.Update(m.ID, func(x *GroupMessage) { x.ReadByGrp = true })
			}
		}
		return c.OK()
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
			Msg     string `json:"msg"`
			ReplyID int64  `json:"reply_id"`
			AttType string `json:"att_type"`
			AttID   int64  `json:"att_id"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		txt, err := validateSend(me.ID, in.Msg, in.AttType, in.AttID)
		if err != nil {
			return err
		}
		replyID := in.ReplyID
		if replyID > 0 {
			if rm, ok := dmT.Get(replyID); !ok || rm.IDGroup != g.ID || rm.IDUser != me.ID {
				replyID = 0
			}
		}
		id := dmT.Insert(&GroupMessage{
			IDGroup:   g.ID,
			IDUser:    me.ID,
			FromGroup: false,
			Msg:       txt,
			Time:      core.Now(),
			ReadByUsr: true,
			ReadByGrp: false,
			ReplyID:   replyID,
			AttType:   in.AttType,
			AttID:     in.AttID,
		})
		m, _ := dmT.Get(id)
		dto := msgDTO(g, m, false)
		a.Hub.Publish("group_dm:"+i(g.ID), "group_dm", dto)
		EmitUpdate(g.ID, "message_new", "messages", map[string]any{
			"id":   m.ID,
			"from": a.Ref(me.ID),
			"text": m.Msg,
			"time": m.Time,
		})
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
				txt := a.Text.Plain(m.Msg, 100)
				if txt == "" {
					txt = attSummary(m.AttType, m.AttID)
				}
				d.lastMsg = txt
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
		pinned := pinnedOf(g, all)
		if len(all) > 150 {
			all = all[len(all)-150:]
		}
		items := make([]map[string]any, 0, len(all))
		for _, m := range all {
			items = append(items, msgDTO(g, m, true))
		}
		return c.JSON(map[string]any{
			"peer":      a.Ref(targetUID),
			"items":     items,
			"pinned":    pinned,
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
			Msg     string `json:"msg"`
			ReplyID int64  `json:"reply_id"`
			AttType string `json:"att_type"`
			AttID   int64  `json:"att_id"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		txt, err := validateSend(me.ID, in.Msg, in.AttType, in.AttID)
		if err != nil {
			return err
		}
		replyID := in.ReplyID
		if replyID > 0 {
			if rm, ok := dmT.Get(replyID); !ok || rm.IDGroup != g.ID || rm.IDUser != targetUID {
				replyID = 0
			}
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
			ReplyID:   replyID,
			AttType:   in.AttType,
			AttID:     in.AttID,
		})
		m, _ := dmT.Get(id)
		staffDTO := msgDTO(g, m, true)
		userDTO := msgDTO(g, m, false)
		a.Hub.Publish("group_dm:"+i(g.ID), "group_dm", staffDTO)
		a.Hub.ToUser(targetUID, "group_dm", userDTO)
		a.Hub.ToUser(targetUID, "counters", nil)
		a.Notify(me.ID, targetUID, "group_dm", g.ID, false)
		return c.JSON(staffDTO)
	}, core.Auth)

	// ── Закрепить / открепить сообщение в диалоге с сообществом
	a.POST("/api/group-dm/{mid}/pin", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		m, ok := dmT.Get(c.PathInt("mid"))
		if !ok {
			return core.ErrNotFound()
		}
		g, ok := groupsT.Get(m.IDGroup)
		if !ok {
			return core.ErrNotFound()
		}
		if m.IDUser != me.ID && !permsOf(me, g).ModMessages {
			return core.ErrForbidden()
		}
		want := !m.Pinned
		if want {
			for _, x := range dmT.By("pair", store.Key(m.IDGroup, m.IDUser)) {
				if x.Pinned && x.ID != m.ID {
					dmT.Update(x.ID, func(z *GroupMessage) { z.Pinned = false })
				}
			}
		}
		dmT.Update(m.ID, func(x *GroupMessage) { x.Pinned = want })
		return c.OK("pinned", want)
	}, core.Auth)

	// ── Удалить сообщение в диалоге с сообществом
	a.DELETE("/api/group-dm/{mid}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		m, ok := dmT.Get(c.PathInt("mid"))
		if !ok {
			return core.ErrNotFound()
		}
		g, ok := groupsT.Get(m.IDGroup)
		if !ok {
			return core.ErrNotFound()
		}
		isStaff := permsOf(me, g).ModMessages
		if !isStaff && (m.IDUser != me.ID || m.FromGroup) {
			return core.ErrForbidden("Можно удалять только свои сообщения")
		}
		dmT.Delete(m.ID)
		return c.OK()
	}, core.Auth)
}
