package oauth

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/mods/groups"
)

// registerV1Bot регистрирует Bot API v1 (/api/v1/bot/*) для токенов сообществ (dcms_grp_...) с Long Polling getUpdates.
func registerV1Bot(a *core.App, extractToken func(*http.Request) string, pageParams pageParamsFn) {
	regOpt := map[string]bool{}
	botHandle := func(method, path, needScope string, fn func(w http.ResponseWriter, r *http.Request, tok groups.GroupToken, g groups.Group, actor model.User)) {
		if !regOpt[path] {
			regOpt[path] = true
			a.Mux.HandleFunc("OPTIONS "+path, func(w http.ResponseWriter, r *http.Request) {
				setCORS(w)
				w.WriteHeader(http.StatusNoContent)
			})
		}
		a.Mux.HandleFunc(method+" "+path, func(w http.ResponseWriter, r *http.Request) {
			setCORS(w)
			raw := extractToken(r)
			if raw == "" {
				w.Header().Set("WWW-Authenticate", `Bearer realm="dcms-bot"`)
				writeOAuthErr(w, 401, "invalid_token", "Отсутствует токен сообщества (Authorization: Bearer dcms_grp_...)")
				return
			}
			tok, g, ok := groups.VerifyGroupToken(raw)
			if !ok {
				writeOAuthErr(w, 401, "invalid_token", "Токен сообщества недействителен, отозван или сообщество заблокировано")
				return
			}
			if needScope != "" && !groups.HasBotScope(tok, needScope) {
				writeOAuthErr(w, 403, "insufficient_scope", "У токена сообщества нет разрешения: "+needScope)
				return
			}
			actor, ok := groups.BotActor(a, g)
			if !ok {
				writeOAuthErr(w, 500, "server_error", "Создатель сообщества не найден")
				return
			}
			fn(w, r, tok, g, actor)
		})
	}

	// 1. GET /api/v1/bot/me — информация о сообществе и правах токена
	botHandle("GET", "/api/v1/bot/me", "", func(w http.ResponseWriter, r *http.Request, tok groups.GroupToken, g groups.Group, actor model.User) {
		card, _ := groups.APIGetGroup(a, &actor, g.ID)
		writeJSON(w, 200, map[string]any{
			"ok":         true,
			"token_name": tok.Name,
			"scopes":     strings.Fields(tok.Scopes),
			"group":      card,
		})
	})

	// 2. GET /api/v1/bot/getUpdates — Long Polling получения новых событий сообщества
	botHandle("GET", "/api/v1/bot/getUpdates", "", func(w http.ResponseWriter, r *http.Request, tok groups.GroupToken, _ groups.Group, _ model.User) {
		offset, _ := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		timeout, _ := strconv.Atoi(r.URL.Query().Get("timeout"))
		updates := groups.BotGetUpdates(r.Context(), tok, offset, limit, timeout)
		writeJSON(w, 200, map[string]any{
			"ok":     true,
			"result": updates,
		})
	})

	// 3. POST /api/v1/bot/sendMessage — ответить пользователю от лица сообщества (строго если он писал первым)
	botHandle("POST", "/api/v1/bot/sendMessage", "messages", func(w http.ResponseWriter, r *http.Request, _ groups.GroupToken, g groups.Group, actor model.User) {
		var in struct {
			UserID int64  `json:"user_id"`
			Text   string `json:"text"`
			Msg    string `json:"msg"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
			writeOAuthErr(w, 400, "invalid_request", "Ожидается JSON с полями user_id и text")
			return
		}
		body := in.Text
		if body == "" {
			body = in.Msg
		}
		res, err := groups.APIReplyInbox(a, &actor, g.ID, in.UserID, body)
		if err != nil {
			writeCoreErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "message": res})
	})

	// 4. GET /api/v1/bot/inbox — список входящих диалогов сообщества
	botHandle("GET", "/api/v1/bot/inbox", "messages", func(w http.ResponseWriter, r *http.Request, _ groups.GroupToken, g groups.Group, actor model.User) {
		items, err := groups.APIListInbox(a, &actor, g.ID)
		if err != nil {
			writeCoreErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "items": items})
	})

	// 5. GET /api/v1/bot/requests — список заявок на вступление в приватное сообщество
	botHandle("GET", "/api/v1/bot/requests", "members", func(w http.ResponseWriter, r *http.Request, _ groups.GroupToken, g groups.Group, _ model.User) {
		writeJSON(w, 200, map[string]any{"ok": true, "items": groups.BotListRequests(a, g)})
	})

	// 6. POST /api/v1/bot/approveRequest — одобрить заявку пользователя на вступление
	botHandle("POST", "/api/v1/bot/approveRequest", "members", func(w http.ResponseWriter, r *http.Request, _ groups.GroupToken, g groups.Group, _ model.User) {
		var in struct {
			UserID int64 `json:"user_id"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil || in.UserID <= 0 {
			writeOAuthErr(w, 400, "invalid_request", "Укажите user_id")
			return
		}
		if err := groups.BotApproveRequest(a, g, in.UserID); err != nil {
			writeCoreErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	})

	// 7. POST /api/v1/bot/rejectRequest — отклонить заявку на вступление
	botHandle("POST", "/api/v1/bot/rejectRequest", "members", func(w http.ResponseWriter, r *http.Request, _ groups.GroupToken, g groups.Group, _ model.User) {
		var in struct {
			UserID int64 `json:"user_id"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil || in.UserID <= 0 {
			writeOAuthErr(w, 400, "invalid_request", "Укажите user_id")
			return
		}
		if err := groups.BotRejectRequest(g, in.UserID); err != nil {
			writeCoreErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	})

	// 8. POST /api/v1/bot/sendChat — написать в чат сообщества
	botHandle("POST", "/api/v1/bot/sendChat", "chat", func(w http.ResponseWriter, r *http.Request, _ groups.GroupToken, g groups.Group, actor model.User) {
		var in struct {
			Text string `json:"text"`
			Msg  string `json:"msg"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
			writeOAuthErr(w, 400, "invalid_request", "Ожидается JSON с полем text")
			return
		}
		body := in.Text
		if body == "" {
			body = in.Msg
		}
		res, err := groups.APISendChat(a, &actor, g.ID, body)
		if err != nil {
			writeCoreErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "message": res})
	})

	// 9. POST /api/v1/bot/createPost — опубликовать запись в блоге сообщества
	botHandle("POST", "/api/v1/bot/createPost", "blog", func(w http.ResponseWriter, r *http.Request, _ groups.GroupToken, g groups.Group, actor model.User) {
		var in struct {
			Title string `json:"title"`
			Text  string `json:"text"`
			Msg   string `json:"msg"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
			writeOAuthErr(w, 400, "invalid_request", "Ожидается JSON с полем text")
			return
		}
		body := in.Text
		if body == "" {
			body = in.Msg
		}
		res, err := groups.APICreateBlogPost(a, &actor, g.ID, in.Title, body)
		if err != nil {
			writeCoreErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "post": res})
	})

	// 10. POST /api/v1/bot/createTopic — создать тему на форуме сообщества
	botHandle("POST", "/api/v1/bot/createTopic", "forum", func(w http.ResponseWriter, r *http.Request, _ groups.GroupToken, g groups.Group, actor model.User) {
		var in struct {
			Sub  int64  `json:"sub"`
			Name string `json:"name"`
			Text string `json:"text"`
			Msg  string `json:"msg"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
			writeOAuthErr(w, 400, "invalid_request", "Ожидается JSON с полями name и text")
			return
		}
		body := in.Text
		if body == "" {
			body = in.Msg
		}
		res, err := groups.APICreateTopic(a, &actor, g.ID, in.Sub, in.Name, body)
		if err != nil {
			writeCoreErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "topic": res})
	})

	// 11. POST /api/v1/bot/replyTopic — ответить в теме форума сообщества
	botHandle("POST", "/api/v1/bot/replyTopic", "forum", func(w http.ResponseWriter, r *http.Request, _ groups.GroupToken, _ groups.Group, actor model.User) {
		var in struct {
			TopicID int64  `json:"topic_id"`
			Text    string `json:"text"`
			Msg     string `json:"msg"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil || in.TopicID <= 0 {
			writeOAuthErr(w, 400, "invalid_request", "Укажите topic_id и text")
			return
		}
		body := in.Text
		if body == "" {
			body = in.Msg
		}
		res, err := groups.APIReplyTopic(a, &actor, in.TopicID, body)
		if err != nil {
			writeCoreErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "post": res})
	})

	// 12. GET /api/v1/bot/files — файлы сообщества
	botHandle("GET", "/api/v1/bot/files", "files", func(w http.ResponseWriter, r *http.Request, _ groups.GroupToken, g groups.Group, actor model.User) {
		page, per, offset := pageParams(r, 20)
		items, total, err := groups.APIListFiles(a, &actor, g.ID, offset, per)
		if err != nil {
			writeCoreErr(w, err)
			return
		}
		pagedJSON(w, items, total, page, per)
	})
}
