package chats

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"dcms/internal/core"
	"dcms/internal/store"
)

// GroupChat — групповая беседа пользователей.
type GroupChat struct {
	store.Base
	Title       string `json:"title"`
	OwnerID     int64  `json:"owner_id"`
	InviteCode  string `json:"invite_code"`
	PinnedMsgID int64  `json:"pinned_msg_id"`
	Created     int64  `json:"created"`
}

// GroupChatMember — участник групповой беседы.
type GroupChatMember struct {
	store.Base
	ChatID   int64  `json:"chat_id"`
	UserID   int64  `json:"user_id"`
	Role     string `json:"role"` // owner | admin | member
	JoinedAt int64  `json:"joined_at"`
}

// GroupChatMsg — сообщение в групповой беседе.
type GroupChatMsg struct {
	store.Base
	ChatID  int64  `json:"chat_id"`
	UserID  int64  `json:"user_id"`
	Msg     string `json:"msg"`
	ReplyID int64  `json:"reply_id"`
	Time    int64  `json:"time"`
}

var (
	chatsT   *store.Table[GroupChat, *GroupChat]
	membersT *store.Table[GroupChatMember, *GroupChatMember]
	msgsT    *store.Table[GroupChatMsg, *GroupChatMsg]
	mentionRe = regexp.MustCompile(`@([A-Za-zА-Яа-яЁё0-9_\-\.]{2,32})`)
)

func genCode() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func i(n int64) string { return store.I(n) }

func memberOf(chatID, uid int64) (GroupChatMember, bool) {
	return membersT.OneBy("pair", store.Key(chatID, uid))
}

func isAdmin(m GroupChatMember) bool {
	return m.Role == "owner" || m.Role == "admin"
}

func msgView(a *core.App, m GroupChatMsg, me int64) map[string]any {
	out := map[string]any{
		"id":   m.ID,
		"chat": m.ChatID,
		"user": a.Ref(m.UserID),
		"mine": m.UserID == me,
		"msg":  a.Text.Render(m.Msg),
		"raw":  m.Msg,
		"time": m.Time,
	}
	if m.ReplyID > 0 {
		if rm, ok := msgsT.Get(m.ReplyID); ok && rm.ChatID == m.ChatID {
			out["reply"] = map[string]any{
				"id":   rm.ID,
				"nick": a.Ref(rm.UserID).Nick,
				"text": a.Text.Plain(rm.Msg, 80),
			}
		}
	}
	return out
}

// Register регистрирует модуль групповых бесед.
func Register(a *core.App) {
	chatsT = store.NewTable[GroupChat](a.DB, "gchats").
		Index("invite", func(x *GroupChat) string { return x.InviteCode })
	membersT = store.NewTable[GroupChatMember](a.DB, "gchat_members").
		Index("chat", func(x *GroupChatMember) string { return i(x.ChatID) }).
		Index("user", func(x *GroupChatMember) string { return i(x.UserID) }).
		Index("pair", func(x *GroupChatMember) string { return store.Key(x.ChatID, x.UserID) })
	msgsT = store.NewTable[GroupChatMsg](a.DB, "gchat_msgs").
		Index("chat", func(x *GroupChatMsg) string { return i(x.ChatID) })

	a.OnUserDelete(func(uid int64) {
		membersT.DeleteBy("user", i(uid))
		msgsT.DeleteWhere(func(m *GroupChatMsg) bool { return m.UserID == uid })
	})

	// Список моих бесед
	a.GET("/api/chats", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		mems := membersT.By("user", i(me.ID))
		type item struct {
			chat     GroupChat
			lastTime int64
			lastText string
			lastNick string
			count    int
			role     string
		}
		var list []item
		for _, m := range mems {
			ch, ok := chatsT.Get(m.ChatID)
			if !ok {
				continue
			}
			it := item{chat: ch, lastTime: ch.Created, count: membersT.CountBy("chat", i(ch.ID)), role: m.Role}
			ms := msgsT.By("chat", i(ch.ID))
			if len(ms) > 0 {
				best := ms[0]
				for _, x := range ms[1:] {
					if x.ID > best.ID {
						best = x
					}
				}
				it.lastTime = best.Time
				it.lastText = a.Text.Plain(best.Msg, 80)
				it.lastNick = a.Ref(best.UserID).Nick
			}
			list = append(list, it)
		}
		sort.Slice(list, func(x, y int) bool { return list[x].lastTime > list[y].lastTime })
		out := make([]map[string]any, 0, len(list))
		for _, it := range list {
			out = append(out, map[string]any{
				"id":        it.chat.ID,
				"title":     it.chat.Title,
				"members":   it.count,
				"role":      it.role,
				"last_time": it.lastTime,
				"last_text": it.lastText,
				"last_nick": it.lastNick,
			})
		}
		return c.JSON(map[string]any{"items": out})
	}, core.Auth)

	// Создать беседу
	a.POST("/api/chats", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		var in struct {
			Title     string  `json:"title"`
			MemberIDs []int64 `json:"member_ids"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		title := strings.TrimSpace(in.Title)
		if utf8.RuneCountInString(title) < 2 || utf8.RuneCountInString(title) > 64 {
			return core.ErrBad("Название беседы должно быть от 2 до 64 символов")
		}
		now := core.Now()
		ch := GroupChat{Title: title, OwnerID: me.ID, InviteCode: genCode(), Created: now}
		chatsT.Insert(&ch)
		membersT.Insert(&GroupChatMember{ChatID: ch.ID, UserID: me.ID, Role: "owner", JoinedAt: now})
		seen := map[int64]bool{me.ID: true}
		for _, uid := range in.MemberIDs {
			if uid <= 0 || seen[uid] || !a.T.Users.Has(uid) {
				continue
			}
			seen[uid] = true
			membersT.Insert(&GroupChatMember{ChatID: ch.ID, UserID: uid, Role: "member", JoinedAt: now})
		}
		return c.OK("id", ch.ID)
	}, core.Auth)

	// Вступить по инвайт-коду
	a.POST("/api/chat-join/{code}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		code := strings.TrimSpace(c.Path("code"))
		ch, ok := chatsT.OneBy("invite", code)
		if !ok {
			return core.ErrNotFound("Беседа по этой ссылке не найдена")
		}
		if _, exists := memberOf(ch.ID, me.ID); !exists {
			membersT.Insert(&GroupChatMember{ChatID: ch.ID, UserID: me.ID, Role: "member", JoinedAt: core.Now()})
		}
		return c.OK("id", ch.ID)
	}, core.Auth)

	// Детали и сообщения беседы
	a.GET("/api/chats/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		ch, ok := chatsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Беседа не найдена")
		}
		myMem, ok := memberOf(ch.ID, me.ID)
		if !ok && me.Level < 7 {
			return core.ErrForbidden("Вы не состоите в этой беседе")
		}
		p := c.Page()
		list, total := msgsT.Select(store.Opts[GroupChatMsg]{Index: "chat", Key: i(ch.ID), Desc: true, Offset: p.Offset, Limit: p.Per})
		items := make([]map[string]any, 0, len(list))
		for _, m := range list {
			items = append(items, msgView(a, m, me.ID))
		}
		var pinned map[string]any
		if ch.PinnedMsgID > 0 {
			if pm, ok := msgsT.Get(ch.PinnedMsgID); ok && pm.ChatID == ch.ID {
				pinned = map[string]any{"id": pm.ID, "nick": a.Ref(pm.UserID).Nick, "text": a.Text.Plain(pm.Msg, 90)}
			}
		}
		mems := membersT.By("chat", i(ch.ID))
		memOut := make([]map[string]any, 0, len(mems))
		for _, m := range mems {
			memOut = append(memOut, map[string]any{"user": a.Ref(m.UserID), "role": m.Role, "joined": m.JoinedAt})
		}
		return c.JSON(map[string]any{
			"id":          ch.ID,
			"title":       ch.Title,
			"owner_id":    ch.OwnerID,
			"invite_code": ch.InviteCode,
			"my_role":     myMem.Role,
			"pinned":      pinned,
			"members":     memOut,
			"items":       items,
			"total":       total,
			"page":        p.Page,
			"pages":       max(1, (total+p.Per-1)/p.Per),
		})
	}, core.Auth)

	// Отправить сообщение в беседу
	a.POST("/api/chats/{id}/msg", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if _, ok := a.ActiveBan(me.ID, "all"); ok {
			return core.ErrForbidden("Вы заблокированы")
		}
		ch, ok := chatsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Беседа не найдена")
		}
		if _, ok := memberOf(ch.ID, me.ID); !ok {
			return core.ErrForbidden("Вы не состоите в этой беседе")
		}
		var in struct {
			Msg     string `json:"msg"`
			ReplyID int64  `json:"reply_id"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		txt := strings.TrimSpace(in.Msg)
		if n := utf8.RuneCountInString(txt); n < 1 || n > 1024 {
			return core.ErrBad("Сообщение должно быть от 1 до 1024 символов")
		}
		clean := a.Text.Clean(txt)
		m := GroupChatMsg{ChatID: ch.ID, UserID: me.ID, Msg: clean, ReplyID: in.ReplyID, Time: core.Now()}
		msgsT.Insert(&m)
		// Упоминания @ник
		notified := map[int64]bool{me.ID: true}
		for _, sub := range mentionRe.FindAllStringSubmatch(txt, 5) {
			if target, ok := a.UserByNick(sub[1]); ok && !notified[target.ID] {
				if _, isM := memberOf(ch.ID, target.ID); isM {
					notified[target.ID] = true
					a.Notify(me.ID, target.ID, "mention", ch.ID, true)
				}
			}
		}
		// Рассылка SSE всем участникам
		for _, mem := range membersT.By("chat", i(ch.ID)) {
			a.Hub.ToUser(mem.UserID, "gchat_msg", msgView(a, m, mem.UserID))
		}
		return c.JSON(msgView(a, m, me.ID))
	}, core.Auth)

	// Удалить сообщение
	a.DELETE("/api/chats/msg/{mid}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		m, ok := msgsT.Get(c.PathInt("mid"))
		if !ok {
			return core.ErrNotFound()
		}
		mem, _ := memberOf(m.ChatID, me.ID)
		if m.UserID != me.ID && !isAdmin(mem) && me.Level < 5 {
			return core.ErrForbidden()
		}
		msgsT.Delete(m.ID)
		return c.OK()
	}, core.Auth)

	// Закрепить / открепить сообщение
	a.POST("/api/chats/{id}/pin", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		ch, ok := chatsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		mem, ok := memberOf(ch.ID, me.ID)
		if !ok || !isAdmin(mem) {
			return core.ErrForbidden("Закреплять могут только администраторы беседы")
		}
		var in struct {
			MsgID int64 `json:"msg_id"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		want := in.MsgID
		if ch.PinnedMsgID == want {
			want = 0
		}
		chatsT.Update(ch.ID, func(x *GroupChat) { x.PinnedMsgID = want })
		return c.OK("pinned_msg_id", want)
	}, core.Auth)

	// Добавить участника по нику или ID
	a.POST("/api/chats/{id}/members", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		ch, ok := chatsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		mem, ok := memberOf(ch.ID, me.ID)
		if !ok || !isAdmin(mem) {
			return core.ErrForbidden("Добавлять участников могут только администраторы")
		}
		var in struct {
			Nick   string `json:"nick"`
			UserID int64  `json:"user_id"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		var uid int64
		if in.UserID > 0 {
			uid = in.UserID
		} else if nick := strings.TrimSpace(in.Nick); nick != "" {
			u, ok := a.UserByNick(nick)
			if !ok {
				return core.ErrNotFound("Пользователь не найден")
			}
			uid = u.ID
		}
		if uid <= 0 || !a.T.Users.Has(uid) {
			return core.ErrBad("Укажите пользователя")
		}
		if _, exists := memberOf(ch.ID, uid); exists {
			return core.ErrBad("Пользователь уже в беседе")
		}
		membersT.Insert(&GroupChatMember{ChatID: ch.ID, UserID: uid, Role: "member", JoinedAt: core.Now()})
		return c.OK()
	}, core.Auth)

	// Выйти или исключить участника
	a.DELETE("/api/chats/{id}/members/{uid}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		ch, ok := chatsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		targetUID := c.PathInt("uid")
		myMem, ok := memberOf(ch.ID, me.ID)
		if !ok {
			return core.ErrForbidden()
		}
		if targetUID != me.ID && !isAdmin(myMem) {
			return core.ErrForbidden("Недостаточно прав")
		}
		tMem, ok := memberOf(ch.ID, targetUID)
		if !ok {
			return core.ErrNotFound()
		}
		if tMem.Role == "owner" && targetUID != me.ID {
			return core.ErrForbidden("Нельзя исключить создателя беседы")
		}
		membersT.Delete(tMem.ID)
		return c.OK()
	}, core.Auth)

	// Назначить/снять админа беседы
	a.POST("/api/chats/{id}/role", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		ch, ok := chatsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		if ch.OwnerID != me.ID {
			return core.ErrForbidden("Роли меняет только создатель беседы")
		}
		var in struct {
			UserID int64  `json:"user_id"`
			Role   string `json:"role"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if in.Role != "admin" && in.Role != "member" {
			return core.ErrBad("Недопустимая роль")
		}
		tMem, ok := memberOf(ch.ID, in.UserID)
		if !ok || tMem.Role == "owner" {
			return core.ErrNotFound()
		}
		membersT.Update(tMem.ID, func(x *GroupChatMember) { x.Role = in.Role })
		return c.OK()
	}, core.Auth)
}
