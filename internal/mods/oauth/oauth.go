// Package oauth — OAuth 2.0 авторизация (Authorization Code + PKCE + Refresh Token) и публичный REST API v1.
package oauth

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"unicode/utf8"

	"dcms/internal/core"
	"dcms/internal/store"
)

// ---- таблицы модуля

// OAuthApp — зарегистрированное стороннее OAuth-приложение.
type OAuthApp struct {
	store.Base
	OwnerID      int64  `json:"owner_id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Website      string `json:"website"`
	RedirectURIs string `json:"redirect_uris"` // по одному URL на строку
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
}

// OAuthCode — одноразовый код авторизации (TTL 5 минут).
type OAuthCode struct {
	store.Base
	Code                string `json:"code"`
	AppID               int64  `json:"app_id"`
	UserID              int64  `json:"user_id"`
	RedirectURI         string `json:"redirect_uri"`
	Scope               string `json:"scope"`
	CodeChallenge       string `json:"code_challenge"`
	CodeChallengeMethod string `json:"code_challenge_method"`
	ExpiresAt           int64  `json:"expires_at"`
	Used                bool   `json:"used"`
}

// OAuthToken — выданный access_token + refresh_token.
type OAuthToken struct {
	store.Base
	AppID            int64  `json:"app_id"`
	UserID           int64  `json:"user_id"`
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	Scope            string `json:"scope"`
	ExpiresAt        int64  `json:"expires_at"`
	RefreshExpiresAt int64  `json:"refresh_expires_at"`
	CreatedAt        int64  `json:"created_at"`
	LastUsedAt       int64  `json:"last_used_at"`
	Revoked          bool   `json:"revoked"`
}

// ScopeInfo — описание разрешения OAuth.
type ScopeInfo struct {
	Name   string `json:"name"`
	Title  string `json:"title"`
	Desc   string `json:"desc"`
	Danger bool   `json:"danger,omitempty"`
}

// AllScopes — поддерживаемые разрешения OAuth 2.0.
var AllScopes = []ScopeInfo{
	{Name: "profile", Title: "Базовый профиль", Desc: "Доступ к ID, нику, имени, полу, аватару, городу, рейтингу, статусу онлайн и группе"},
	{Name: "email", Title: "Почта и контакты", Desc: "Доступ к вашему e-mail, Telegram, VK, Skype и номеру телефона из анкеты"},
	{Name: "friends", Title: "Список друзей", Desc: "Просмотр списка ваших друзей и друзей других пользователей"},
	{Name: "wall", Title: "Чтение стены", Desc: "Просмотр записей на вашей стене и стенах пользователей"},
	{Name: "wall_write", Title: "Публикация на стене", Desc: "Публикация новых записей на вашей стене от вашего имени"},
	{Name: "notifications", Title: "Уведомления", Desc: "Чтение списка ваших уведомлений на сайте"},
	{Name: "messages", Title: "Чтение личных сообщений", Desc: "Полный доступ к списку диалогов и содержимому ваших личных переписок", Danger: true},
	{Name: "messages_write", Title: "Отправка личных сообщений", Desc: "Отправка личных сообщений другим пользователям от вашего имени", Danger: true},
	{Name: "offline", Title: "Долгий доступ", Desc: "Продление токена на 30 дней без повторной авторизации"},
}

var scopeMap = func() map[string]ScopeInfo {
	m := make(map[string]ScopeInfo, len(AllScopes))
	for _, s := range AllScopes {
		m[s.Name] = s
	}
	return m
}()

var (
	appsT   *store.Table[OAuthApp, *OAuthApp]
	codesT  *store.Table[OAuthCode, *OAuthCode]
	tokensT *store.Table[OAuthToken, *OAuthToken]
)

func i(n int64) string { return store.I(n) }

// parseScopes очищает, дедуплицирует и валидирует список разрешений.
func parseScopes(raw string) ([]string, error) {
	raw = strings.ReplaceAll(raw, ",", " ")
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return []string{"profile"}, nil
	}
	seen := map[string]bool{}
	var out []string
	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if _, ok := scopeMap[p]; !ok {
			return nil, core.ErrBad("Неизвестное разрешение (scope): " + p)
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out, nil
}

func hasScope(tokenScope, need string) bool {
	if need == "" {
		return true
	}
	for _, s := range strings.Fields(tokenScope) {
		if s == need {
			return true
		}
	}
	return false
}

func normalizeRedirectURIs(raw string) ([]string, error) {
	lines := strings.FieldsFunc(raw, func(r rune) bool { return r == '\n' || r == '\r' || r == ',' })
	var out []string
	seen := map[string]bool{}
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		u, err := url.Parse(l)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return nil, core.ErrBad("Некорректный Redirect URI: " + l + " (требуется http:// или https://)")
		}
		if u.Fragment != "" {
			return nil, core.ErrBad("Redirect URI не должен содержать фрагмент (#...): " + l)
		}
		if !seen[l] {
			seen[l] = true
			out = append(out, l)
		}
	}
	if len(out) == 0 {
		return nil, core.ErrBad("Укажите хотя бы один Redirect URI")
	}
	if len(out) > 10 {
		return nil, core.ErrBad("Максимум 10 Redirect URI на одно приложение")
	}
	return out, nil
}

func matchRedirectURI(app OAuthApp, uri string) bool {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return false
	}
	for _, allowed := range strings.Split(app.RedirectURIs, "\n") {
		if strings.TrimSpace(allowed) == uri {
			return true
		}
	}
	return false
}

func appendQueryParams(base string, params map[string]string) string {
	u, err := url.Parse(base)
	if err != nil {
		return base
	}
	q := u.Query()
	for k, v := range params {
		if v != "" {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func setCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept")
	w.Header().Set("Access-Control-Max-Age", "86400")
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeOAuthErr(w http.ResponseWriter, code int, errCode, desc string) {
	writeJSON(w, code, map[string]string{
		"error":             errCode,
		"error_description": desc,
	})
}

func verifyPKCE(verifier, challenge, method string) bool {
	if challenge == "" {
		return true
	}
	if verifier == "" {
		return false
	}
	switch strings.ToUpper(method) {
	case "", "PLAIN":
		return subtle.ConstantTimeCompare([]byte(verifier), []byte(challenge)) == 1
	case "S256":
		sum := sha256.Sum256([]byte(verifier))
		got := base64.RawURLEncoding.EncodeToString(sum[:])
		return subtle.ConstantTimeCompare([]byte(got), []byte(challenge)) == 1
	default:
		return false
	}
}

// Register регистрирует OAuth 2.0 и публичный REST API v1.
func Register(a *core.App) {
	appsT = store.NewTable[OAuthApp](a.DB, "oauth_apps").
		Index("owner", func(x *OAuthApp) string { return i(x.OwnerID) }).
		Index("client_id", func(x *OAuthApp) string { return x.ClientID })
	codesT = store.NewTable[OAuthCode](a.DB, "oauth_codes").
		Index("code", func(x *OAuthCode) string { return x.Code }).
		Index("app", func(x *OAuthCode) string { return i(x.AppID) })
	tokensT = store.NewTable[OAuthToken](a.DB, "oauth_tokens").
		Index("access", func(x *OAuthToken) string { return x.AccessToken }).
		Index("refresh", func(x *OAuthToken) string { return x.RefreshToken }).
		Index("user", func(x *OAuthToken) string { return i(x.UserID) }).
		Index("app", func(x *OAuthToken) string { return i(x.AppID) })

	a.OnUserDelete(func(uid int64) {
		for _, tok := range tokensT.By("user", i(uid)) {
			tokensT.Delete(tok.ID)
		}
		for _, app := range appsT.By("owner", i(uid)) {
			for _, tok := range tokensT.By("app", i(app.ID)) {
				tokensT.Delete(tok.ID)
			}
			for _, cd := range codesT.By("app", i(app.ID)) {
				codesT.Delete(cd.ID)
			}
			appsT.Delete(app.ID)
		}
	})

	// ── Список всех доступных разрешений (scopes)
	a.GET("/api/oauth/scopes", func(c *core.Ctx) error {
		return c.JSON(map[string]any{"scopes": AllScopes})
	})

	// ── Управление своими OAuth-приложениями (SPA)
	appDTO := func(app OAuthApp) map[string]any {
		now := core.Now()
		users := map[int64]bool{}
		for _, t := range tokensT.By("app", i(app.ID)) {
			if !t.Revoked && t.RefreshExpiresAt > now {
				users[t.UserID] = true
			}
		}
		return map[string]any{
			"id":            app.ID,
			"name":          app.Name,
			"description":   app.Description,
			"website":       app.Website,
			"redirect_uris": strings.Split(app.RedirectURIs, "\n"),
			"client_id":     app.ClientID,
			"client_secret": app.ClientSecret,
			"active_users":  len(users),
			"created_at":    app.CreatedAt,
			"updated_at":    app.UpdatedAt,
		}
	}

	a.GET("/api/oauth/apps", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		list := appsT.By("owner", i(me.ID))
		sort.Slice(list, func(x, y int) bool { return list[x].ID > list[y].ID })
		items := make([]map[string]any, 0, len(list))
		for _, app := range list {
			items = append(items, appDTO(app))
		}
		return c.JSON(map[string]any{"items": items})
	}, core.Auth)

	a.POST("/api/oauth/apps", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if len(appsT.By("owner", i(me.ID))) >= 20 {
			return core.ErrBad("Максимум 20 OAuth-приложений на аккаунт")
		}
		var in struct {
			Name         string `json:"name"`
			Description  string `json:"description"`
			Website      string `json:"website"`
			RedirectURIs string `json:"redirect_uris"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		name := a.Text.Clean(strings.TrimSpace(in.Name))
		if utf8.RuneCountInString(name) < 2 || utf8.RuneCountInString(name) > 64 {
			return core.ErrBad("Название приложения: от 2 до 64 символов")
		}
		desc := a.Text.Clean(strings.TrimSpace(in.Description))
		if utf8.RuneCountInString(desc) > 500 {
			return core.ErrBad("Описание: не более 500 символов")
		}
		website := strings.TrimSpace(in.Website)
		if website != "" {
			u, err := url.Parse(website)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				return core.ErrBad("Некорректный URL сайта (нужен http:// или https://)")
			}
		}
		uris, err := normalizeRedirectURIs(in.RedirectURIs)
		if err != nil {
			return err
		}
		now := core.Now()
		id := appsT.Insert(&OAuthApp{
			OwnerID:      me.ID,
			Name:         name,
			Description:  desc,
			Website:      website,
			RedirectURIs: strings.Join(uris, "\n"),
			ClientID:     "dcms_" + core.RandToken(12),
			ClientSecret: "dcms_sec_" + core.RandToken(24),
			CreatedAt:    now,
			UpdatedAt:    now,
		})
		app, _ := appsT.Get(id)
		return c.JSON(appDTO(app))
	}, core.Auth)

	a.PUT("/api/oauth/apps/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		app, ok := appsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Приложение не найдено")
		}
		if app.OwnerID != me.ID && me.Level < 3 {
			return core.ErrForbidden()
		}
		var in struct {
			Name         string `json:"name"`
			Description  string `json:"description"`
			Website      string `json:"website"`
			RedirectURIs string `json:"redirect_uris"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		name := a.Text.Clean(strings.TrimSpace(in.Name))
		if utf8.RuneCountInString(name) < 2 || utf8.RuneCountInString(name) > 64 {
			return core.ErrBad("Название приложения: от 2 до 64 символов")
		}
		desc := a.Text.Clean(strings.TrimSpace(in.Description))
		if utf8.RuneCountInString(desc) > 500 {
			return core.ErrBad("Описание: не более 500 символов")
		}
		website := strings.TrimSpace(in.Website)
		if website != "" {
			u, err := url.Parse(website)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				return core.ErrBad("Некорректный URL сайта")
			}
		}
		uris, err := normalizeRedirectURIs(in.RedirectURIs)
		if err != nil {
			return err
		}
		appsT.Update(app.ID, func(x *OAuthApp) {
			x.Name = name
			x.Description = desc
			x.Website = website
			x.RedirectURIs = strings.Join(uris, "\n")
			x.UpdatedAt = core.Now()
		})
		upd, _ := appsT.Get(app.ID)
		return c.JSON(appDTO(upd))
	}, core.Auth)

	a.POST("/api/oauth/apps/{id}/secret", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		app, ok := appsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Приложение не найдено")
		}
		if app.OwnerID != me.ID && me.Level < 3 {
			return core.ErrForbidden()
		}
		appsT.Update(app.ID, func(x *OAuthApp) {
			x.ClientSecret = "dcms_sec_" + core.RandToken(24)
			x.UpdatedAt = core.Now()
		})
		upd, _ := appsT.Get(app.ID)
		return c.JSON(appDTO(upd))
	}, core.Auth)

	a.DELETE("/api/oauth/apps/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		app, ok := appsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Приложение не найдено")
		}
		if app.OwnerID != me.ID && me.Level < 3 {
			return core.ErrForbidden()
		}
		for _, tok := range tokensT.By("app", i(app.ID)) {
			tokensT.Delete(tok.ID)
		}
		for _, cd := range codesT.By("app", i(app.ID)) {
			codesT.Delete(cd.ID)
		}
		appsT.Delete(app.ID)
		return c.OK()
	}, core.Auth)

	// ── Выданные пользователем доступы сторонним приложениям
	a.GET("/api/oauth/grants", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		now := core.Now()
		type agg struct {
			app      OAuthApp
			scopes   map[string]bool
			lastUsed int64
			created  int64
		}
		byApp := map[int64]*agg{}
		for _, tok := range tokensT.By("user", i(me.ID)) {
			if tok.Revoked || tok.RefreshExpiresAt <= now {
				continue
			}
			app, ok := appsT.Get(tok.AppID)
			if !ok {
				continue
			}
			g, exists := byApp[app.ID]
			if !exists {
				g = &agg{app: app, scopes: map[string]bool{}, created: tok.CreatedAt}
				byApp[app.ID] = g
			}
			for _, sc := range strings.Fields(tok.Scope) {
				g.scopes[sc] = true
			}
			if tok.LastUsedAt > g.lastUsed {
				g.lastUsed = tok.LastUsedAt
			}
			if tok.CreatedAt < g.created {
				g.created = tok.CreatedAt
			}
		}
		items := make([]map[string]any, 0, len(byApp))
		for _, g := range byApp {
			var scList []ScopeInfo
			for _, s := range AllScopes {
				if g.scopes[s.Name] {
					scList = append(scList, s)
				}
			}
			items = append(items, map[string]any{
				"app_id":      g.app.ID,
				"name":        g.app.Name,
				"description": g.app.Description,
				"website":     g.app.Website,
				"owner":       a.Ref(g.app.OwnerID),
				"scopes":      scList,
				"created_at":  g.created,
				"last_used":   g.lastUsed,
			})
		}
		sort.Slice(items, func(x, y int) bool { return items[x]["created_at"].(int64) > items[y]["created_at"].(int64) })
		return c.JSON(map[string]any{"items": items})
	}, core.Auth)

	a.DELETE("/api/oauth/grants/{app_id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		appID := c.PathInt("app_id")
		for _, tok := range tokensT.By("user", i(me.ID)) {
			if tok.AppID == appID && !tok.Revoked {
				tokensT.Update(tok.ID, func(x *OAuthToken) { x.Revoked = true })
			}
		}
		return c.OK()
	}, core.Auth)

	// ── Экран согласия OAuth 2.0 (вызывается из SPA /oauth/authorize)
	a.GET("/api/oauth/authorize-info", func(c *core.Ctx) error {
		clientID := strings.TrimSpace(c.Q("client_id"))
		redirectURI := strings.TrimSpace(c.Q("redirect_uri"))
		respType := strings.TrimSpace(c.Q("response_type"))
		if respType == "" {
			respType = "code"
		}
		if respType != "code" {
			return core.ErrBad("Поддерживается только response_type=code")
		}
		app, ok := appsT.OneBy("client_id", clientID)
		if !ok {
			return core.ErrNotFound("Приложение с указанным client_id не найдено")
		}
		if !matchRedirectURI(app, redirectURI) {
			return core.ErrBad("redirect_uri не совпадает со списком разрешённых URL приложения")
		}
		scopes, err := parseScopes(c.Q("scope"))
		if err != nil {
			return err
		}
		scInfos := make([]ScopeInfo, 0, len(scopes))
		for _, s := range scopes {
			scInfos = append(scInfos, scopeMap[s])
		}
		return c.JSON(map[string]any{
			"app": map[string]any{
				"id":          app.ID,
				"name":        app.Name,
				"description": app.Description,
				"website":     app.Website,
				"owner":       a.Ref(app.OwnerID),
			},
			"scopes":       scInfos,
			"scope_string": strings.Join(scopes, " "),
			"redirect_uri": redirectURI,
			"state":        c.Q("state"),
			"logged_in":    c.User() != nil,
		})
	})

	a.POST("/api/oauth/authorize", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		var in struct {
			ClientID            string `json:"client_id"`
			RedirectURI         string `json:"redirect_uri"`
			Scope               string `json:"scope"`
			State               string `json:"state"`
			Approve             bool   `json:"approve"`
			CodeChallenge       string `json:"code_challenge"`
			CodeChallengeMethod string `json:"code_challenge_method"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		app, ok := appsT.OneBy("client_id", strings.TrimSpace(in.ClientID))
		if !ok {
			return core.ErrNotFound("Приложение не найдено")
		}
		if !matchRedirectURI(app, in.RedirectURI) {
			return core.ErrBad("Недопустимый redirect_uri")
		}
		if !in.Approve {
			return c.JSON(map[string]any{
				"redirect": appendQueryParams(in.RedirectURI, map[string]string{
					"error":             "access_denied",
					"error_description": "Пользователь отклонил запрос доступа",
					"state":             in.State,
				}),
			})
		}
		scopes, err := parseScopes(in.Scope)
		if err != nil {
			return err
		}
		code := core.RandToken(24)
		codesT.Insert(&OAuthCode{
			Code:                code,
			AppID:               app.ID,
			UserID:              me.ID,
			RedirectURI:         strings.TrimSpace(in.RedirectURI),
			Scope:               strings.Join(scopes, " "),
			CodeChallenge:       strings.TrimSpace(in.CodeChallenge),
			CodeChallengeMethod: strings.TrimSpace(in.CodeChallengeMethod),
			ExpiresAt:           core.Now() + 300,
		})
		return c.JSON(map[string]any{
			"redirect": appendQueryParams(in.RedirectURI, map[string]string{
				"code":  code,
				"state": in.State,
			}),
		})
	}, core.Auth)

	// ── Внешний endpoint: POST /oauth/token (RFC 6749, без X-DCMS, с CORS)
	a.Mux.HandleFunc("OPTIONS /oauth/token", func(w http.ResponseWriter, r *http.Request) {
		setCORS(w)
		w.WriteHeader(http.StatusNoContent)
	})
	a.Mux.HandleFunc("POST /oauth/token", func(w http.ResponseWriter, r *http.Request) {
		setCORS(w)
		var grantType, clientID, clientSecret, code, redirectURI, refreshToken, codeVerifier string
		ct := r.Header.Get("Content-Type")
		if strings.Contains(ct, "application/json") {
			var body map[string]string
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
				writeOAuthErr(w, 400, "invalid_request", "Некорректный JSON")
				return
			}
			grantType = body["grant_type"]
			clientID = body["client_id"]
			clientSecret = body["client_secret"]
			code = body["code"]
			redirectURI = body["redirect_uri"]
			refreshToken = body["refresh_token"]
			codeVerifier = body["code_verifier"]
		} else {
			_ = r.ParseForm()
			grantType = r.FormValue("grant_type")
			clientID = r.FormValue("client_id")
			clientSecret = r.FormValue("client_secret")
			code = r.FormValue("code")
			redirectURI = r.FormValue("redirect_uri")
			refreshToken = r.FormValue("refresh_token")
			codeVerifier = r.FormValue("code_verifier")
		}
		if u, p, ok := r.BasicAuth(); ok {
			if clientID == "" {
				clientID = u
			}
			if clientSecret == "" {
				clientSecret = p
			}
		}

		app, ok := appsT.OneBy("client_id", strings.TrimSpace(clientID))
		if !ok || subtle.ConstantTimeCompare([]byte(app.ClientSecret), []byte(clientSecret)) != 1 {
			writeOAuthErr(w, 401, "invalid_client", "Неверный client_id или client_secret")
			return
		}

		now := core.Now()
		issueToken := func(userID int64, scope string) {
			ttl := int64(3600)
			if hasScope(scope, "offline") {
				ttl = 30 * 86400
			}
			tid := tokensT.Insert(&OAuthToken{
				AppID:            app.ID,
				UserID:           userID,
				AccessToken:      "dcms_at_" + core.RandToken(24),
				RefreshToken:     "dcms_rt_" + core.RandToken(24),
				Scope:            scope,
				ExpiresAt:        now + ttl,
				RefreshExpiresAt: now + 30*86400,
				CreatedAt:        now,
				LastUsedAt:       now,
			})
			tok, _ := tokensT.Get(tid)
			writeJSON(w, 200, map[string]any{
				"access_token":  tok.AccessToken,
				"token_type":    "Bearer",
				"expires_in":    ttl,
				"refresh_token": tok.RefreshToken,
				"scope":         tok.Scope,
				"user_id":       tok.UserID,
			})
		}

		switch grantType {
		case "authorization_code":
			cd, ok := codesT.OneBy("code", strings.TrimSpace(code))
			if !ok || cd.AppID != app.ID || cd.Used || cd.ExpiresAt < now {
				writeOAuthErr(w, 400, "invalid_grant", "Код авторизации недействителен или истёк")
				return
			}
			if redirectURI != "" && strings.TrimSpace(redirectURI) != cd.RedirectURI {
				writeOAuthErr(w, 400, "invalid_grant", "redirect_uri не совпадает")
				return
			}
			if !verifyPKCE(codeVerifier, cd.CodeChallenge, cd.CodeChallengeMethod) {
				writeOAuthErr(w, 400, "invalid_grant", "Неверный code_verifier (PKCE)")
				return
			}
			codesT.Update(cd.ID, func(x *OAuthCode) { x.Used = true })
			issueToken(cd.UserID, cd.Scope)

		case "refresh_token":
			old, ok := tokensT.OneBy("refresh", strings.TrimSpace(refreshToken))
			if !ok || old.AppID != app.ID || old.Revoked || old.RefreshExpiresAt < now {
				writeOAuthErr(w, 400, "invalid_grant", "refresh_token недействителен или истёк")
				return
			}
			tokensT.Update(old.ID, func(x *OAuthToken) { x.Revoked = true })
			issueToken(old.UserID, old.Scope)

		default:
			writeOAuthErr(w, 400, "unsupported_grant_type", "Поддерживаются grant_type: authorization_code, refresh_token")
		}
	})

	// ── Внешний endpoint: POST /oauth/revoke
	a.Mux.HandleFunc("OPTIONS /oauth/revoke", func(w http.ResponseWriter, r *http.Request) {
		setCORS(w)
		w.WriteHeader(http.StatusNoContent)
	})
	a.Mux.HandleFunc("POST /oauth/revoke", func(w http.ResponseWriter, r *http.Request) {
		setCORS(w)
		var tokStr string
		if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
			var body map[string]string
			_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body)
			tokStr = strings.TrimSpace(body["token"])
		} else {
			_ = r.ParseForm()
			tokStr = strings.TrimSpace(r.FormValue("token"))
		}
		if tokStr != "" {
			if t, ok := tokensT.OneBy("access", tokStr); ok {
				tokensT.Update(t.ID, func(x *OAuthToken) { x.Revoked = true })
			} else if t, ok := tokensT.OneBy("refresh", tokStr); ok {
				tokensT.Update(t.ID, func(x *OAuthToken) { x.Revoked = true })
			}
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	})

	registerV1(a)
}
