package chats

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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
	Role     string `json:"role"`   // owner | admin | member
	Folder   string `json:"folder"` // common | favorite | ignor | deleted ("" == common)
	JoinedAt int64  `json:"joined_at"`
}

// GroupChatMsg — сообщение в групповой беседе (полный паритет с ЛС).
type GroupChatMsg struct {
	store.Base
	ChatID   int64  `json:"chat_id"`
	UserID   int64  `json:"user_id"`
	Msg      string `json:"msg"`
	ReplyID  int64  `json:"reply_id"`
	AttType  string `json:"att_type,omitempty"`
	AttID    int64  `json:"att_id,omitempty"`
	EditedAt int64  `json:"edited_at,omitempty"`
	FwdFrom  int64  `json:"fwd_from,omitempty"`
	VoiceExt string `json:"voice_ext,omitempty"`
	VoiceDur int    `json:"voice_dur,omitempty"`
	Sticker  string `json:"sticker,omitempty"`
	Time     int64  `json:"time"`
}

// GroupChatReaction — эмодзи-реакция на сообщение в беседе.
type GroupChatReaction struct {
	store.Base
	IDMsg  int64  `json:"id_msg"`
	IDUser int64  `json:"id_user"`
	Emoji  string `json:"emoji"`
}

var (
	chatsT        *store.Table[GroupChat, *GroupChat]
	membersT      *store.Table[GroupChatMember, *GroupChatMember]
	msgsT         *store.Table[GroupChatMsg, *GroupChatMsg]
	reactsT       *store.Table[GroupChatReaction, *GroupChatReaction]
	mentionRe     = regexp.MustCompile(`@([A-Za-zА-Яа-яЁё0-9_\-\.]{2,32})`)
	allowedReacts = map[string]bool{"👍": true, "❤️": true, "😂": true, "🔥": true, "😱": true, "👎": true}
	vipStickers   = map[string]bool{"👑": true, "💎": true, "🛸": true, "🌌": true, "🦄": true, "🐉": true, "⚡": true, "🔮": true, "🏆": true, "🎭": true, "🌠": true, "🦚": true}
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

func normFolder(f string) string {
	switch f {
	case "favorite", "ignor", "deleted":
		return f
	default:
		return "common"
	}
}

func reactionsOf(msgID, me int64) []map[string]any {
	if reactsT == nil {
		return nil
	}
	list := reactsT.By("msg", i(msgID))
	if len(list) == 0 {
		return nil
	}
	type agg struct {
		count int
		mine  bool
	}
	byE := map[string]*agg{}
	var order []string
	for _, r := range list {
		a, ok := byE[r.Emoji]
		if !ok {
			a = &agg{}
			byE[r.Emoji] = a
			order = append(order, r.Emoji)
		}
		a.count++
		if r.IDUser == me {
			a.mine = true
		}
	}
	out := make([]map[string]any, 0, len(order))
	for _, e := range order {
		out = append(out, map[string]any{"emoji": e, "count": byE[e].count, "mine": byE[e].mine})
	}
	return out
}

func msgPreview(a *core.App, m GroupChatMsg) string {
	if s := a.Text.Plain(m.Msg, 85); s != "" {
		return s
	}
	if m.VoiceExt != "" {
		return "🎤 Голосовое сообщение"
	}
	if m.Sticker != "" {
		return m.Sticker + " Стикер"
	}
	if m.AttType != "" {
		return map[string]string{"foto": "📷 Фотография", "files": "📎 Файл", "shorts": "🎬 Клип"}[m.AttType]
	}
	return ""
}

func msgView(a *core.App, m GroupChatMsg, me int64, pinnedID int64) map[string]any {
	out := map[string]any{
		"id":        m.ID,
		"chat":      m.ChatID,
		"user":      a.Ref(m.UserID),
		"mine":      m.UserID == me,
		"msg":       a.Text.Render(m.Msg),
		"raw":       m.Msg,
		"time":      m.Time,
		"pinned":    pinnedID > 0 && pinnedID == m.ID,
		"edited_at": m.EditedAt,
	}
	if m.FwdFrom > 0 {
		out["fwd"] = a.Ref(m.FwdFrom)
	}
	if m.Sticker != "" {
		out["sticker"] = m.Sticker
	}
	if m.VoiceExt != "" {
		out["voice"] = map[string]any{
			"url": "/api/gvoice/" + i(m.ID) + "." + m.VoiceExt,
			"dur": m.VoiceDur,
		}
	}
	if m.ReplyID > 0 {
		if rm, ok := msgsT.Get(m.ReplyID); ok && rm.ChatID == m.ChatID {
			out["reply"] = map[string]any{
				"id":   rm.ID,
				"nick": a.Ref(rm.UserID).Nick,
				"text": msgPreview(a, rm),
			}
		}
	}
	if m.AttType != "" && m.AttID > 0 {
		if d, ok := a.Describe(m.AttType, m.AttID); ok {
			out["attach"] = map[string]any{
				"type":  m.AttType,
				"id":    m.AttID,
				"title": d.Title,
				"text":  d.Text,
				"link":  d.Link,
				"image": d.Image,
			}
		}
	}
	if r := reactionsOf(m.ID, me); len(r) > 0 {
		out["reactions"] = r
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
	reactsT = store.NewTable[GroupChatReaction](a.DB, "gchat_reactions").
		Index("msg", func(x *GroupChatReaction) string { return i(x.IDMsg) }).
		Index("pair", func(x *GroupChatReaction) string { return store.Key(x.IDMsg, x.IDUser) })

	voiceDir := filepath.Join(a.DataDir, "gvoice")
	_ = os.MkdirAll(voiceDir, 0o755)

	a.OnUserDelete(func(uid int64) {
		membersT.DeleteBy("user", i(uid))
		msgsT.DeleteWhere(func(m *GroupChatMsg) bool { return m.UserID == uid })
		reactsT.DeleteWhere(func(r *GroupChatReaction) bool { return r.IDUser == uid })
	})

	// Список моих бесед (с поддержкой папок common|favorite|ignor|deleted для общего списка Почты)
	a.GET("/api/chats", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		wantFolder := c.Q("type")
		mems := membersT.By("user", i(me.ID))
		type item struct {
			chat     GroupChat
			lastTime int64
			lastText string
			lastNick string
			count    int
			role     string
			folder   string
		}
		var list []item
		for _, m := range mems {
			f := normFolder(m.Folder)
			if wantFolder != "" && f != wantFolder {
				continue
			}
			ch, ok := chatsT.Get(m.ChatID)
			if !ok {
				continue
			}
			it := item{chat: ch, lastTime: ch.Created, count: membersT.CountBy("chat", i(ch.ID)), role: m.Role, folder: f}
			ms := msgsT.By("chat", i(ch.ID))
			if len(ms) > 0 {
				best := ms[0]
				for _, x := range ms[1:] {
					if x.ID > best.ID {
						best = x
					}
				}
				it.lastTime = best.Time
				it.lastText = msgPreview(a, best)
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
				"folder":    it.folder,
				"last_time": it.lastTime,
				"last_text": it.lastText,
				"last_nick": it.lastNick,
			})
		}
		return c.JSON(map[string]any{"items": out})
	}, core.Auth)

	// Перемещение бесед между папками Почты (common | favorite | ignor | deleted)
	a.POST("/api/chats/move", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		var in struct {
			IDs  []int64 `json:"ids"`
			Type string  `json:"type"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		target := normFolder(in.Type)
		for _, cid := range in.IDs {
			if m, ok := memberOf(cid, me.ID); ok {
				membersT.Update(m.ID, func(x *GroupChatMember) { x.Folder = target })
			}
		}
		return c.OK()
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
		membersT.Insert(&GroupChatMember{ChatID: ch.ID, UserID: me.ID, Role: "owner", Folder: "common", JoinedAt: now})
		seen := map[int64]bool{me.ID: true}
		for _, uid := range in.MemberIDs {
			if uid <= 0 || seen[uid] || !a.T.Users.Has(uid) {
				continue
			}
			seen[uid] = true
			membersT.Insert(&GroupChatMember{ChatID: ch.ID, UserID: uid, Role: "member", Folder: "common", JoinedAt: now})
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
		if m, exists := memberOf(ch.ID, me.ID); !exists {
			membersT.Insert(&GroupChatMember{ChatID: ch.ID, UserID: me.ID, Role: "member", Folder: "common", JoinedAt: core.Now()})
		} else if normFolder(m.Folder) == "deleted" {
			membersT.Update(m.ID, func(x *GroupChatMember) { x.Folder = "common" })
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
			items = append(items, msgView(a, m, me.ID, ch.PinnedMsgID))
		}
		var pinned map[string]any
		if ch.PinnedMsgID > 0 {
			if pm, ok := msgsT.Get(ch.PinnedMsgID); ok && pm.ChatID == ch.ID {
				pinned = map[string]any{"id": pm.ID, "nick": a.Ref(pm.UserID).Nick, "text": msgPreview(a, pm)}
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

	// Поиск по истории беседы
	a.GET("/api/chats/{id}/search", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		ch, ok := chatsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		if _, ok := memberOf(ch.ID, me.ID); !ok && me.Level < 7 {
			return core.ErrForbidden()
		}
		q := strings.ToLower(strings.TrimSpace(c.Q("q")))
		if utf8.RuneCountInString(q) < 1 {
			return c.JSON(map[string]any{"items": []any{}})
		}
		var hits []GroupChatMsg
		for _, m := range msgsT.By("chat", i(ch.ID)) {
			if strings.Contains(strings.ToLower(m.Msg), q) {
				hits = append(hits, m)
			}
		}
		sort.Slice(hits, func(x, y int) bool { return hits[x].ID > hits[y].ID })
		if len(hits) > 30 {
			hits = hits[:30]
		}
		out := make([]map[string]any, 0, len(hits))
		for _, m := range hits {
			out = append(out, msgView(a, m, me.ID, ch.PinnedMsgID))
		}
		return c.JSON(map[string]any{"items": out})
	}, core.Auth)

	// Отправить сообщение в беседу (текст, вложения, стикеры, пересылка, ответ)
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
			AttType string `json:"att_type"`
			AttID   int64  `json:"att_id"`
			FwdFrom int64  `json:"fwd_from"`
			Sticker string `json:"sticker"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		txt := strings.TrimSpace(in.Msg)
		st := strings.TrimSpace(in.Sticker)
		if st != "" {
			if utf8.RuneCountInString(st) > 8 {
				return core.ErrBad("Некорректный стикер")
			}
			if vipStickers[st] && me.VIPUntil <= core.Now() {
				return core.ErrForbidden("Этот набор стикеров доступен только для VIP")
			}
		}
		hasAtt := in.AttType != "" && in.AttID > 0
		if hasAtt {
			if in.AttType != "foto" && in.AttType != "files" && in.AttType != "shorts" {
				return core.ErrBad("Неподдерживаемый тип вложения")
			}
			desc, ok := a.Describe(in.AttType, in.AttID)
			if !ok {
				return core.ErrNotFound("Вложение не найдено")
			}
			if in.AttType != "shorts" && desc.Owner != me.ID {
				return core.ErrForbidden("Можно прикреплять только свои файлы и фото")
			}
		}
		if n := utf8.RuneCountInString(txt); n < 1 && !hasAtt && st == "" {
			return core.ErrBad("Введите сообщение или прикрепите вложение")
		} else if n > 1024 {
			return core.ErrBad("Сообщение слишком длинное (максимум 1024 символа)")
		}
		clean := ""
		if txt != "" {
			clean = a.Text.Clean(txt)
		}
		m := GroupChatMsg{
			ChatID: ch.ID, UserID: me.ID, Msg: clean, ReplyID: in.ReplyID,
			AttType: in.AttType, AttID: in.AttID, FwdFrom: in.FwdFrom, Sticker: st, Time: core.Now(),
		}
		msgsT.Insert(&m)
		notified := map[int64]bool{me.ID: true}
		for _, sub := range mentionRe.FindAllStringSubmatch(txt, 5) {
			if target, ok := a.UserByNick(sub[1]); ok && !notified[target.ID] {
				if _, isM := memberOf(ch.ID, target.ID); isM {
					notified[target.ID] = true
					a.Notify(me.ID, target.ID, "mention", ch.ID, true)
				}
			}
		}
		for _, mem := range membersT.By("chat", i(ch.ID)) {
			a.Hub.ToUser(mem.UserID, "gchat_msg", msgView(a, m, mem.UserID, ch.PinnedMsgID))
		}
		return c.JSON(msgView(a, m, me.ID, ch.PinnedMsgID))
	}, core.Auth)

	// Индикатор «печатает…» в беседе
	a.POST("/api/chats/{id}/typing", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		cid := c.PathInt("id")
		if _, ok := memberOf(cid, me.ID); !ok {
			return c.OK()
		}
		for _, mem := range membersT.By("chat", i(cid)) {
			if mem.UserID != me.ID {
				a.Hub.ToUser(mem.UserID, "gchat_typing", map[string]any{"chat": cid, "nick": me.Nick})
			}
		}
		return c.OK()
	}, core.Auth)

	// Голосовые сообщения в беседе
	a.POST("/api/chats/{id}/voice", func(c *core.Ctx) error {
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
			return core.ErrForbidden()
		}
		if err := c.R.ParseMultipartForm(12 << 20); err != nil {
			return core.ErrBad("Не удалось прочитать аудиофайл")
		}
		file, hdr, err := c.R.FormFile("file")
		if err != nil {
			return core.ErrBad("Аудиофайл не передан")
		}
		defer file.Close()
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(hdr.Filename), "."))
		switch ext {
		case "webm", "ogg", "mp4", "m4a", "mp3", "wav":
		default:
			ext = "webm"
		}
		dur, _ := strconv.Atoi(c.R.FormValue("dur"))
		if dur < 1 {
			dur = 1
		}
		if dur > 600 {
			dur = 600
		}
		m := GroupChatMsg{ChatID: ch.ID, UserID: me.ID, Time: core.Now(), VoiceExt: ext, VoiceDur: dur}
		msgsT.Insert(&m)
		dstPath := filepath.Join(voiceDir, i(m.ID)+"."+ext)
		dst, err := os.Create(dstPath)
		if err != nil {
			msgsT.Delete(m.ID)
			return core.ErrBad("Ошибка сохранения аудио")
		}
		n, err := io.Copy(dst, io.LimitReader(file, 10<<20))
		_ = dst.Close()
		if err != nil || n == 0 {
			_ = os.Remove(dstPath)
			msgsT.Delete(m.ID)
			return core.ErrBad("Пустой аудиофайл")
		}
		for _, mem := range membersT.By("chat", i(ch.ID)) {
			a.Hub.ToUser(mem.UserID, "gchat_msg", msgView(a, m, mem.UserID, ch.PinnedMsgID))
		}
		return c.JSON(msgView(a, m, me.ID, ch.PinnedMsgID))
	}, core.Auth)

	a.GET("/api/gvoice/{file}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		name := filepath.Base(c.Path("file"))
		parts := strings.SplitN(name, ".", 2)
		if len(parts) != 2 {
			return core.ErrNotFound()
		}
		id, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return core.ErrNotFound()
		}
		m, ok := msgsT.Get(id)
		if !ok || m.VoiceExt == "" {
			return core.ErrNotFound()
		}
		if _, isM := memberOf(m.ChatID, me.ID); !isM && me.Level < 7 {
			return core.ErrForbidden()
		}
		http.ServeFile(c.W, c.R, filepath.Join(voiceDir, i(m.ID)+"."+m.VoiceExt))
		return nil
	}, core.Auth)

	// Редактирование сообщения в беседе
	a.PUT("/api/chats/msg/{mid}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		m, ok := msgsT.Get(c.PathInt("mid"))
		if !ok || m.UserID != me.ID {
			return core.ErrNotFound("Сообщение не найдено")
		}
		if m.VoiceExt != "" || m.Sticker != "" {
			return core.ErrBad("Это сообщение нельзя редактировать")
		}
		var in struct {
			Msg string `json:"msg"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		txt := strings.TrimSpace(in.Msg)
		if n := utf8.RuneCountInString(txt); n < 1 || n > 1024 {
			return core.ErrBad("Длина сообщения должна быть от 1 до 1024 символов")
		}
		clean := a.Text.Clean(txt)
		now := core.Now()
		msgsT.Update(m.ID, func(x *GroupChatMsg) { x.Msg = clean; x.EditedAt = now })
		ch, _ := chatsT.Get(m.ChatID)
		upd, _ := msgsT.Get(m.ID)
		for _, mem := range membersT.By("chat", i(m.ChatID)) {
			a.Hub.ToUser(mem.UserID, "gchat_edit", map[string]any{"chat": m.ChatID, "id": m.ID, "msg": a.Text.Render(clean), "raw": clean, "edited_at": now})
		}
		return c.JSON(msgView(a, upd, me.ID, ch.PinnedMsgID))
	}, core.Auth)

	// Реакция-эмодзи на сообщение в беседе
	a.POST("/api/chats/msg/{mid}/react", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		m, ok := msgsT.Get(c.PathInt("mid"))
		if !ok {
			return core.ErrNotFound()
		}
		if _, isM := memberOf(m.ChatID, me.ID); !isM {
			return core.ErrForbidden()
		}
		var in struct {
			Emoji string `json:"emoji"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if !allowedReacts[in.Emoji] {
			return core.ErrBad("Неподдерживаемая реакция")
		}
		if old, ok := reactsT.OneBy("pair", store.Key(m.ID, me.ID)); ok {
			if old.Emoji == in.Emoji {
				reactsT.Delete(old.ID)
			} else {
				reactsT.Update(old.ID, func(x *GroupChatReaction) { x.Emoji = in.Emoji })
			}
		} else {
			reactsT.Insert(&GroupChatReaction{IDMsg: m.ID, IDUser: me.ID, Emoji: in.Emoji})
		}
		for _, mem := range membersT.By("chat", i(m.ChatID)) {
			a.Hub.ToUser(mem.UserID, "gchat_react", map[string]any{"chat": m.ChatID, "id": m.ID, "reactions": reactionsOf(m.ID, mem.UserID)})
		}
		return c.OK("reactions", reactionsOf(m.ID, me.ID))
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
		if m.VoiceExt != "" {
			_ = os.Remove(filepath.Join(voiceDir, i(m.ID)+"."+m.VoiceExt))
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
		membersT.Insert(&GroupChatMember{ChatID: ch.ID, UserID: uid, Role: "member", Folder: "common", JoinedAt: core.Now()})
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
