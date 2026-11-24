package groups

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

// GroupToken — API-токен (бот) сообщества. Работает только с методами своего сообщества.
type GroupToken struct {
	store.Base
	IDGroup    int64  `json:"id_group"`
	Name       string `json:"name"`
	Token      string `json:"token"`  // dcms_grp_<48 hex>
	Scopes     string `json:"scopes"` // через пробел: messages chat forum blog members files
	CreatedBy  int64  `json:"created_by"`
	CreatedAt  int64  `json:"created_at"`
	LastUsedAt int64  `json:"last_used_at"`
	Revoked    bool   `json:"revoked"`
}

// GroupUpdate — событие сообщества для Long Polling (getUpdates).
type GroupUpdate struct {
	store.Base
	IDGroup int64  `json:"id_group"`
	Type    string `json:"type"`    // message_new | chat_message_new | forum_topic_new | forum_post_new | blog_post_new | join_request | member_join | member_leave
	Scope   string `json:"scope"`   // требуемое разрешение токена для получения события
	Payload string `json:"payload"` // JSON-объект события
	Time    int64  `json:"time"`
}

// BotScopeInfo — описание разрешения токена сообщества.
type BotScopeInfo struct {
	Name  string `json:"name"`
	Title string `json:"title"`
	Desc  string `json:"desc"`
}

// AllBotScopes — разрешения, доступные для токенов ботов сообщества.
var AllBotScopes = []BotScopeInfo{
	{Name: "messages", Title: "Личные сообщения", Desc: "Получение входящих сообщений (message_new) и ответы пользователям от лица сообщества"},
	{Name: "chat", Title: "Чат сообщества", Desc: "Получение сообщений чата (chat_message_new) и отправка сообщений в чат"},
	{Name: "forum", Title: "Форум сообщества", Desc: "Получение новых тем/ответов (forum_topic_new, forum_post_new) и публикация на форуме"},
	{Name: "blog", Title: "Блог сообщества", Desc: "Получение новых записей (blog_post_new) и публикация постов в блог"},
	{Name: "members", Title: "Участники и заявки", Desc: "Получение заявок и вступлений (join_request, member_join, member_leave), авто-одобрение/отклонение заявок"},
	{Name: "files", Title: "Файлы сообщества", Desc: "Просмотр списка файлов сообщества"},
}

var botScopeSet = func() map[string]bool {
	m := map[string]bool{}
	for _, s := range AllBotScopes {
		m[s.Name] = true
	}
	return m
}()

// BotButton — кнопка в сообщении (inline) или под полем ввода (keyboard).
type BotButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	Color        string `json:"color,omitempty"` // default | primary | positive | negative
	URL          string `json:"url,omitempty"`
}

// BotCommand — команда бота для быстрого меню «/».
type BotCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

// GroupBotKeyboard — нижняя клавиатура бота для конкретного диалога (сообщество + пользователь).
type GroupBotKeyboard struct {
	store.Base
	IDGroup   int64  `json:"id_group"`
	IDUser    int64  `json:"id_user"`
	Keyboard  string `json:"keyboard"` // JSON [][]BotButton
	OneTime   bool   `json:"one_time"`
	UpdatedAt int64  `json:"updated_at"`
}

// GroupWebhook — настройки Callback API (вебхука) сообщества.
type GroupWebhook struct {
	store.Base
	IDGroup    int64  `json:"id_group"`
	URL        string `json:"url"`
	Secret     string `json:"secret"`
	Enabled    bool   `json:"enabled"`
	LastStatus int    `json:"last_status"`
	LastError  string `json:"last_error"`
	LastAt     int64  `json:"last_at"`
}

var (
	botTokensT   *store.Table[GroupToken, *GroupToken]
	botUpdatesT  *store.Table[GroupUpdate, *GroupUpdate]
	botKBT       *store.Table[GroupBotKeyboard, *GroupBotKeyboard]
	botWebhooksT *store.Table[GroupWebhook, *GroupWebhook]

	webhookHTTP = &http.Client{Timeout: 5 * time.Second}

	lpMu      sync.Mutex
	lpWaiters = map[int64][]chan struct{}{}
)

func deliverWebhook(wh GroupWebhook, body []byte, eventType string) (int, string) {
	req, err := http.NewRequest(http.MethodPost, wh.URL, bytes.NewReader(body))
	if err != nil {
		return 0, err.Error()
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "DCMS-Webhook/1.0")
	req.Header.Set("X-DCMS-Event", eventType)
	if wh.Secret != "" {
		mac := hmac.New(sha256.New, []byte(wh.Secret))
		mac.Write(body)
		req.Header.Set("X-DCMS-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	resp, err := webhookHTTP.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp.StatusCode, ""
	}
	return resp.StatusCode, resp.Status
}

// EmitUpdate записывает событие в очередь Long Poll сообщества, будит ожидающих ботов и отправляет Webhook (если включён).
func EmitUpdate(gid int64, typ, scope string, data any) {
	if botUpdatesT == nil || gid == 0 {
		return
	}
	b, err := json.Marshal(data)
	if err != nil {
		return
	}
	now := core.Now()
	updID := botUpdatesT.Insert(&GroupUpdate{
		IDGroup: gid,
		Type:    typ,
		Scope:   scope,
		Payload: string(b),
		Time:    now,
	})
	// Оставляем не более 1000 последних событий на сообщество, чтобы таблица не разрасталась
	all := botUpdatesT.By("group", i(gid))
	if len(all) > 1000 {
		sort.Slice(all, func(x, y int) bool { return all[x].ID < all[y].ID })
		for _, old := range all[:len(all)-1000] {
			botUpdatesT.Delete(old.ID)
		}
	}
	lpMu.Lock()
	chans := lpWaiters[gid]
	delete(lpWaiters, gid)
	lpMu.Unlock()
	for _, ch := range chans {
		close(ch)
	}
	if botWebhooksT != nil {
		if wh, ok := botWebhooksT.OneBy("group", i(gid)); ok && wh.Enabled && wh.URL != "" {
			envelope, err := json.Marshal(map[string]any{
				"update_id": updID,
				"group_id":  gid,
				"type":      typ,
				"scope":     scope,
				"time":      now,
				"data":      data,
			})
			if err == nil {
				go func(w GroupWebhook, payload []byte, ev string) {
					code, errMsg := deliverWebhook(w, payload, ev)
					botWebhooksT.Update(w.ID, func(x *GroupWebhook) {
						x.LastStatus = code
						x.LastError = errMsg
						x.LastAt = core.Now()
					})
				}(wh, envelope, typ)
			}
		}
	}
}

func waitForGroupUpdate(ctx context.Context, gid int64, timeout time.Duration) {
	ch := make(chan struct{})
	lpMu.Lock()
	lpWaiters[gid] = append(lpWaiters[gid], ch)
	lpMu.Unlock()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ch:
	case <-timer.C:
	case <-ctx.Done():
	}
}

// HasBotScope проверяет наличие разрешения у токена сообщества.
func HasBotScope(tok GroupToken, need string) bool {
	if need == "" {
		return true
	}
	for _, s := range strings.Fields(tok.Scopes) {
		if s == need {
			return true
		}
	}
	return false
}

// VerifyGroupToken проверяет Bearer-токен сообщества (dcms_grp_...).
func VerifyGroupToken(raw string) (GroupToken, Group, bool) {
	if botTokensT == nil || !strings.HasPrefix(raw, "dcms_grp_") {
		return GroupToken{}, Group{}, false
	}
	tok, ok := botTokensT.OneBy("token", raw)
	if !ok || tok.Revoked {
		return GroupToken{}, Group{}, false
	}
	g, ok := groupsT.Get(tok.IDGroup)
	if !ok || g.Blocked {
		return GroupToken{}, Group{}, false
	}
	now := core.Now()
	if now-tok.LastUsedAt >= 60 {
		botTokensT.Update(tok.ID, func(x *GroupToken) { x.LastUsedAt = now })
	}
	return tok, g, true
}

// BotGetUpdates реализует Long Polling (getUpdates) для бота сообщества.
func BotGetUpdates(ctx context.Context, tok GroupToken, offset int64, limit, timeoutSec int) []map[string]any {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	if timeoutSec < 0 {
		timeoutSec = 0
	}
	if timeoutSec > 30 {
		timeoutSec = 30
	}

	collect := func() []map[string]any {
		all := botUpdatesT.By("group", i(tok.IDGroup))
		sort.Slice(all, func(x, y int) bool { return all[x].ID < all[y].ID })
		out := make([]map[string]any, 0)
		for _, u := range all {
			if u.ID < offset {
				continue
			}
			if !HasBotScope(tok, u.Scope) {
				continue
			}
			var data any
			_ = json.Unmarshal([]byte(u.Payload), &data)
			out = append(out, map[string]any{
				"update_id": u.ID,
				"type":      u.Type,
				"time":      u.Time,
				"data":      data,
			})
			if len(out) >= limit {
				break
			}
		}
		return out
	}

	res := collect()
	if len(res) > 0 || timeoutSec == 0 {
		return res
	}
	waitForGroupUpdate(ctx, tok.IDGroup, time.Duration(timeoutSec)*time.Second)
	return collect()
}

// BotActor возвращает пользователя-создателя сообщества для выполнения действий от имени бота.
func BotActor(a *core.App, g Group) (model.User, bool) {
	return a.GetUser(g.IDCreator)
}

// BotListRequests возвращает список заявок на вступление в приватное сообщество.
func BotListRequests(a *core.App, g Group) []map[string]any {
	all := requestsT.By("group", i(g.ID))
	sort.Slice(all, func(x, y int) bool { return all[x].Time < all[y].Time })
	out := make([]map[string]any, 0, len(all))
	for _, r := range all {
		out = append(out, map[string]any{"user": a.Ref(r.IDUser), "time": r.Time})
	}
	return out
}

// BotApproveRequest автоматически одобряет заявку пользователя на вступление.
func BotApproveRequest(a *core.App, g Group, uid int64) error {
	req, ok := requestsT.OneBy("pair", store.Key(g.ID, uid))
	if !ok {
		return core.ErrNotFound("Заявка от этого пользователя не найдена")
	}
	requestsT.Delete(req.ID)
	if _, exists := memberOf(g.ID, uid); !exists {
		membersT.Insert(&GroupMember{IDGroup: g.ID, IDUser: uid, Role: "member", Time: core.Now()})
		a.Notify(g.IDCreator, uid, "group_req_ok", g.ID, false)
		EmitUpdate(g.ID, "member_join", "members", map[string]any{"user": a.Ref(uid), "via": "bot_approve"})
	}
	return nil
}

// BotRejectRequest отклоняет заявку пользователя на вступление.
func BotRejectRequest(g Group, uid int64) error {
	req, ok := requestsT.OneBy("pair", store.Key(g.ID, uid))
	if !ok {
		return core.ErrNotFound("Заявка от этого пользователя не найдена")
	}
	requestsT.Delete(req.ID)
	return nil
}

// BotSetCommands устанавливает список команд бота для меню «/».
func BotSetCommands(a *core.App, g Group, cmds []BotCommand) ([]BotCommand, error) {
	if len(cmds) > 30 {
		return nil, core.ErrBad("Максимум 30 команд")
	}
	clean := make([]BotCommand, 0, len(cmds))
	for _, c := range cmds {
		cmd := strings.TrimPrefix(strings.TrimSpace(c.Command), "/")
		cmd = a.Text.Clean(cmd)
		if cmd == "" || utf8.RuneCountInString(cmd) > 32 {
			continue
		}
		desc := a.Text.Clean(strings.TrimSpace(c.Description))
		if utf8.RuneCountInString(desc) > 120 {
			desc = core.Trunc(desc, 120)
		}
		clean = append(clean, BotCommand{Command: cmd, Description: desc})
	}
	b, _ := json.Marshal(clean)
	groupsT.Update(g.ID, func(x *Group) { x.BotCommands = string(b) })
	return clean, nil
}

// BotGetCommands возвращает список команд бота сообщества.
func BotGetCommands(g Group) []BotCommand {
	if g.BotCommands == "" {
		return nil
	}
	var out []BotCommand
	_ = json.Unmarshal([]byte(g.BotCommands), &out)
	return out
}

// sanitizeKeyboard очищает двумерный массив кнопок.
func sanitizeKeyboard(a *core.App, rows [][]BotButton) [][]BotButton {
	if len(rows) > 8 {
		rows = rows[:8]
	}
	out := make([][]BotButton, 0, len(rows))
	for _, r := range rows {
		if len(r) > 6 {
			r = r[:6]
		}
		cr := make([]BotButton, 0, len(r))
		for _, btn := range r {
			txt := a.Text.Clean(strings.TrimSpace(btn.Text))
			if txt == "" {
				continue
			}
			if utf8.RuneCountInString(txt) > 48 {
				txt = core.Trunc(txt, 48)
			}
			col := strings.ToLower(strings.TrimSpace(btn.Color))
			if col != "primary" && col != "positive" && col != "negative" {
				col = "default"
			}
			u := strings.TrimSpace(btn.URL)
			if u != "" && !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "http://") {
				u = ""
			}
			cb := strings.TrimSpace(btn.CallbackData)
			if len(cb) > 128 {
				cb = cb[:128]
			}
			cr = append(cr, BotButton{Text: txt, CallbackData: cb, Color: col, URL: u})
		}
		if len(cr) > 0 {
			out = append(out, cr)
		}
	}
	return out
}

// BotReplyWithKeyboard отправляет ответ от лица сообщества с поддержкой inline-кнопок и нижней клавиатуры.
func BotReplyWithKeyboard(a *core.App, g Group, targetUID int64, msg string, inlineKB [][]BotButton, replyKB [][]BotButton, oneTime, removeKB bool) (map[string]any, error) {
	if g.Blocked {
		return nil, core.ErrNotFound("Сообщество недоступно")
	}
	if !hasUserInitiated(g.ID, targetUID) {
		return nil, core.ErrForbidden("Сообщество не может писать первым — пользователь должен начать диалог сам")
	}
	txt := a.Text.Clean(strings.TrimSpace(msg))
	if utf8.RuneCountInString(txt) < 1 || utf8.RuneCountInString(txt) > 2000 {
		return nil, core.ErrBad("Сообщение: от 1 до 2000 символов")
	}
	cleanInline := sanitizeKeyboard(a, inlineKB)
	var inlineJSON string
	if len(cleanInline) > 0 {
		b, _ := json.Marshal(cleanInline)
		inlineJSON = string(b)
	}
	// Обновляем нижнюю клавиатуру диалога, если передана или запрошено удаление
	var kbUpdate any
	kbChanged := false
	if removeKB {
		kbChanged = true
		kbUpdate = nil
		if old, ok := botKBT.OneBy("pair", store.Key(g.ID, targetUID)); ok {
			botKBT.Delete(old.ID)
		}
	} else if len(replyKB) > 0 {
		cleanReply := sanitizeKeyboard(a, replyKB)
		if len(cleanReply) > 0 {
			kbChanged = true
			kbUpdate = cleanReply
			b, _ := json.Marshal(cleanReply)
			if old, ok := botKBT.OneBy("pair", store.Key(g.ID, targetUID)); ok {
				botKBT.Update(old.ID, func(x *GroupBotKeyboard) {
					x.Keyboard = string(b)
					x.OneTime = oneTime
					x.UpdatedAt = core.Now()
				})
			} else {
				botKBT.Insert(&GroupBotKeyboard{
					IDGroup: g.ID, IDUser: targetUID, Keyboard: string(b), OneTime: oneTime, UpdatedAt: core.Now(),
				})
			}
		}
	}

	now := core.Now()
	id := dmT.Insert(&GroupMessage{
		IDGroup: g.ID, IDUser: targetUID, IDStaff: g.IDCreator,
		FromGroup: true, Msg: txt, Time: now, ReadByGrp: true, InlineKB: inlineJSON,
	})
	dto := map[string]any{
		"id": id, "group_id": g.ID, "user_id": targetUID, "from_group": true,
		"msg": a.Text.Render(txt), "raw": txt, "time": now,
	}
	if len(cleanInline) > 0 {
		dto["inline_keyboard"] = cleanInline
	}
	if kbChanged {
		dto["keyboard_update"] = true
		dto["keyboard"] = kbUpdate
	}
	a.Hub.Publish("group_dm:"+i(g.ID), "group_dm", dto)
	a.Hub.ToUser(targetUID, "group_dm", dto)
	a.Hub.ToUser(targetUID, "counters", nil)
	a.Notify(g.IDCreator, targetUID, "group_dm", g.ID, false)
	return map[string]any{"id": id, "group_id": g.ID, "user_id": targetUID, "text": txt, "time": now}, nil
}

func getUserBotKeyboard(gid, uid int64) [][]BotButton {
	if botKBT == nil || uid == 0 {
		return nil
	}
	kb, ok := botKBT.OneBy("pair", store.Key(gid, uid))
	if !ok || kb.Keyboard == "" {
		return nil
	}
	var out [][]BotButton
	_ = json.Unmarshal([]byte(kb.Keyboard), &out)
	return out
}

func registerBot(a *core.App) {
	botTokensT = store.NewTable[GroupToken](a.DB, "group_tokens").
		Index("group", func(x *GroupToken) string { return i(x.IDGroup) }).
		Index("token", func(x *GroupToken) string { return x.Token })
	botUpdatesT = store.NewTable[GroupUpdate](a.DB, "group_updates").
		Index("group", func(x *GroupUpdate) string { return i(x.IDGroup) })
	botKBT = store.NewTable[GroupBotKeyboard](a.DB, "group_bot_keyboards").
		Index("group", func(x *GroupBotKeyboard) string { return i(x.IDGroup) }).
		Index("pair", func(x *GroupBotKeyboard) string { return store.Key(x.IDGroup, x.IDUser) })
	botWebhooksT = store.NewTable[GroupWebhook](a.DB, "group_webhooks").
		Index("group", func(x *GroupWebhook) string { return i(x.IDGroup) })

	// ── Вебхук (Callback API) сообщества
	a.GET("/api/groups/{id}/webhook", func(c *core.Ctx) error {
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
		wh, ok := botWebhooksT.OneBy("group", i(g.ID))
		if !ok {
			return c.JSON(map[string]any{"url": "", "secret": "", "enabled": false, "last_status": 0, "last_error": "", "last_at": 0})
		}
		return c.JSON(map[string]any{
			"url":         wh.URL,
			"secret":      wh.Secret,
			"enabled":     wh.Enabled,
			"last_status": wh.LastStatus,
			"last_error":  wh.LastError,
			"last_at":     wh.LastAt,
		})
	}, core.Auth)

	a.PUT("/api/groups/{id}/webhook", func(c *core.Ctx) error {
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
			URL     string `json:"url"`
			Secret  string `json:"secret"`
			Enabled bool   `json:"enabled"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		rawURL := strings.TrimSpace(in.URL)
		if rawURL != "" {
			u, err := url.Parse(rawURL)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				return core.ErrBad("URL вебхука должен начинаться с http:// или https://")
			}
		}
		secret := strings.TrimSpace(in.Secret)
		if len(secret) > 128 {
			return core.ErrBad("Секретный ключ: не более 128 символов")
		}
		if wh, ok := botWebhooksT.OneBy("group", i(g.ID)); ok {
			botWebhooksT.Update(wh.ID, func(x *GroupWebhook) {
				x.URL = rawURL
				x.Secret = secret
				x.Enabled = in.Enabled && rawURL != ""
			})
		} else {
			botWebhooksT.Insert(&GroupWebhook{
				IDGroup: g.ID,
				URL:     rawURL,
				Secret:  secret,
				Enabled: in.Enabled && rawURL != "",
			})
		}
		return c.OK()
	}, core.Auth)

	a.POST("/api/groups/{id}/webhook/test", func(c *core.Ctx) error {
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
		wh, ok := botWebhooksT.OneBy("group", i(g.ID))
		if !ok || wh.URL == "" {
			return core.ErrBad("Сначала укажите и сохраните URL вебхука")
		}
		payload, _ := json.Marshal(map[string]any{
			"update_id": 0,
			"group_id":  g.ID,
			"type":      "ping",
			"scope":     "",
			"time":      core.Now(),
			"data":      map[string]any{"message": "Тестовое событие от DCMS Callback API"},
		})
		code, errMsg := deliverWebhook(wh, payload, "ping")
		botWebhooksT.Update(wh.ID, func(x *GroupWebhook) {
			x.LastStatus = code
			x.LastError = errMsg
			x.LastAt = core.Now()
		})
		return c.JSON(map[string]any{"ok": code >= 200 && code < 300, "status": code, "error": errMsg})
	}, core.Auth)

	// Список токенов сообщества (создатель/админ)
	a.GET("/api/groups/{id}/tokens", func(c *core.Ctx) error {
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
		all := botTokensT.By("group", i(g.ID))
		sort.Slice(all, func(x, y int) bool { return all[x].ID > all[y].ID })
		items := make([]map[string]any, 0, len(all))
		for _, t := range all {
			if t.Revoked {
				continue
			}
			items = append(items, map[string]any{
				"id":         t.ID,
				"name":       t.Name,
				"token":      t.Token,
				"scopes":     strings.Fields(t.Scopes),
				"created_by": a.Ref(t.CreatedBy),
				"created_at": t.CreatedAt,
				"last_used":  t.LastUsedAt,
			})
		}
		return c.JSON(map[string]any{"items": items, "available_scopes": AllBotScopes})
	}, core.Auth)

	// Создать токен бота сообщества
	a.POST("/api/groups/{id}/tokens", func(c *core.Ctx) error {
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
		var activeCnt int
		for _, t := range botTokensT.By("group", i(g.ID)) {
			if !t.Revoked {
				activeCnt++
			}
		}
		if activeCnt >= 10 {
			return core.ErrBad("Максимум 10 активных API-токенов на одно сообщество")
		}
		var in struct {
			Name   string   `json:"name"`
			Scopes []string `json:"scopes"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		name := a.Text.Clean(strings.TrimSpace(in.Name))
		if name == "" {
			name = "Бот сообщества"
		}
		if utf8.RuneCountInString(name) > 64 {
			return core.ErrBad("Название токена: не более 64 символов")
		}
		seen := map[string]bool{}
		var valid []string
		for _, s := range in.Scopes {
			s = strings.ToLower(strings.TrimSpace(s))
			if botScopeSet[s] && !seen[s] {
				seen[s] = true
				valid = append(valid, s)
			}
		}
		if len(valid) == 0 {
			return core.ErrBad("Выберите хотя бы одно разрешение для токена")
		}
		now := core.Now()
		id := botTokensT.Insert(&GroupToken{
			IDGroup:   g.ID,
			Name:      name,
			Token:     "dcms_grp_" + core.RandToken(24),
			Scopes:    strings.Join(valid, " "),
			CreatedBy: me.ID,
			CreatedAt: now,
		})
		t, _ := botTokensT.Get(id)
		return c.JSON(map[string]any{
			"id":         t.ID,
			"name":       t.Name,
			"token":      t.Token,
			"scopes":     valid,
			"created_at": t.CreatedAt,
		})
	}, core.Auth)

	// Отозвать/удалить токен бота сообщества
	a.DELETE("/api/group-tokens/{tid}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		t, ok := botTokensT.Get(c.PathInt("tid"))
		if !ok {
			return core.ErrNotFound()
		}
		g, ok := groupsT.Get(t.IDGroup)
		if !ok {
			return core.ErrNotFound()
		}
		if !permsOf(me, g).Manage {
			return core.ErrForbidden()
		}
		botTokensT.Delete(t.ID)
		return c.OK()
	}, core.Auth)
}
