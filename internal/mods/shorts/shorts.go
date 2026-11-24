// Package shorts — раздел коротких вертикальных видео (клипы до 60 сек с сжатием через ffmpeg).
package shorts

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

func init() {
	core.Defaults["shorts_max_mb"] = "100"
	core.Defaults["shorts_max_sec"] = "60"
	core.Defaults["shorts_crf"] = "27"
}

// Short — короткое видео (таблица shorts).
type Short struct {
	store.Base
	IDUser      int64  `json:"id_user"`
	Title       string `json:"title"`
	Opis        string `json:"opis"`
	Time        int64  `json:"time"`
	Duration    int64  `json:"duration"`
	Size        int64  `json:"size"`
	Width       int64  `json:"width"`
	Height      int64  `json:"height"`
	Views       int64  `json:"views"`
	Status      string `json:"status"` // processing | ready | failed
	ErrMsg      string `json:"err_msg"`
	Hidden      bool   `json:"hidden"`
	Blocked     bool   `json:"blocked"`
	BlockReason string `json:"block_reason"`
}

// ShortKomm — комментарий к клипу (таблица shorts_komm).
type ShortKomm struct {
	store.Base
	IDShort int64  `json:"id_short"`
	IDUser  int64  `json:"id_user"`
	Time    int64  `json:"time"`
	Msg     string `json:"msg"`
}

var (
	shortsT *store.Table[Short, *Short]
	kommsT  *store.Table[ShortKomm, *ShortKomm]
	viewed  sync.Map // "<viewerKey>|<shortID>" -> struct{}
)

var allowedExt = map[string]bool{
	"mp4": true, "mov": true, "webm": true, "mkv": true, "avi": true, "m4v": true, "3gp": true,
}

func i(n int64) string { return store.I(n) }

func canManage(a *core.App, u *model.User, s Short) bool {
	if u == nil {
		return false
	}
	return u.ID == s.IDUser || u.Level >= 2 || a.Can(u.ID, "shorts_edit") || a.Can(u.ID, "shorts_delete")
}

func canBlockShort(a *core.App, u *model.User) bool {
	return u != nil && (u.Level >= 2 || a.Can(u.ID, "shorts_block"))
}

func shortView(a *core.App, s Short, viewer *model.User) map[string]any {
	likes := a.T.Likes.By("obj", store.Key("shorts", s.ID))
	liked := false
	bookmarked := false
	if viewer != nil {
		for _, l := range likes {
			if l.IDUser == viewer.ID && l.Like {
				liked = true
				break
			}
		}
		_, bookmarked = a.T.Bookmarks.OneBy("uobj", store.Key(viewer.ID, "shorts", s.ID))
	}
	likeCnt := 0
	for _, l := range likes {
		if l.Like {
			likeCnt++
		}
	}
	m := map[string]any{
		"id":         s.ID,
		"user":       a.Ref(s.IDUser),
		"title":      s.Title,
		"opis":       s.Opis,
		"opis_html":  a.Text.Render(s.Opis),
		"time":       s.Time,
		"duration":   s.Duration,
		"size":       s.Size,
		"size_str":   core.SizeStr(s.Size),
		"width":      s.Width,
		"height":     s.Height,
		"views":      s.Views,
		"status":     s.Status,
		"err_msg":    s.ErrMsg,
		"likes":      likeCnt,
		"liked":      liked,
		"bookmarked": bookmarked,
		"komms":      kommsT.CountBy("short", i(s.ID)),
		"video_url":    "/api/shorts/" + i(s.ID) + "/video",
		"poster_url":   "/api/shorts/" + i(s.ID) + "/poster",
		"hidden":       s.Hidden,
		"blocked":      s.Blocked,
		"block_reason": s.BlockReason,
		"can_edit":     canManage(a, viewer, s),
		"can_block":    canBlockShort(a, viewer),
	}
	return m
}

func deleteShortData(a *core.App, id int64) {
	shortsT.Delete(id)
	kommsT.DeleteBy("short", i(id))
	a.T.Likes.DeleteWhere(func(l *model.LikeObject) bool { return l.Type == "shorts" && l.IDObject == id })
	a.T.Bookmarks.DeleteWhere(func(b *model.Bookmarks) bool { return b.Type == "shorts" && b.IDObject == id })
	_ = os.Remove(videoPath(a, id))
	_ = os.Remove(posterPath(a, id))
}

// Register регистрирует таблицы, хуки и маршруты раздела коротких видео.
// SearchShorts ищет готовые публичные клипы по названию или описанию (для глобального поиска).
func SearchShorts(a *core.App, q string, limit int) []map[string]any {
	if shortsT == nil {
		return nil
	}
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return nil
	}
	var res []Short
	for _, s := range shortsT.By("status", "ready") {
		if s.Hidden || s.Blocked {
			continue
		}
		if strings.Contains(strings.ToLower(s.Title), q) || strings.Contains(strings.ToLower(s.Opis), q) {
			res = append(res, s)
		}
	}
	sort.Slice(res, func(x, y int) bool { return res[x].Views > res[y].Views })
	if len(res) > limit {
		res = res[:limit]
	}
	out := make([]map[string]any, 0, len(res))
	for _, s := range res {
		out = append(out, shortView(a, s, nil))
	}
	return out
}

func Register(a *core.App) {
	shortsT = store.NewTable[Short](a.DB, "shorts").
		Index("user", func(s *Short) string { return i(s.IDUser) }).
		Index("status", func(s *Short) string { return s.Status })
	kommsT = store.NewTable[ShortKomm](a.DB, "shorts_komm").
		Index("short", func(k *ShortKomm) string { return i(k.IDShort) }).
		Index("user", func(k *ShortKomm) string { return i(k.IDUser) })

	a.Describer("shorts", func(id int64) (core.Desc, bool) {
		s, ok := shortsT.Get(id)
		if !ok {
			return core.Desc{}, false
		}
		t := s.Title
		if t == "" {
			t = "Клип #" + i(s.ID)
		}
		return core.Desc{
			Title: t,
			Text:  a.Text.Plain(s.Opis, 140),
			Link:  "/shorts/" + i(s.ID),
			Image: "/api/shorts/" + i(s.ID) + "/poster",
			Owner: s.IDUser,
		}, true
	})

	a.Counter("shorts", func(a *core.App, u *model.User) string {
		n := shortsT.CountBy("status", "ready")
		if n == 0 {
			return ""
		}
		return store.I(int64(n))
	})

	a.ProfileStat(func(a *core.App, viewer, target *model.User) []core.ProfileItem {
		var n int64
		for _, s := range shortsT.By("user", i(target.ID)) {
			if s.Status == "ready" {
				n++
			}
		}
		return []core.ProfileItem{{Key: "shorts", Title: "Клипы", Icon: "video", Link: "/shorts/user/" + i(target.ID), Count: n}}
	})

	a.OnUserDelete(func(uid int64) {
		for _, s := range shortsT.By("user", i(uid)) {
			deleteShortData(a, s.ID)
		}
		kommsT.DeleteBy("user", i(uid))
	})

	// ── Лента клипов (new | popular | friends | user)
	a.GET("/api/shorts", func(c *core.Ctx) error {
		tab := c.Q("tab")
		if tab == "" {
			tab = "new"
		}
		uid := c.QInt("user")
		startID := c.QInt("start")
		me := c.User()
		p := c.Page(12)

		var list []Short
		var pending []map[string]any
		isMod := canBlockShort(a, me)
		if uid > 0 {
			for _, s := range shortsT.By("user", i(uid)) {
				if s.Status == "ready" {
					if (s.Hidden || s.Blocked) && !isMod && (me == nil || me.ID != uid) {
						continue
					}
					list = append(list, s)
				} else if me != nil && me.ID == uid {
					pv := shortView(a, s, me)
					if s.Status == "processing" {
						st, prog, qPos, qTot := shortProcInfo(s.ID)
						pv["proc_state"] = st
						pv["progress"] = prog
						pv["queue_pos"] = qPos
						pv["queue_total"] = qTot
					}
					pending = append(pending, pv)
				}
			}
		} else {
			for _, s := range shortsT.By("status", "ready") {
				if s.Hidden || s.Blocked {
					continue
				}
				list = append(list, s)
			}
		}

		if tab == "friends" && me != nil {
			fset := map[int64]bool{me.ID: true}
			for _, f := range a.T.Friends.By("user", i(me.ID)) {
				if f.I == 0 {
					fset[f.Frend] = true
				}
			}
			var filt []Short
			for _, s := range list {
				if fset[s.IDUser] {
					filt = append(filt, s)
				}
			}
			list = filt
		}

		if tab == "popular" {
			likeCnt := map[int64]int{}
			for _, s := range list {
				likeCnt[s.ID] = a.T.Likes.CountBy("obj", store.Key("shorts", s.ID))
			}
			sort.Slice(list, func(x, y int) bool {
				sx := int64(likeCnt[list[x].ID])*5 + list[x].Views
				sy := int64(likeCnt[list[y].ID])*5 + list[y].Views
				if sx != sy {
					return sx > sy
				}
				return list[x].Time > list[y].Time
			})
		} else {
			sort.Slice(list, func(x, y int) bool { return list[x].ID > list[y].ID })
		}

		// Если открыта прямая ссылка /shorts/:id — ставим этот клип первым
		if startID > 0 {
			idx := -1
			for k := range list {
				if list[k].ID == startID {
					idx = k
					break
				}
			}
			if idx > 0 {
				picked := list[idx]
				copy(list[1:idx+1], list[0:idx])
				list[0] = picked
			} else if idx < 0 {
				if s, ok := shortsT.Get(startID); ok && (s.Status == "ready" || (me != nil && me.ID == s.IDUser)) {
					list = append([]Short{s}, list...)
				}
			}
		}

		total := len(list)
		end := p.Offset + p.Per
		if end > total {
			end = total
		}
		var slice []Short
		if p.Offset < total {
			slice = list[p.Offset:end]
		}
		items := make([]map[string]any, 0, len(slice))
		for _, s := range slice {
			items = append(items, shortView(a, s, me))
		}

		if pending == nil {
			pending = []map[string]any{}
		}
		pages := (total + p.Per - 1) / p.Per
		if pages < 1 {
			pages = 1
		}
		resp := map[string]any{
			"items":   items,
			"total":   total,
			"page":    p.Page,
			"per":     p.Per,
			"pages":   pages,
			"pending": pending,
			"max_sec": a.Cfg.Int("shorts_max_sec"),
			"max_mb":  a.Cfg.Int("shorts_max_mb"),
		}
		if uid > 0 {
			resp["owner"] = a.Ref(uid)
		}
		return c.JSON(resp)
	})

	// ── Один клип (в т.ч. для проверки статуса обработки)
	a.GET("/api/shorts/{id}", func(c *core.Ctx) error {
		s, ok := shortsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Клип не найден")
		}
		me := c.User()
		if s.Status != "ready" && !canManage(a, me, s) {
			return core.ErrNotFound("Клип ещё обрабатывается")
		}
		return c.JSON(shortView(a, s, me))
	})

	// ── Загрузка клипа (до 60 сек)
	a.POST("/api/shorts", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if b, ok := a.ActiveBan(me.ID, "all"); ok {
			return core.ErrForbidden("Вы заблокированы: " + b.Prich)
		}
		maxMB := int64(a.Cfg.Int("shorts_max_mb"))
		if maxMB <= 0 {
			maxMB = 100
		}
		up, err := c.FormFile("file", maxMB<<20)
		if err != nil {
			return err
		}
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(up.Name), "."))
		if !allowedExt[ext] {
			return core.ErrBad("Неподдерживаемый формат видео (допустимы: mp4, mov, webm, mkv, avi, 3gp)")
		}
		title := a.Text.Clean(strings.TrimSpace(c.FormValue("title")))
		if title == "" {
			title = strings.TrimSuffix(up.Name, filepath.Ext(up.Name))
			title = a.Text.Clean(strings.TrimSpace(title))
		}
		if utf8.RuneCountInString(title) > 100 {
			title = core.Trunc(title, 100)
		}
		if title == "" {
			title = "Короткое видео"
		}
		opis := a.Text.Clean(strings.TrimSpace(c.FormValue("opis")))
		if utf8.RuneCountInString(opis) > 500 {
			return core.ErrBad("Описание слишком длинное (максимум 500 символов)")
		}

		sh := Short{
			IDUser: me.ID,
			Title:  title,
			Opis:   opis,
			Time:   core.Now(),
			Status: "processing",
		}
		id := shortsT.Insert(&sh)
		raw := rawPath(a, id, ext)
		if err := core.WriteFile(raw, up.Data); err != nil {
			shortsT.Delete(id)
			return core.Err(500, "Не удалось сохранить файл")
		}

		// Быстрая проверка длительности до постановки в очередь обработки
		maxSec := a.Cfg.Int("shorts_max_sec")
		if maxSec <= 0 {
			maxSec = 60
		}
		var rawDur float64
		if info, err := probeVideo(raw); err == nil {
			rawDur = info.Duration
			if info.Duration > float64(maxSec)+2 {
				_ = os.Remove(raw)
				shortsT.Delete(id)
				return core.ErrBad("Видео слишком длинное — допустимо до " + store.I(int64(maxSec)) + " секунд")
			}
		}

		qPos, qTot := enqueueShort(a, id, me.ID, raw, rawDur)
		return c.OK("id", id, "status", "processing", "queue_pos", qPos, "queue_total", qTot)
	}, core.Auth)

	// ── Редактирование описания/названия
	a.PUT("/api/shorts/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		s, ok := shortsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Клип не найден")
		}
		if !canManage(a, me, s) {
			return core.ErrForbidden()
		}
		var in struct {
			Title string `json:"title"`
			Opis  string `json:"opis"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		title := a.Text.Clean(strings.TrimSpace(in.Title))
		if title == "" || utf8.RuneCountInString(title) > 100 {
			return core.ErrBad("Укажите название (1–100 символов)")
		}
		opis := a.Text.Clean(strings.TrimSpace(in.Opis))
		if utf8.RuneCountInString(opis) > 500 {
			return core.ErrBad("Описание слишком длинное (максимум 500 символов)")
		}
		upd, _ := shortsT.Update(s.ID, func(x *Short) {
			x.Title = title
			x.Opis = opis
		})
		return c.JSON(shortView(a, upd, me))
	}, core.Auth)

	// ── Удаление клипа
	a.DELETE("/api/shorts/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		s, ok := shortsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Клип не найден")
		}
		if !canManage(a, me, s) {
			return core.ErrForbidden()
		}
		deleteShortData(a, s.ID)
		if me.ID != s.IDUser {
			a.AdminLog(me.ID, "Клипы", "Удаление", "Удалён клип #"+i(s.ID)+" ("+s.Title+")")
		}
		return c.OK()
	}, core.Auth)

	// ── Стриминг видео (MP4, поддерживает Range-запросы)
	a.GET("/api/shorts/{id}/video", func(c *core.Ctx) error {
		s, ok := shortsT.Get(c.PathInt("id"))
		if !ok || s.Status != "ready" {
			return core.ErrNotFound()
		}
		if (s.Hidden || s.Blocked) && !canBlockShort(a, c.User()) && (c.User() == nil || c.User().ID != s.IDUser) {
			return core.ErrForbidden("Клип скрыт или заблокирован модератором")
		}
		c.W.Header().Set("Cache-Control", "public, max-age=604800")
		return c.ServeFile(videoPath(a, s.ID), "", "video/mp4")
	})

	// ── Модерация клипа (скрыть / заблокировать)
	a.POST("/api/shorts/{id}/moderate", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if !canBlockShort(a, me) {
			return core.ErrForbidden()
		}
		s, ok := shortsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Клип не найден")
		}
		var in struct {
			Hidden  *bool  `json:"hidden"`
			Blocked *bool  `json:"blocked"`
			Reason  string `json:"reason"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		upd, _ := shortsT.Update(s.ID, func(x *Short) {
			if in.Hidden != nil {
				x.Hidden = *in.Hidden
			}
			if in.Blocked != nil {
				x.Blocked = *in.Blocked
				if x.Blocked {
					x.BlockReason = a.Text.Clean(strings.TrimSpace(in.Reason))
				} else {
					x.BlockReason = ""
				}
			}
		})
		act := "Модерация"
		if in.Blocked != nil {
			if *in.Blocked {
				act = "Блокировка"
			} else {
				act = "Разблокировка"
			}
		} else if in.Hidden != nil {
			if *in.Hidden {
				act = "Скрытие"
			} else {
				act = "Показ"
			}
		}
		a.AdminLog(me.ID, "Клипы", act, "Клип #"+i(s.ID)+" ("+s.Title+")")
		return c.JSON(shortView(a, upd, me))
	}, core.Auth)

	// ── Админ-список клипов для модераторов клипов
	a.GET("/api/admin/shorts", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if !canBlockShort(a, me) && !a.Can(me.ID, "shorts_delete") {
			return core.ErrForbidden()
		}
		filter := c.Q("filter") // all | hidden | blocked
		q := strings.ToLower(strings.TrimSpace(c.Q("q")))
		p := c.Page(20)
		all := shortsT.All()
		var filt []Short
		for _, s := range all {
			if filter == "hidden" && !s.Hidden {
				continue
			}
			if filter == "blocked" && !s.Blocked {
				continue
			}
			if q != "" && !strings.Contains(strings.ToLower(s.Title), q) && !strings.Contains(strings.ToLower(s.Opis), q) {
				continue
			}
			filt = append(filt, s)
		}
		sort.Slice(filt, func(x, y int) bool { return filt[x].ID > filt[y].ID })
		total := len(filt)
		end := p.Offset + p.Per
		if end > total {
			end = total
		}
		var sl []Short
		if p.Offset < total {
			sl = filt[p.Offset:end]
		}
		items := make([]map[string]any, 0, len(sl))
		for _, s := range sl {
			items = append(items, shortView(a, s, me))
		}
		return c.List(items, total, p)
	}, core.Auth)

	// ── Обложка (JPEG)
	a.GET("/api/shorts/{id}/poster", func(c *core.Ctx) error {
		s, ok := shortsT.Get(c.PathInt("id"))
		if !ok || s.Status != "ready" {
			return core.ErrNotFound()
		}
		c.W.Header().Set("Cache-Control", "public, max-age=604800")
		return c.ServeFile(posterPath(a, s.ID), "", "image/jpeg")
	})

	// ── Учёт просмотра
	a.POST("/api/shorts/{id}/view", func(c *core.Ctx) error {
		id := c.PathInt("id")
		s, ok := shortsT.Get(id)
		if !ok || s.Status != "ready" {
			return core.ErrNotFound()
		}
		vkey := c.IP()
		if u := c.User(); u != nil {
			vkey = "u" + i(u.ID)
		}
		if _, dup := viewed.LoadOrStore(vkey+"|"+i(id), struct{}{}); !dup {
			s, _ = shortsT.Update(id, func(x *Short) { x.Views++ })
		}
		return c.OK("views", s.Views)
	})

	// ── Лайк / снятие лайка
	a.POST("/api/shorts/{id}/like", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		id := c.PathInt("id")
		s, ok := shortsT.Get(id)
		if !ok || s.Status != "ready" {
			return core.ErrNotFound("Клип не найден")
		}
		key := store.Key(me.ID, "shorts", id)
		liked := false
		if ex, ok := a.T.Likes.OneBy("uobj", key); ok {
			a.T.Likes.Delete(ex.ID)
		} else {
			a.T.Likes.Insert(&model.LikeObject{IDUser: me.ID, Type: "shorts", IDObject: id, Like: true})
			liked = true
			if s.IDUser != me.ID {
				a.AddRatingTmp(s.IDUser, 1)
				a.Notify(me.ID, s.IDUser, "shorts_like", id, true)
			}
		}
		cnt := 0
		for _, l := range a.T.Likes.By("obj", store.Key("shorts", id)) {
			if l.Like {
				cnt++
			}
		}
		return c.OK("liked", liked, "likes", cnt)
	}, core.Auth)

	// ── Комментарии
	a.GET("/api/shorts/{id}/komms", func(c *core.Ctx) error {
		id := c.PathInt("id")
		s, ok := shortsT.Get(id)
		if !ok {
			return core.ErrNotFound("Клип не найден")
		}
		me := c.User()
		p := c.Page(20)
		list, total := kommsT.Select(store.Opts[ShortKomm]{Index: "short", Key: i(id), Desc: true, Offset: p.Offset, Limit: p.Per})
		items := make([]map[string]any, 0, len(list))
		for _, k := range list {
			items = append(items, map[string]any{
				"id":         k.ID,
				"user":       a.Ref(k.IDUser),
				"time":       k.Time,
				"msg":        a.Text.Render(k.Msg),
				"raw":        k.Msg,
				"can_delete": me != nil && (me.ID == k.IDUser || canManage(a, me, s)),
				"can_reply":  me != nil && me.ID != k.IDUser,
			})
		}
		return c.List(items, total, p)
	})

	a.POST("/api/shorts/{id}/komms", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if b, ok := a.ActiveBan(me.ID, "all"); ok {
			return core.ErrForbidden("Вы заблокированы: " + b.Prich)
		}
		id := c.PathInt("id")
		s, ok := shortsT.Get(id)
		if !ok || s.Status != "ready" {
			return core.ErrNotFound("Клип не найден")
		}
		var in struct {
			Msg     string `json:"msg"`
			ReplyTo int64  `json:"reply_to"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		msg := a.Text.Clean(strings.TrimSpace(in.Msg))
		if n := utf8.RuneCountInString(msg); n < 1 {
			return core.ErrBad("Введите текст комментария")
		} else if n > 1024 {
			return core.ErrBad("Комментарий слишком длинный (максимум 1024 символа)")
		}
		kid := kommsT.Insert(&ShortKomm{IDShort: id, IDUser: me.ID, Time: core.Now(), Msg: msg})
		a.AddBalls(me.ID, 1, 1)
		a.Discuss(s.IDUser, me.ID, core.KObmen, "shorts", id)
		if in.ReplyTo > 0 && in.ReplyTo != me.ID && a.T.Users.Has(in.ReplyTo) {
			a.Notify(me.ID, in.ReplyTo, "shorts_komm", id, true)
		} else if s.IDUser != me.ID {
			a.Notify(me.ID, s.IDUser, "shorts_komm", id, true)
		}
		return c.OK("id", kid, "komms", kommsT.CountBy("short", i(id)))
	}, core.Auth)

	a.DELETE("/api/shorts/komm/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		k, ok := kommsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		s, _ := shortsT.Get(k.IDShort)
		if !(me.ID == k.IDUser || canManage(a, me, s)) {
			return core.ErrForbidden()
		}
		kommsT.Delete(k.ID)
		return c.OK()
	}, core.Auth)

	// ── Жалоба на клип
	a.POST("/api/shorts/{id}/spam", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		s, ok := shortsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		var in struct {
			Msg string `json:"msg"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		msg := a.Text.Clean(strings.TrimSpace(in.Msg))
		if msg == "" {
			return core.ErrBad("Опишите нарушение")
		}
		a.ReportSpam(me.ID, s.IDUser, "shorts", s.ID, s.ID, s.Title, msg)
		return c.OK()
	}, core.Auth)

	// ── Список друзей для быстрого «Поделиться в ЛС»
	a.GET("/api/shorts/share-targets", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		seen := map[int64]bool{me.ID: true}
		var out []model.UserRef
		for _, f := range a.T.Friends.By("user", i(me.ID)) {
			if f.I == 0 && !seen[f.Frend] {
				seen[f.Frend] = true
				out = append(out, a.Ref(f.Frend))
			}
		}
		// Если друзей мало — добавим последних пользователей сайта (кроме себя)
		if len(out) < 10 {
			us, _ := a.T.Users.Select(store.Opts[model.User]{Desc: true, Limit: 15})
			for _, u := range us {
				if !seen[u.ID] {
					seen[u.ID] = true
					out = append(out, a.Ref(u.ID))
					if len(out) >= 15 {
						break
					}
				}
			}
		}
		if out == nil {
			out = []model.UserRef{}
		}
		return c.JSON(map[string]any{"users": out})
	}, core.Auth)
}
