package groups

import (
	"context"
	"encoding/json"
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

var (
	botTokensT  *store.Table[GroupToken, *GroupToken]
	botUpdatesT *store.Table[GroupUpdate, *GroupUpdate]

	lpMu      sync.Mutex
	lpWaiters = map[int64][]chan struct{}{}
)

// EmitUpdate записывает событие в очередь Long Poll сообщества и мгновенно будит ожидающих ботов.
func EmitUpdate(gid int64, typ, scope string, data any) {
	if botUpdatesT == nil || gid == 0 {
		return
	}
	b, err := json.Marshal(data)
	if err != nil {
		return
	}
	botUpdatesT.Insert(&GroupUpdate{
		IDGroup: gid,
		Type:    typ,
		Scope:   scope,
		Payload: string(b),
		Time:    core.Now(),
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

func registerBot(a *core.App) {
	botTokensT = store.NewTable[GroupToken](a.DB, "group_tokens").
		Index("group", func(x *GroupToken) string { return i(x.IDGroup) }).
		Index("token", func(x *GroupToken) string { return x.Token })
	botUpdatesT = store.NewTable[GroupUpdate](a.DB, "group_updates").
		Index("group", func(x *GroupUpdate) string { return i(x.IDGroup) })

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
