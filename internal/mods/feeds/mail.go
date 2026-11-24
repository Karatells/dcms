package feeds

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

// Mail — личное сообщение (таблица mail). Unlink — id пользователя, скрывшего переписку у себя.
type Mail struct {
	store.Base
	IDUser   int64  `json:"id_user"`
	IDKont   int64  `json:"id_kont"`
	Time     int64  `json:"time"`
	Msg      string `json:"msg"`
	Read     bool   `json:"read"`
	Unlink   int64  `json:"unlink"`
	ReplyID  int64  `json:"reply_id"`
	AttType  string `json:"att_type"` // foto | files | shorts
	AttID    int64  `json:"att_id"`
	Pinned   bool   `json:"pinned"`
	EditedAt int64  `json:"edited_at,omitempty"`
	FwdFrom  int64  `json:"fwd_from,omitempty"`
	VoiceExt string `json:"voice_ext,omitempty"`
	VoiceDur int    `json:"voice_dur,omitempty"`
	Sticker  string `json:"sticker,omitempty"`
}

type stickerPack struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	VIP      bool     `json:"vip"`
	Stickers []string `json:"stickers"`
}

var builtInStickerPacks = []stickerPack{
	{ID: "emotions", Title: "Эмоции", VIP: false, Stickers: []string{"😀", "😂", "🥰", "😎", "🤔", "😴", "🥳", "🤯", "😭", "😡", "🥺", "🤩"}},
	{ID: "reactions", Title: "Реакции", VIP: false, Stickers: []string{"👍", "👎", "🔥", "💯", "❤️", "💔", "👏", "🙌", "🤝", "💪", "🎉", "🚀"}},
	{ID: "vip_space", Title: "VIP Космос", VIP: true, Stickers: []string{"👑", "💎", "🛸", "🌌", "🦄", "🐉", "⚡", "🔮", "🏆", "🎭", "🌠", "🦚"}},
}

func isVIPSticker(st string) bool {
	for _, p := range builtInStickerPacks {
		if !p.VIP {
			continue
		}
		for _, s := range p.Stickers {
			if s == st {
				return true
			}
		}
	}
	return false
}

// Kont — контакт пользователя (users_konts): type common|favorite|ignor|deleted.
type Kont struct {
	store.Base
	IDUser int64  `json:"id_user"`
	IDKont int64  `json:"id_kont"`
	Time   int64  `json:"time"`
	Type   string `json:"type"`
	Name   string `json:"name"`
}

// MailReaction — реакция-эмодзи на личное сообщение.
type MailReaction struct {
	store.Base
	IDMsg  int64  `json:"id_msg"`
	IDUser int64  `json:"id_user"`
	Emoji  string `json:"emoji"`
}

var (
	mails      *store.Table[Mail, *Mail]
	konts      *store.Table[Kont, *Kont]
	mailReacts *store.Table[MailReaction, *MailReaction]
)

var allowedReacts = map[string]bool{"👍": true, "❤️": true, "😂": true, "🔥": true, "😱": true, "👎": true}

func reactionsOf(msgID, me int64) []map[string]any {
	if mailReacts == nil {
		return nil
	}
	list := mailReacts.By("msg", store.I(msgID))
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

const deletedTTL = 30 * 86400

func pairKey(a, b int64) string {
	if a > b {
		a, b = b, a
	}
	return store.Key(a, b)
}

func kontOf(user, other int64) (Kont, bool) { return konts.OneBy("pair", store.Key(user, other)) }

// unreadFrom — число непрочитанных сообщений от other для user.
func unreadFrom(user, other int64) int {
	n := 0
	for _, m := range mails.By("dialog", pairKey(user, other)) {
		if m.IDUser == other && m.IDKont == user && !m.Read && m.Unlink != user {
			n++
		}
	}
	return n
}

var extraMailUnread []func(uid int64) int

// RegisterExtraUnread позволяет другим модулям (например, личным сообщениям сообществ) добавлять непрочитанные в счётчик почты.
func RegisterExtraUnread(fn func(uid int64) int) { extraMailUnread = append(extraMailUnread, fn) }

func unreadTotal(user int64) int {
	n := 0
	mails.Scan(func(m *Mail) bool {
		if m.IDKont == user && !m.Read && m.Unlink != user {
			if k, ok := kontOf(user, m.IDUser); !ok || (k.Type != "ignor" && k.Type != "deleted") {
				n++
			}
		}
		return true
	})
	for _, fn := range extraMailUnread {
		n += fn(user)
	}
	return n
}

func lastMsg(user, other int64) (Mail, bool) {
	var best Mail
	found := false
	for _, m := range mails.By("dialog", pairKey(user, other)) {
		if m.Unlink == user {
			continue
		}
		if !found || m.ID > best.ID {
			best, found = m, true
		}
	}
	return best, found
}

func canWriteTo(a *core.App, from *model.User, to *model.User) error {
	if from.ID == to.ID {
		return core.ErrBad("Нельзя писать самому себе")
	}
	if from.Level >= 7 {
		return nil
	}
	switch a.UserSettings(to.ID).PrivatMail {
	case 0:
		return core.ErrForbidden("Пользователь закрыл личные сообщения")
	case 2:
		if !a.IsFriend(from.ID, to.ID) {
			return core.ErrForbidden("Пользователь принимает сообщения только от друзей")
		}
	}
	if k, ok := kontOf(to.ID, from.ID); ok && k.Type == "ignor" {
		return core.ErrForbidden("Пользователь добавил вас в игнор-лист")
	}
	return nil
}

type sendOpts struct {
	ReplyID int64
	AttType string
	AttID   int64
	FwdFrom int64
	Sticker string
}

// ListDialogs возвращает список диалогов пользователя (для публичного API v1).
func ListDialogs(a *core.App, uid int64, offset, per int) ([]map[string]any, int) {
	type dlg struct {
		peer     int64
		lastText string
		lastTime int64
		unread   int
		favorite bool
	}
	var list []dlg
	for _, k := range konts.By("user", store.I(uid)) {
		if k.Type == "deleted" || k.Type == "ignor" {
			continue
		}
		lm, has := lastMsg(uid, k.IDKont)
		lt := k.Time
		txt := ""
		if has {
			lt = lm.Time
			txt = lm.Msg
		}
		list = append(list, dlg{
			peer:     k.IDKont,
			lastText: txt,
			lastTime: lt,
			unread:   unreadFrom(uid, k.IDKont),
			favorite: k.Type == "favorite",
		})
	}
	// Сортируем по времени последнего сообщения по убыванию
	for i := 0; i < len(list); i++ {
		for j := i + 1; j < len(list); j++ {
			if list[j].lastTime > list[i].lastTime {
				list[i], list[j] = list[j], list[i]
			}
		}
	}
	total := len(list)
	if offset < 0 {
		offset = 0
	}
	end := offset + per
	if end > total {
		end = total
	}
	var sl []dlg
	if offset < total {
		sl = list[offset:end]
	}
	items := make([]map[string]any, 0, len(sl))
	for _, d := range sl {
		items = append(items, map[string]any{
			"peer":         a.Ref(d.peer),
			"last_message": d.lastText,
			"last_time":    d.lastTime,
			"unread":       d.unread,
			"favorite":     d.favorite,
		})
	}
	return items, total
}

// ListDialogMessages возвращает сообщения диалога между uid и peerID (для публичного API v1).
func ListDialogMessages(a *core.App, uid, peerID int64, offset, per int) ([]map[string]any, int) {
	raw := mails.By("dialog", pairKey(uid, peerID))
	var vis []Mail
	for _, m := range raw {
		if m.Unlink == uid {
			continue
		}
		vis = append(vis, m)
	}
	for i := 0; i < len(vis); i++ {
		for j := i + 1; j < len(vis); j++ {
			if vis[j].ID > vis[i].ID {
				vis[i], vis[j] = vis[j], vis[i]
			}
		}
	}
	total := len(vis)
	if offset < 0 {
		offset = 0
	}
	end := offset + per
	if end > total {
		end = total
	}
	var sl []Mail
	if offset < total {
		sl = vis[offset:end]
	}
	items := make([]map[string]any, 0, len(sl))
	for _, m := range sl {
		items = append(items, map[string]any{
			"id":       m.ID,
			"from":     a.Ref(m.IDUser),
			"to":       a.Ref(m.IDKont),
			"text":     m.Msg,
			"html":     a.Text.Render(m.Msg),
			"read":     m.Read,
			"pinned":   m.Pinned,
			"reply_id": m.ReplyID,
			"time":     m.Time,
		})
	}
	return items, total
}

// SendMessage отправляет личное сообщение от from к to (для публичного API v1).
func SendMessage(a *core.App, from, to *model.User, msg string, replyID int64) (Mail, error) {
	return send(a, from, to, msg, sendOpts{ReplyID: replyID})
}

func send(a *core.App, from, to *model.User, msg string, opts ...sendOpts) (Mail, error) {
	var o sendOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	msg = strings.TrimSpace(msg)
	st := strings.TrimSpace(o.Sticker)
	if st != "" {
		if utf8.RuneCountInString(st) > 8 {
			return Mail{}, core.ErrBad("Некорректный стикер")
		}
		if isVIPSticker(st) && from.VIPUntil <= core.Now() {
			return Mail{}, core.ErrForbidden("Этот набор стикеров доступен только для VIP")
		}
	}
	hasAtt := o.AttType != "" && o.AttID > 0
	if hasAtt {
		if o.AttType != "foto" && o.AttType != "files" && o.AttType != "shorts" {
			return Mail{}, core.ErrBad("Неподдерживаемый тип вложения")
		}
		desc, ok := a.Describe(o.AttType, o.AttID)
		if !ok {
			return Mail{}, core.ErrNotFound("Вложение не найдено")
		}
		if o.AttType != "shorts" && desc.Owner != from.ID {
			return Mail{}, core.ErrForbidden("Можно прикреплять только свои файлы и фото")
		}
	}
	if n := utf8.RuneCountInString(msg); n < 1 && !hasAtt && st == "" {
		return Mail{}, core.ErrBad("Введите сообщение или прикрепите вложение")
	} else if n > 1024 {
		return Mail{}, core.ErrBad("Сообщение слишком длинное (максимум 1024 символа)")
	}
	if err := canWriteTo(a, from, to); err != nil {
		return Mail{}, err
	}
	if _, ok := a.ActiveBan(from.ID, "all"); ok {
		return Mail{}, core.ErrForbidden("Вы заблокированы")
	}
	if o.ReplyID > 0 {
		rm, ok := mails.Get(o.ReplyID)
		if !ok || pairKey(rm.IDUser, rm.IDKont) != pairKey(from.ID, to.ID) {
			o.ReplyID = 0
		}
	}
	now := core.Now()
	cleanMsg := ""
	if msg != "" {
		cleanMsg = a.Text.Clean(msg)
	}
	m := Mail{IDUser: from.ID, IDKont: to.ID, Time: now, Msg: cleanMsg, ReplyID: o.ReplyID, AttType: o.AttType, AttID: o.AttID, FwdFrom: o.FwdFrom, Sticker: st}
	mails.Insert(&m)
	// отправитель получает контакт, у получателя обновляется время существующего
	if k, ok := kontOf(from.ID, to.ID); ok {
		konts.Update(k.ID, func(x *Kont) {
			x.Time = now
			if x.Type == "deleted" {
				x.Type = "common"
			}
		})
	} else {
		konts.Insert(&Kont{IDUser: from.ID, IDKont: to.ID, Time: now, Type: "common"})
	}
	if k, ok := kontOf(to.ID, from.ID); ok {
		konts.Update(k.ID, func(x *Kont) {
			x.Time = now
			if x.Type == "deleted" {
				x.Type = "common"
			}
		})
	} else {
		konts.Insert(&Kont{IDUser: to.ID, IDKont: from.ID, Time: now, Type: "common"})
	}
	// если собеседник «скрыл» переписку — возвращаем её
	for _, x := range mails.By("dialog", pairKey(from.ID, to.ID)) {
		if x.Unlink == to.ID {
			mails.Update(x.ID, func(y *Mail) { y.Unlink = 0 })
		}
	}
	preview := a.Text.Plain(m.Msg, 120)
	if preview == "" && st != "" {
		preview = st + " Стикер"
	} else if preview == "" && hasAtt {
		preview = map[string]string{"foto": "📷 Фотография", "files": "📎 Файл", "shorts": "🎬 Клип"}[o.AttType]
	}
	a.Hub.ToUser(to.ID, "message", map[string]any{"from": a.Ref(from.ID), "id": m.ID, "text": preview})
	a.Hub.ToUser(to.ID, "counters", map[string]any{"mail": true})
	return m, nil
}

func msgPreview(a *core.App, m Mail) string {
	if s := a.Text.Plain(m.Msg, 90); s != "" {
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

func msgView(a *core.App, m Mail, me int64) map[string]any {
	out := map[string]any{
		"id":        m.ID,
		"mine":      m.IDUser == me,
		"time":      m.Time,
		"msg":       a.Text.Render(m.Msg),
		"raw":       m.Msg,
		"read":      m.Read,
		"pinned":    m.Pinned,
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
			"url": "/api/voice/" + store.I(m.ID) + "." + m.VoiceExt,
			"dur": m.VoiceDur,
		}
	}
	if m.ReplyID > 0 {
		if rm, ok := mails.Get(m.ReplyID); ok && rm.Unlink != me {
			out["reply"] = map[string]any{
				"id":   rm.ID,
				"nick": a.Ref(rm.IDUser).Nick,
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

func registerMail(a *core.App) {
	mails = store.NewTable[Mail](a.DB, "mail").Index("dialog", func(m *Mail) string { return pairKey(m.IDUser, m.IDKont) })
	konts = store.NewTable[Kont](a.DB, "users_konts").
		Index("user", func(k *Kont) string { return store.I(k.IDUser) }).
		Index("pair", func(k *Kont) string { return store.Key(k.IDUser, k.IDKont) })
	mailReacts = store.NewTable[MailReaction](a.DB, "mail_reactions").
		Index("msg", func(r *MailReaction) string { return store.I(r.IDMsg) }).
		Index("pair", func(r *MailReaction) string { return store.Key(r.IDMsg, r.IDUser) })

	a.OnUserDelete(func(uid int64) {
		mails.DeleteWhere(func(m *Mail) bool { return m.IDUser == uid || m.IDKont == uid })
		konts.DeleteWhere(func(k *Kont) bool { return k.IDUser == uid || k.IDKont == uid })
		a.T.Notifications.DeleteWhere(func(n *model.Notification) bool { return n.IDUser == uid || n.Avtor == uid })
		a.T.Tape.DeleteWhere(func(n *model.Tape) bool { return n.IDUser == uid || n.Avtor == uid })
		a.T.Discussions.DeleteWhere(func(n *model.Discussions) bool { return n.IDUser == uid })
	})
	// очистка удалённых контактов старше 30 дней
	a.Cron("konts.cleanup", time.Hour, func(a *core.App) {
		now := core.Now()
		for _, k := range konts.Find(func(k *Kont) bool { return k.Type == "deleted" && k.Time < now-deletedTTL }) {
			konts.Delete(k.ID)
			if _, back := kontOf(k.IDKont, k.IDUser); !back {
				mails.DeleteWhere(func(m *Mail) bool {
					return (m.IDUser == k.IDUser && m.IDKont == k.IDKont) || (m.IDUser == k.IDKont && m.IDKont == k.IDUser)
				})
			}
		}
	})
	a.Counter("mail", func(a *core.App, u *model.User) string { return strconv.Itoa(unreadTotal(u.ID)) })

	// ---- диалог
	a.GET("/api/mail/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		other, ok := a.GetUser(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Пользователь не найден")
		}
		key := pairKey(me.ID, other.ID)
		var pinned map[string]any
		markedRead := false
		for _, m := range mails.By("dialog", key) {
			if m.IDUser == other.ID && m.IDKont == me.ID && !m.Read {
				mails.Update(m.ID, func(x *Mail) { x.Read = true })
				markedRead = true
			}
			if m.Pinned && m.Unlink != me.ID {
				pinned = map[string]any{"id": m.ID, "nick": a.Ref(m.IDUser).Nick, "text": msgPreview(a, m)}
			}
		}
		if markedRead {
			a.Hub.ToUser(other.ID, "mail_read", map[string]any{"by": me.ID})
		}
		p := c.Page()
		list, total := mails.Select(store.Opts[Mail]{Index: "dialog", Key: key, Where: func(m *Mail) bool { return m.Unlink != me.ID }, Desc: true, Offset: p.Offset, Limit: p.Per})
		items := make([]map[string]any, 0, len(list))
		for _, m := range list {
			items = append(items, msgView(a, m, me.ID))
		}
		k, has := kontOf(me.ID, other.ID)
		out := map[string]any{"items": items, "total": total, "page": p.Page, "per": p.Per, "pages": max(1, (total+p.Per-1)/p.Per),
			"user": a.Ref(other.ID), "contact": has, "contact_type": k.Type, "can_write": canWriteTo(a, me, &other) == nil, "pinned": pinned}
		a.Hub.ToUser(me.ID, "counters", map[string]any{"mail": true})
		return c.JSON(out)
	}, core.Auth)

	a.POST("/api/mail/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		other, ok := a.GetUser(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Пользователь не найден")
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
		m, err := send(a, me, &other, in.Msg, sendOpts{ReplyID: in.ReplyID, AttType: in.AttType, AttID: in.AttID, FwdFrom: in.FwdFrom, Sticker: in.Sticker})
		if err != nil {
			return err
		}
		return c.JSON(msgView(a, m, me.ID))
	}, core.Auth)

	// редактирование своего сообщения
	a.PUT("/api/mail/msg/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		m, ok := mails.Get(c.PathInt("id"))
		if !ok || m.IDUser != me.ID {
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
		mails.Update(m.ID, func(x *Mail) {
			x.Msg = clean
			x.EditedAt = now
		})
		upd, _ := mails.Get(m.ID)
		a.Hub.ToUser(m.IDKont, "mail_edit", map[string]any{"id": m.ID, "msg": a.Text.Render(clean), "raw": clean, "edited_at": now})
		return c.JSON(msgView(a, upd, me.ID))
	}, core.Auth)

	// поиск по истории диалога
	a.GET("/api/mail/{id}/search", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		otherID := c.PathInt("id")
		q := strings.ToLower(strings.TrimSpace(c.Q("q")))
		if utf8.RuneCountInString(q) < 1 {
			return c.JSON(map[string]any{"items": []any{}})
		}
		all := mails.By("dialog", pairKey(me.ID, otherID))
		var hits []Mail
		for _, m := range all {
			if m.Unlink == me.ID {
				continue
			}
			if strings.Contains(strings.ToLower(m.Msg), q) {
				hits = append(hits, m)
			}
		}
		// новые сверху, максимум 30
		for i := 0; i < len(hits); i++ {
			for j := i + 1; j < len(hits); j++ {
				if hits[j].ID > hits[i].ID {
					hits[i], hits[j] = hits[j], hits[i]
				}
			}
		}
		if len(hits) > 30 {
			hits = hits[:30]
		}
		out := make([]map[string]any, 0, len(hits))
		for _, m := range hits {
			out = append(out, msgView(a, m, me.ID))
		}
		return c.JSON(map[string]any{"items": out})
	}, core.Auth)

	// голосовые сообщения
	voiceDir := filepath.Join(a.DataDir, "voice")
	_ = os.MkdirAll(voiceDir, 0o755)
	a.POST("/api/mail/{id}/voice", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		other, ok := a.GetUser(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Пользователь не найден")
		}
		if err := canWriteTo(a, me, &other); err != nil {
			return err
		}
		if _, ok := a.ActiveBan(me.ID, "all"); ok {
			return core.ErrForbidden("Вы заблокированы")
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
		now := core.Now()
		m := Mail{IDUser: me.ID, IDKont: other.ID, Time: now, VoiceExt: ext, VoiceDur: dur}
		mails.Insert(&m)
		dstPath := filepath.Join(voiceDir, store.I(m.ID)+"."+ext)
		dst, err := os.Create(dstPath)
		if err != nil {
			mails.Delete(m.ID)
			return core.ErrBad("Ошибка сохранения аудио")
		}
		n, err := io.Copy(dst, io.LimitReader(file, 10<<20))
		_ = dst.Close()
		if err != nil || n == 0 {
			_ = os.Remove(dstPath)
			mails.Delete(m.ID)
			return core.ErrBad("Пустой аудиофайл")
		}
		if k, ok := kontOf(me.ID, other.ID); ok {
			konts.Update(k.ID, func(x *Kont) { x.Time = now; if x.Type == "deleted" { x.Type = "common" } })
		} else {
			konts.Insert(&Kont{IDUser: me.ID, IDKont: other.ID, Time: now, Type: "common"})
		}
		if k, ok := kontOf(other.ID, me.ID); ok {
			konts.Update(k.ID, func(x *Kont) { x.Time = now; if x.Type == "deleted" { x.Type = "common" } })
		} else {
			konts.Insert(&Kont{IDUser: other.ID, IDKont: me.ID, Time: now, Type: "common"})
		}
		a.Hub.ToUser(other.ID, "message", map[string]any{"from": a.Ref(me.ID), "id": m.ID, "text": "🎤 Голосовое сообщение"})
		a.Hub.ToUser(other.ID, "counters", map[string]any{"mail": true})
		return c.JSON(msgView(a, m, me.ID))
	}, core.Auth)

	a.GET("/api/voice/{file}", func(c *core.Ctx) error {
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
		m, ok := mails.Get(id)
		if !ok || m.VoiceExt == "" || (m.IDUser != me.ID && m.IDKont != me.ID && me.Level < 7) {
			return core.ErrForbidden()
		}
		p := filepath.Join(voiceDir, store.I(m.ID)+"."+m.VoiceExt)
		http.ServeFile(c.W, c.R, p)
		return nil
	}, core.Auth)

	// наборы стикеров
	a.GET("/api/stickers", func(c *core.Ctx) error {
		me := c.User()
		isVIP := me != nil && me.VIPUntil > core.Now()
		return c.JSON(map[string]any{"packs": builtInStickerPacks, "is_vip": isVIP})
	})

	// индикатор «печатает…»
	a.POST("/api/mail/{id}/typing", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		otherID := c.PathInt("id")
		if otherID > 0 && otherID != me.ID {
			a.Hub.ToUser(otherID, "typing", map[string]any{"from": me.ID})
		}
		return c.OK()
	}, core.Auth)

	// реакция-эмодзи на сообщение
	a.POST("/api/mail/msg/{id}/react", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		m, ok := mails.Get(c.PathInt("id"))
		if !ok || (m.IDUser != me.ID && m.IDKont != me.ID) {
			return core.ErrNotFound()
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
		if old, ok := mailReacts.OneBy("pair", store.Key(m.ID, me.ID)); ok {
			if old.Emoji == in.Emoji {
				mailReacts.Delete(old.ID)
			} else {
				mailReacts.Update(old.ID, func(x *MailReaction) { x.Emoji = in.Emoji })
			}
		} else {
			mailReacts.Insert(&MailReaction{IDMsg: m.ID, IDUser: me.ID, Emoji: in.Emoji})
		}
		otherID := m.IDKont
		if m.IDKont == me.ID {
			otherID = m.IDUser
		}
		myReacts := reactionsOf(m.ID, me.ID)
		a.Hub.ToUser(otherID, "mail_react", map[string]any{"id": m.ID, "reactions": reactionsOf(m.ID, otherID)})
		return c.OK("reactions", myReacts)
	}, core.Auth)

	// закрепить / открепить сообщение в диалоге
	a.POST("/api/mail/msg/{id}/pin", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		m, ok := mails.Get(c.PathInt("id"))
		if !ok || (m.IDUser != me.ID && m.IDKont != me.ID) {
			return core.ErrNotFound()
		}
		want := !m.Pinned
		key := pairKey(m.IDUser, m.IDKont)
		if want {
			for _, x := range mails.By("dialog", key) {
				if x.Pinned && x.ID != m.ID {
					mails.Update(x.ID, func(y *Mail) { y.Pinned = false })
				}
			}
		}
		mails.Update(m.ID, func(x *Mail) { x.Pinned = want })
		return c.OK("pinned", want)
	}, core.Auth)

	// отправка по нику (mails.php)
	a.POST("/api/mail/send", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		var in struct{ To, Msg string }
		if err := c.Bind(&in); err != nil {
			return err
		}
		to, ok := a.UserByNick(strings.TrimSpace(in.To))
		if !ok {
			return core.ErrBad("Пользователь с таким логином не найден")
		}
		if _, err := send(a, me, &to, in.Msg); err != nil {
			return err
		}
		return c.OK("id", to.ID)
	}, core.Auth)

	// скрыть всю переписку у себя
	a.DELETE("/api/mail/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		other := c.PathInt("id")
		hideDialog(me.ID, other)
		return c.OK()
	}, core.Auth)

	// удалить сообщение у себя
	a.DELETE("/api/mail/msg/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		m, ok := mails.Get(c.PathInt("id"))
		if !ok || (m.IDUser != me.ID && m.IDKont != me.ID) {
			return core.ErrNotFound()
		}
		if m.Unlink != 0 && m.Unlink != me.ID {
			mails.Delete(m.ID)
		} else {
			mails.Update(m.ID, func(x *Mail) { x.Unlink = me.ID })
		}
		return c.OK()
	}, core.Auth)

	a.POST("/api/mail/msg/{id}/spam", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		m, ok := mails.Get(c.PathInt("id"))
		if !ok || m.IDKont != me.ID {
			return core.ErrNotFound()
		}
		var in struct {
			Msg   string `json:"msg"`
			Types int64  `json:"types"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if utf8.RuneCountInString(in.Msg) < 3 {
			return core.ErrBad("Укажите причину")
		}
		if a.T.Spam.Count(func(s *model.Spamus) bool { return s.Razdel == "mail" && s.IDPost == m.ID && s.IDSpam == me.ID }) > 0 {
			return core.ErrBad("Жалоба уже отправлена")
		}
		a.ReportSpam(me.ID, m.IDUser, "mail", m.ID, 0, m.Msg, in.Msg)
		return c.OK()
	}, core.Auth)

	// ---- новые сообщения от тех, кого нет в контактах (new_mess.php)
	a.GET("/api/mail/new", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		type agg struct {
			n    int
			last int64
			id   int64
		}
		by := map[int64]*agg{}
		mails.Scan(func(m *Mail) bool {
			if m.IDKont == me.ID && !m.Read && m.Unlink != me.ID {
				if k, ok := kontOf(me.ID, m.IDUser); ok && k.Type != "common" {
					return true
				}
				g := by[m.IDUser]
				if g == nil {
					g = &agg{}
					by[m.IDUser] = g
				}
				g.n++
				if m.Time >= g.last {
					g.last, g.id = m.Time, m.ID
				}
			}
			return true
		})
		items := []map[string]any{}
		for uid, g := range by {
			m, _ := mails.Get(g.id)
			items = append(items, map[string]any{"user": a.Ref(uid), "count": g.n, "time": g.last, "text": a.Text.Plain(m.Msg, 100)})
		}
		sortByTime(items)
		return c.JSON(map[string]any{"items": items})
	}, core.Auth)

	// ---- контакты (konts.php)
	a.GET("/api/konts", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		typ := c.Q("type")
		if typ != "favorite" && typ != "ignor" && typ != "deleted" {
			typ = "common"
		}
		counts := map[string]int{}
		var rows []Kont
		for _, k := range konts.By("user", store.I(me.ID)) {
			counts[k.Type]++
			if k.Type == typ {
				rows = append(rows, k)
			}
		}
		type row struct {
			k   Kont
			new int
		}
		rs := make([]row, 0, len(rows))
		for _, k := range rows {
			rs = append(rs, row{k, unreadFrom(me.ID, k.IDKont)})
		}
		for i := 1; i < len(rs); i++ { // новые сверху, затем по времени
			for j := i; j > 0 && (rs[j].new > 0 && rs[j-1].new == 0 || (rs[j].new > 0) == (rs[j-1].new > 0) && rs[j].k.Time > rs[j-1].k.Time); j-- {
				rs[j], rs[j-1] = rs[j-1], rs[j]
			}
		}
		p := c.Page()
		items := []map[string]any{}
		for i := p.Offset; i < min(p.Offset+p.Per, len(rs)); i++ {
			r := rs[i]
			it := map[string]any{"user": a.Ref(r.k.IDKont), "new": r.new, "time": r.k.Time, "name": r.k.Name}
			if m, ok := lastMsg(me.ID, r.k.IDKont); ok {
				it["last"] = map[string]any{"text": a.Text.Plain(m.Msg, 80), "time": m.Time, "mine": m.IDUser == me.ID}
			}
			if typ == "deleted" {
				it["expires"] = r.k.Time + deletedTTL
			}
			items = append(items, it)
		}
		return c.JSON(map[string]any{"items": items, "total": len(rs), "page": p.Page, "per": p.Per, "pages": max(1, (len(rs)+p.Per-1)/p.Per), "counts": counts, "type": typ})
	}, core.Auth)

	a.POST("/api/konts/{id}", func(c *core.Ctx) error { // добавить в контакты
		me, err := c.Need()
		if err != nil {
			return err
		}
		other, ok := a.GetUser(c.PathInt("id"))
		if !ok || other.ID == me.ID {
			return core.ErrBad("Некорректный пользователь")
		}
		if k, ok := kontOf(me.ID, other.ID); ok {
			if k.Type == "deleted" {
				konts.Update(k.ID, func(x *Kont) { x.Type = "common"; x.Time = core.Now() })
			}
		} else {
			konts.Insert(&Kont{IDUser: me.ID, IDKont: other.ID, Time: core.Now(), Type: "common"})
		}
		return c.OK()
	}, core.Auth)

	a.DELETE("/api/konts/{id}", func(c *core.Ctx) error { // убрать (в «удалённые» на 30 дней)
		me, err := c.Need()
		if err != nil {
			return err
		}
		if k, ok := kontOf(me.ID, c.PathInt("id")); ok {
			konts.Update(k.ID, func(x *Kont) { x.Type = "deleted"; x.Time = core.Now() })
		}
		return c.OK()
	}, core.Auth)

	a.POST("/api/konts/move", func(c *core.Ctx) error { // массовое перемещение
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
		if in.Type != "common" && in.Type != "favorite" && in.Type != "ignor" && in.Type != "deleted" {
			return core.ErrBad("Неизвестный тип")
		}
		for _, id := range in.IDs {
			if k, ok := kontOf(me.ID, id); ok {
				konts.Update(k.ID, func(x *Kont) { x.Type = in.Type; x.Time = core.Now() })
			}
		}
		return c.OK()
	}, core.Auth)

	a.PUT("/api/konts/{id}/name", func(c *core.Ctx) error { // своё имя для контакта
		me, err := c.Need()
		if err != nil {
			return err
		}
		var in struct {
			Name string `json:"name"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if utf8.RuneCountInString(in.Name) > 64 {
			return core.ErrBad("Не длиннее 64 символов")
		}
		if k, ok := kontOf(me.ID, c.PathInt("id")); ok {
			konts.Update(k.ID, func(x *Kont) { x.Name = in.Name })
		}
		return c.OK()
	}, core.Auth)
}

func hideDialog(me, other int64) {
	for _, m := range mails.By("dialog", pairKey(me, other)) {
		if m.Unlink != 0 && m.Unlink != me {
			mails.Delete(m.ID)
		} else {
			mails.Update(m.ID, func(x *Mail) { x.Unlink = me })
		}
	}
}

func sortByTime(items []map[string]any) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j]["time"].(int64) > items[j-1]["time"].(int64); j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}
