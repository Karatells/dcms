package oauth

import (
	"encoding/json"
	"net/http"
	"strconv"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/mods/groups"
)

type v1HandlerFn func(method, path, needScope string, fn func(w http.ResponseWriter, r *http.Request, tok *OAuthToken, u *model.User))
type pageParamsFn func(r *http.Request, defPer int) (page, per, offset int)

func writeCoreErr(w http.ResponseWriter, err error) {
	code := 400
	errCode := "invalid_request"
	if he, ok := err.(*core.HTTPError); ok {
		code = he.Code
		switch code {
		case 401:
			errCode = "unauthorized"
		case 403:
			errCode = "forbidden"
		case 404:
			errCode = "not_found"
		}
	}
	writeOAuthErr(w, code, errCode, err.Error())
}

func pagedJSON(w http.ResponseWriter, items any, total, page, per int) {
	pages := (total + per - 1) / per
	if pages < 1 {
		pages = 1
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": total, "page": page, "per": per, "pages": pages})
}

func registerV1Groups(a *core.App, handle v1HandlerFn, pageParams pageParamsFn) {
	// 1. GET /api/v1/groups — каталог сообществ
	handle("GET", "/api/v1/groups", "groups", func(w http.ResponseWriter, r *http.Request, _ *OAuthToken, me *model.User) {
		page, per, offset := pageParams(r, 20)
		items, total := groups.APIListGroups(a, me, false, r.URL.Query().Get("q"), r.URL.Query().Get("category"), offset, per)
		pagedJSON(w, items, total, page, per)
	})

	// 2. GET /api/v1/me/groups — сообщества, в которых состоит пользователь
	handle("GET", "/api/v1/me/groups", "groups", func(w http.ResponseWriter, r *http.Request, _ *OAuthToken, me *model.User) {
		page, per, offset := pageParams(r, 20)
		items, total := groups.APIListGroups(a, me, true, r.URL.Query().Get("q"), r.URL.Query().Get("category"), offset, per)
		pagedJSON(w, items, total, page, per)
	})

	// 3. GET /api/v1/groups/{id} — информация о сообществе
	handle("GET", "/api/v1/groups/{id}", "groups", func(w http.ResponseWriter, r *http.Request, _ *OAuthToken, me *model.User) {
		gid, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		res, err := groups.APIGetGroup(a, me, gid)
		if err != nil {
			writeCoreErr(w, err)
			return
		}
		writeJSON(w, 200, res)
	})

	// 4. POST /api/v1/groups/{id}/join — вступить / подать заявку
	handle("POST", "/api/v1/groups/{id}/join", "groups_write", func(w http.ResponseWriter, r *http.Request, _ *OAuthToken, me *model.User) {
		gid, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		res, err := groups.APIJoinGroup(a, me, gid)
		if err != nil {
			writeCoreErr(w, err)
			return
		}
		writeJSON(w, 200, res)
	})

	// 5. POST /api/v1/groups/{id}/leave — выйти из сообщества
	handle("POST", "/api/v1/groups/{id}/leave", "groups_write", func(w http.ResponseWriter, r *http.Request, _ *OAuthToken, me *model.User) {
		gid, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err := groups.APILeaveGroup(me, gid); err != nil {
			writeCoreErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	})

	// 6. GET /api/v1/groups/{id}/members — участники сообщества
	handle("GET", "/api/v1/groups/{id}/members", "groups", func(w http.ResponseWriter, r *http.Request, _ *OAuthToken, me *model.User) {
		gid, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		page, per, offset := pageParams(r, 30)
		items, total, err := groups.APIListMembers(a, me, gid, offset, per)
		if err != nil {
			writeCoreErr(w, err)
			return
		}
		pagedJSON(w, items, total, page, per)
	})

	// 7. GET /api/v1/groups/{id}/blog — записи блога сообщества
	handle("GET", "/api/v1/groups/{id}/blog", "groups", func(w http.ResponseWriter, r *http.Request, _ *OAuthToken, me *model.User) {
		gid, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		page, per, offset := pageParams(r, 20)
		items, total, err := groups.APIListBlog(a, me, gid, offset, per)
		if err != nil {
			writeCoreErr(w, err)
			return
		}
		pagedJSON(w, items, total, page, per)
	})

	// 8. POST /api/v1/groups/{id}/blog — опубликовать запись в блоге сообщества
	handle("POST", "/api/v1/groups/{id}/blog", "groups_write", func(w http.ResponseWriter, r *http.Request, _ *OAuthToken, me *model.User) {
		gid, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
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
		res, err := groups.APICreateBlogPost(a, me, gid, in.Title, body)
		if err != nil {
			writeCoreErr(w, err)
			return
		}
		writeJSON(w, 201, res)
	})

	// 9. GET /api/v1/groups/{id}/subforums — подфорумы сообщества
	handle("GET", "/api/v1/groups/{id}/subforums", "groups", func(w http.ResponseWriter, r *http.Request, _ *OAuthToken, me *model.User) {
		gid, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		items, err := groups.APIListSubforums(me, gid)
		if err != nil {
			writeCoreErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"items": items})
	})

	// 10. GET /api/v1/groups/{id}/forum — темы форума сообщества
	handle("GET", "/api/v1/groups/{id}/forum", "groups", func(w http.ResponseWriter, r *http.Request, _ *OAuthToken, me *model.User) {
		gid, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		subID, _ := strconv.ParseInt(r.URL.Query().Get("sub"), 10, 64)
		page, per, offset := pageParams(r, 20)
		items, total, err := groups.APIListTopics(a, me, gid, subID, offset, per)
		if err != nil {
			writeCoreErr(w, err)
			return
		}
		pagedJSON(w, items, total, page, per)
	})

	// 11. POST /api/v1/groups/{id}/forum — создать тему на форуме сообщества
	handle("POST", "/api/v1/groups/{id}/forum", "groups_write", func(w http.ResponseWriter, r *http.Request, _ *OAuthToken, me *model.User) {
		gid, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
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
		res, err := groups.APICreateTopic(a, me, gid, in.Sub, in.Name, body)
		if err != nil {
			writeCoreErr(w, err)
			return
		}
		writeJSON(w, 201, res)
	})

	// 12. GET /api/v1/group-topics/{tid} — тема форума и ответы
	handle("GET", "/api/v1/group-topics/{tid}", "groups", func(w http.ResponseWriter, r *http.Request, _ *OAuthToken, me *model.User) {
		tid, _ := strconv.ParseInt(r.PathValue("tid"), 10, 64)
		page, per, offset := pageParams(r, 20)
		topic, posts, total, err := groups.APIGetTopic(a, me, tid, offset, per)
		if err != nil {
			writeCoreErr(w, err)
			return
		}
		pages := (total + per - 1) / per
		if pages < 1 {
			pages = 1
		}
		writeJSON(w, 200, map[string]any{"topic": topic, "items": posts, "total": total, "page": page, "per": per, "pages": pages})
	})

	// 13. POST /api/v1/group-topics/{tid}/posts — ответить в теме форума
	handle("POST", "/api/v1/group-topics/{tid}/posts", "groups_write", func(w http.ResponseWriter, r *http.Request, _ *OAuthToken, me *model.User) {
		tid, _ := strconv.ParseInt(r.PathValue("tid"), 10, 64)
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
		res, err := groups.APIReplyTopic(a, me, tid, body)
		if err != nil {
			writeCoreErr(w, err)
			return
		}
		writeJSON(w, 201, res)
	})

	// 14. GET /api/v1/groups/{id}/chat — сообщения чата сообщества
	handle("GET", "/api/v1/groups/{id}/chat", "groups", func(w http.ResponseWriter, r *http.Request, _ *OAuthToken, me *model.User) {
		gid, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		lim, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		items, err := groups.APIListChat(a, me, gid, lim)
		if err != nil {
			writeCoreErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"items": items})
	})

	// 15. POST /api/v1/groups/{id}/chat — написать в чат сообщества
	handle("POST", "/api/v1/groups/{id}/chat", "groups_write", func(w http.ResponseWriter, r *http.Request, _ *OAuthToken, me *model.User) {
		gid, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
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
		res, err := groups.APISendChat(a, me, gid, body)
		if err != nil {
			writeCoreErr(w, err)
			return
		}
		writeJSON(w, 201, res)
	})

	// 16. GET /api/v1/groups/{id}/files — файлы сообщества
	handle("GET", "/api/v1/groups/{id}/files", "groups", func(w http.ResponseWriter, r *http.Request, _ *OAuthToken, me *model.User) {
		gid, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		page, per, offset := pageParams(r, 20)
		items, total, err := groups.APIListFiles(a, me, gid, offset, per)
		if err != nil {
			writeCoreErr(w, err)
			return
		}
		pagedJSON(w, items, total, page, per)
	})

	// 17. POST /api/v1/groups/{id}/dm — написать личное сообщение сообществу
	handle("POST", "/api/v1/groups/{id}/dm", "messages_write", func(w http.ResponseWriter, r *http.Request, _ *OAuthToken, me *model.User) {
		gid, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
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
		res, err := groups.APISendUserDM(a, me, gid, body)
		if err != nil {
			writeCoreErr(w, err)
			return
		}
		writeJSON(w, 201, res)
	})

	// 18. GET /api/v1/groups/{id}/inbox — входящие диалоги сообщества (для администрации)
	handle("GET", "/api/v1/groups/{id}/inbox", "messages", func(w http.ResponseWriter, r *http.Request, _ *OAuthToken, me *model.User) {
		gid, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		items, err := groups.APIListInbox(a, me, gid)
		if err != nil {
			writeCoreErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"items": items})
	})

	// 19. POST /api/v1/groups/{id}/inbox/{uid} — ответить пользователю от имени сообщества (если он писал первым)
	handle("POST", "/api/v1/groups/{id}/inbox/{uid}", "messages_write", func(w http.ResponseWriter, r *http.Request, _ *OAuthToken, me *model.User) {
		gid, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		uid, _ := strconv.ParseInt(r.PathValue("uid"), 10, 64)
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
		res, err := groups.APIReplyInbox(a, me, gid, uid, body)
		if err != nil {
			writeCoreErr(w, err)
			return
		}
		writeJSON(w, 201, res)
	})
}
