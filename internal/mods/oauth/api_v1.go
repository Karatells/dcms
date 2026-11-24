package oauth

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/mods/feeds"
	"dcms/internal/mods/profile"
)

// v1Handler — обработчик публичного REST API v1 с Bearer-токеном и проверкой scope.
func registerV1(a *core.App) {
	extractToken := func(r *http.Request) string {
		auth := strings.TrimSpace(r.Header.Get("Authorization"))
		if len(auth) > 7 && strings.EqualFold(auth[:7], "Bearer ") {
			return strings.TrimSpace(auth[7:])
		}
		return strings.TrimSpace(r.URL.Query().Get("access_token"))
	}

	registeredOptions := map[string]bool{}
	handle := func(method, path, needScope string, fn func(w http.ResponseWriter, r *http.Request, tok *OAuthToken, u *model.User)) {
		if !registeredOptions[path] {
			registeredOptions[path] = true
			a.Mux.HandleFunc("OPTIONS "+path, func(w http.ResponseWriter, r *http.Request) {
				setCORS(w)
				w.WriteHeader(http.StatusNoContent)
			})
		}
		a.Mux.HandleFunc(method+" "+path, func(w http.ResponseWriter, r *http.Request) {
			setCORS(w)
			if needScope == "-" {
				fn(w, r, nil, nil)
				return
			}
			raw := extractToken(r)
			if raw == "" {
				w.Header().Set("WWW-Authenticate", `Bearer realm="dcms"`)
				writeOAuthErr(w, 401, "invalid_token", "Отсутствует заголовок Authorization: Bearer <token>")
				return
			}
			tok, ok := tokensT.OneBy("access", raw)
			now := core.Now()
			if !ok || tok.Revoked || tok.ExpiresAt <= now {
				w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
				writeOAuthErr(w, 401, "invalid_token", "Токен недействителен, отозван или истёк")
				return
			}
			u, ok := a.GetUser(tok.UserID)
			if !ok || u.Activation != "" {
				writeOAuthErr(w, 401, "invalid_token", "Пользователь не найден или не активирован")
				return
			}
			if a.IsBanned(u.ID) {
				writeOAuthErr(w, 403, "user_banned", "Аккаунт пользователя заблокирован")
				return
			}
			if needScope != "" && !hasScope(tok.Scope, needScope) {
				writeOAuthErr(w, 403, "insufficient_scope", "Для этого метода требуется разрешение: "+needScope)
				return
			}
			if now-tok.LastUsedAt >= 60 {
				tokensT.Update(tok.ID, func(x *OAuthToken) { x.LastUsedAt = now })
			}
			fn(w, r, &tok, &u)
		})
	}

	pageParams := func(r *http.Request, defPer int) (page, per, offset int) {
		page, _ = strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		per, _ = strconv.Atoi(r.URL.Query().Get("per"))
		if per < 1 {
			per = defPer
		}
		if per > 100 {
			per = 100
		}
		return page, per, (page - 1) * per
	}

	userPublicDTO := func(u *model.User, includeEmail bool) map[string]any {
		g, _ := a.T.Groups.Get(u.GroupAccess)
		gender := "female"
		if u.Pol == 1 {
			gender = "male"
		}
		m := map[string]any{
			"id":        u.ID,
			"nick":      u.Nick,
			"name":      u.AnkName,
			"gender":    gender,
			"avatar":    core.AvatarURL(u, 200),
			"city":      u.AnkCity,
			"about":     u.AnkOSebe,
			"group":     g.Name,
			"level":     g.Level,
			"rating":    u.Rating,
			"medal":     core.Medal(u.Rating),
			"online":    core.Now()-u.DateLast < 300,
			"last_seen": u.DateLast,
			"reg_date":  u.DateReg,
		}
		if u.AnkGR > 0 {
			m["birthday"] = map[string]int64{"day": u.AnkDR, "month": u.AnkMR, "year": u.AnkGR}
		}
		if includeEmail {
			m["email"] = u.Email
			m["telegram"] = u.AnkTelegram
			m["vk"] = u.AnkVk
			m["skype"] = u.AnkSkype
			m["phone"] = u.AnkNTel
		}
		return m
	}

	canViewUser := func(viewer *model.User, target *model.User) bool {
		if viewer.ID == target.ID || viewer.Level >= 7 {
			return true
		}
		switch a.UserSettings(target.ID).PrivatStr {
		case 0:
			return false
		case 2:
			return a.IsFriend(viewer.ID, target.ID)
		}
		return true
	}

	// 1. GET /api/v1/site — публичная информация о сайте (токен не обязателен)
	handle("GET", "/api/v1/site", "-", func(w http.ResponseWriter, r *http.Request, _ *OAuthToken, _ *model.User) {
		now := core.Now()
		online := a.T.Users.Count(func(u *model.User) bool { return now-u.DateLast < 300 })
		writeJSON(w, 200, map[string]any{
			"site_name":     a.Cfg.Str("site_name"),
			"api_version":   "1.0",
			"users_total":   a.T.Users.Len(),
			"online_now":    online,
			"scopes":        AllScopes,
			"oauth_auth":    "/oauth/authorize",
			"oauth_token":   "/oauth/token",
			"oauth_revoke":  "/oauth/revoke",
		})
	})

	// 2. GET /api/v1/token/info — информация о текущем токене (любой валидный токен)
	handle("GET", "/api/v1/token/info", "", func(w http.ResponseWriter, r *http.Request, tok *OAuthToken, u *model.User) {
		app, _ := appsT.Get(tok.AppID)
		writeJSON(w, 200, map[string]any{
			"active":     true,
			"user_id":    u.ID,
			"nick":       u.Nick,
			"app_id":     app.ID,
			"app_name":   app.Name,
			"scopes":     strings.Fields(tok.Scope),
			"expires_at": tok.ExpiresAt,
			"created_at": tok.CreatedAt,
		})
	})

	// 3. GET /api/v1/me — профиль авторизованного пользователя
	handle("GET", "/api/v1/me", "profile", func(w http.ResponseWriter, r *http.Request, tok *OAuthToken, u *model.User) {
		writeJSON(w, 200, userPublicDTO(u, hasScope(tok.Scope, "email")))
	})

	// 4. GET /api/v1/users — поиск пользователей
	handle("GET", "/api/v1/users", "profile", func(w http.ResponseWriter, r *http.Request, _ *OAuthToken, _ *model.User) {
		q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
		page, per, offset := pageParams(r, 20)
		all := a.T.Users.All()
		var filt []model.User
		for _, u := range all {
			if u.Activation != "" {
				continue
			}
			if q != "" && !strings.Contains(u.NickLower, q) && !strings.Contains(strings.ToLower(u.AnkName), q) {
				continue
			}
			filt = append(filt, u)
		}
		sort.Slice(filt, func(x, y int) bool { return filt[x].ID < filt[y].ID })
		total := len(filt)
		end := offset + per
		if end > total {
			end = total
		}
		var sl []model.User
		if offset < total {
			sl = filt[offset:end]
		}
		items := make([]map[string]any, 0, len(sl))
		for i := range sl {
			items = append(items, userPublicDTO(&sl[i], false))
		}
		pages := (total + per - 1) / per
		if pages < 1 {
			pages = 1
		}
		writeJSON(w, 200, map[string]any{"items": items, "total": total, "page": page, "per": per, "pages": pages})
	})

	// 5. GET /api/v1/users/{id} — профиль пользователя по ID
	handle("GET", "/api/v1/users/{id}", "profile", func(w http.ResponseWriter, r *http.Request, tok *OAuthToken, me *model.User) {
		uid, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		target, ok := a.GetUser(uid)
		if !ok {
			writeOAuthErr(w, 404, "not_found", "Пользователь не найден")
			return
		}
		if !canViewUser(me, &target) {
			writeOAuthErr(w, 403, "privacy_restricted", "Профиль пользователя скрыт настройками приватности")
			return
		}
		includeEmail := target.ID == me.ID && hasScope(tok.Scope, "email")
		writeJSON(w, 200, userPublicDTO(&target, includeEmail))
	})

	serveFriends := func(w http.ResponseWriter, r *http.Request, targetID int64) {
		page, per, offset := pageParams(r, 30)
		fr := a.T.Friends.By("user", i(targetID))
		sort.Slice(fr, func(x, y int) bool { return fr[x].Time > fr[y].Time })
		total := len(fr)
		end := offset + per
		if end > total {
			end = total
		}
		var sl []model.Frends
		if offset < total {
			sl = fr[offset:end]
		}
		items := make([]model.UserRef, 0, len(sl))
		for _, f := range sl {
			items = append(items, a.Ref(f.Frend))
		}
		pages := (total + per - 1) / per
		if pages < 1 {
			pages = 1
		}
		writeJSON(w, 200, map[string]any{"items": items, "total": total, "page": page, "per": per, "pages": pages})
	}

	// 6. GET /api/v1/me/friends — друзья текущего пользователя
	handle("GET", "/api/v1/me/friends", "friends", func(w http.ResponseWriter, r *http.Request, _ *OAuthToken, me *model.User) {
		serveFriends(w, r, me.ID)
	})

	// 7. GET /api/v1/users/{id}/friends — друзья пользователя
	handle("GET", "/api/v1/users/{id}/friends", "friends", func(w http.ResponseWriter, r *http.Request, _ *OAuthToken, me *model.User) {
		uid, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		target, ok := a.GetUser(uid)
		if !ok {
			writeOAuthErr(w, 404, "not_found", "Пользователь не найден")
			return
		}
		if !canViewUser(me, &target) {
			writeOAuthErr(w, 403, "privacy_restricted", "Список друзей скрыт настройками приватности")
			return
		}
		serveFriends(w, r, target.ID)
	})

	// 8. GET /api/v1/users/{id}/wall — стена пользователя
	handle("GET", "/api/v1/users/{id}/wall", "wall", func(w http.ResponseWriter, r *http.Request, _ *OAuthToken, me *model.User) {
		uid, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		target, ok := a.GetUser(uid)
		if !ok {
			writeOAuthErr(w, 404, "not_found", "Пользователь не найден")
			return
		}
		if !canViewUser(me, &target) {
			writeOAuthErr(w, 403, "privacy_restricted", "Стена скрыта настройками приватности")
			return
		}
		page, per, offset := pageParams(r, 20)
		items, total := profile.GetWallPosts(a, target.ID, offset, per)
		pages := (total + per - 1) / per
		if pages < 1 {
			pages = 1
		}
		writeJSON(w, 200, map[string]any{"items": items, "total": total, "page": page, "per": per, "pages": pages})
	})

	// 9. POST /api/v1/me/wall — публикация записи на своей стене
	handle("POST", "/api/v1/me/wall", "wall_write", func(w http.ResponseWriter, r *http.Request, _ *OAuthToken, me *model.User) {
		var in struct {
			Text string `json:"text"`
			Msg  string `json:"msg"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
			writeOAuthErr(w, 400, "invalid_request", "Ожидается JSON с полем text")
			return
		}
		txt := strings.TrimSpace(in.Text)
		if txt == "" {
			txt = strings.TrimSpace(in.Msg)
		}
		p, err := profile.CreateWallPost(a, me.ID, me.ID, txt)
		if err != nil {
			writeOAuthErr(w, 400, "invalid_request", err.Error())
			return
		}
		writeJSON(w, 201, map[string]any{
			"id":      p.ID,
			"wall_id": p.IDStena,
			"author":  a.Ref(p.IDUser),
			"text":    p.Msg,
			"html":    a.Text.Render(p.Msg),
			"time":    p.Time,
		})
	})

	// 10. GET /api/v1/me/notifications — уведомления авторизованного пользователя
	handle("GET", "/api/v1/me/notifications", "notifications", func(w http.ResponseWriter, r *http.Request, _ *OAuthToken, me *model.User) {
		page, per, offset := pageParams(r, 20)
		all := a.T.Notifications.By("user", i(me.ID))
		sort.Slice(all, func(x, y int) bool { return all[x].ID > all[y].ID })
		total := len(all)
		end := offset + per
		if end > total {
			end = total
		}
		var sl []model.Notification
		if offset < total {
			sl = all[offset:end]
		}
		items := make([]map[string]any, 0, len(sl))
		for _, n := range sl {
			items = append(items, map[string]any{
				"id":        n.ID,
				"type":      n.Type,
				"object_id": n.IDObject,
				"from":      a.Ref(n.Avtor),
				"read":      n.Read == 1,
				"time":      n.Time,
			})
		}
		pages := (total + per - 1) / per
		if pages < 1 {
			pages = 1
		}
		writeJSON(w, 200, map[string]any{"items": items, "total": total, "page": page, "per": per, "pages": pages})
	})

	// 11. GET /api/v1/me/messages — список диалогов (опасное разрешение: messages)
	handle("GET", "/api/v1/me/messages", "messages", func(w http.ResponseWriter, r *http.Request, _ *OAuthToken, me *model.User) {
		page, per, offset := pageParams(r, 20)
		items, total := feeds.ListDialogs(a, me.ID, offset, per)
		pages := (total + per - 1) / per
		if pages < 1 {
			pages = 1
		}
		writeJSON(w, 200, map[string]any{"items": items, "total": total, "page": page, "per": per, "pages": pages})
	})

	// 12. GET /api/v1/me/messages/{id} — сообщения переписки с пользователем {id} (опасное разрешение: messages)
	handle("GET", "/api/v1/me/messages/{id}", "messages", func(w http.ResponseWriter, r *http.Request, _ *OAuthToken, me *model.User) {
		peerID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if _, ok := a.GetUser(peerID); !ok {
			writeOAuthErr(w, 404, "not_found", "Пользователь не найден")
			return
		}
		page, per, offset := pageParams(r, 30)
		items, total := feeds.ListDialogMessages(a, me.ID, peerID, offset, per)
		pages := (total + per - 1) / per
		if pages < 1 {
			pages = 1
		}
		writeJSON(w, 200, map[string]any{"peer": a.Ref(peerID), "items": items, "total": total, "page": page, "per": per, "pages": pages})
	})

	// 13. POST /api/v1/me/messages/{id} — отправка личного сообщения пользователю {id} (опасное разрешение: messages_write)
	handle("POST", "/api/v1/me/messages/{id}", "messages_write", func(w http.ResponseWriter, r *http.Request, _ *OAuthToken, me *model.User) {
		peerID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		target, ok := a.GetUser(peerID)
		if !ok {
			writeOAuthErr(w, 404, "not_found", "Получатель не найден")
			return
		}
		var in struct {
			Text    string `json:"text"`
			Msg     string `json:"msg"`
			ReplyID int64  `json:"reply_id"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
			writeOAuthErr(w, 400, "invalid_request", "Ожидается JSON с полем text")
			return
		}
		txt := strings.TrimSpace(in.Text)
		if txt == "" {
			txt = strings.TrimSpace(in.Msg)
		}
		m, err := feeds.SendMessage(a, me, &target, txt, in.ReplyID)
		if err != nil {
			code := 400
			errCode := "invalid_request"
			if he, ok := err.(*core.HTTPError); ok {
				code = he.Code
				if code == 403 {
					errCode = "forbidden"
				}
			}
			writeOAuthErr(w, code, errCode, err.Error())
			return
		}
		writeJSON(w, 201, map[string]any{
			"id":       m.ID,
			"from":     a.Ref(m.IDUser),
			"to":       a.Ref(m.IDKont),
			"text":     m.Msg,
			"html":     a.Text.Render(m.Msg),
			"reply_id": m.ReplyID,
			"time":     m.Time,
		})
	})

	registerV1Groups(a, handle, pageParams)
}
