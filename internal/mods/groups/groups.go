// Package groups — модуль «Сообщества / Группы» с собственным блогом, форумом, чатом, файлами и ролевой моделью.
package groups

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

// Group — сообщество (таблица groups).
type Group struct {
	store.Base
	IDCreator   int64  `json:"id_creator"`
	Name        string `json:"name"`
	Opis        string `json:"opis"`
	Category    string `json:"category"`
	Time        int64  `json:"time"`
	AvatarExt   string `json:"avatar_ext"`
	BannerExt   string `json:"banner_ext"`
	EnableBlog  bool   `json:"enable_blog"`
	EnableForum bool   `json:"enable_forum"`
	EnableChat  bool   `json:"enable_chat"`
	EnableFiles bool   `json:"enable_files"`
	OpenBlog    bool   `json:"open_blog"` // могут ли обычные участники публиковать посты в блог
	Private     bool   `json:"private"`   // приватное сообщество (вступление по одобрению или ссылке)
	Hidden      bool   `json:"hidden"`
	Blocked     bool   `json:"blocked"`
	BlockReason string `json:"block_reason"`
}

// GroupMember — участник сообщества и его роль / права модератора по разделам.
type GroupMember struct {
	store.Base
	IDGroup  int64  `json:"id_group"`
	IDUser   int64  `json:"id_user"`
	Role     string `json:"role"` // creator | admin | member
	ModBlog  bool   `json:"mod_blog"`
	ModForum bool   `json:"mod_forum"`
	ModChat  bool   `json:"mod_chat"`
	ModFiles bool   `json:"mod_files"`
	Time     int64  `json:"time"`
}

// GroupPost — запись в блоге сообщества.
type GroupPost struct {
	store.Base
	IDGroup int64  `json:"id_group"`
	IDUser  int64  `json:"id_user"`
	Title   string `json:"title"`
	Msg     string `json:"msg"`
	Time    int64  `json:"time"`
	Pinned  bool   `json:"pinned"`
}

// GroupPostKomm — комментарий к записи в блоге сообщества.
type GroupPostKomm struct {
	store.Base
	IDPost  int64  `json:"id_post"`
	IDGroup int64  `json:"id_group"`
	IDUser  int64  `json:"id_user"`
	Time    int64  `json:"time"`
	Msg     string `json:"msg"`
}

// GroupSubforum — подфорум (раздел) внутри форума сообщества.
type GroupSubforum struct {
	store.Base
	IDGroup int64  `json:"id_group"`
	Name    string `json:"name"`
	Opis    string `json:"opis"`
	Pos     int64  `json:"pos"`
	Time    int64  `json:"time"`
}

// GroupTopic — тема на форуме сообщества.
type GroupTopic struct {
	store.Base
	IDGroup  int64  `json:"id_group"`
	IDSub    int64  `json:"id_sub"`
	IDUser   int64  `json:"id_user"`
	Name     string `json:"name"`
	Msg      string `json:"msg"`
	Time     int64  `json:"time"`
	TimeLast int64  `json:"time_last"`
	Closed   bool   `json:"closed"`
	Pinned   bool   `json:"pinned"`
}

// GroupTopicPost — ответ в теме форума сообщества.
type GroupTopicPost struct {
	store.Base
	IDTopic int64  `json:"id_topic"`
	IDGroup int64  `json:"id_group"`
	IDUser  int64  `json:"id_user"`
	Time    int64  `json:"time"`
	Msg     string `json:"msg"`
}

// GroupChat — сообщение в чате сообщества.
type GroupChat struct {
	store.Base
	IDGroup int64  `json:"id_group"`
	IDUser  int64  `json:"id_user"`
	Time    int64  `json:"time"`
	Msg     string `json:"msg"`
}

// GroupFile — файл сообщества.
type GroupFile struct {
	store.Base
	IDGroup int64  `json:"id_group"`
	IDUser  int64  `json:"id_user"`
	Name    string `json:"name"`
	Ras     string `json:"ras"`
	Type    string `json:"type"`
	Size    int64  `json:"size"`
	Opis    string `json:"opis"`
	Time    int64  `json:"time"`
	Loads   int64  `json:"loads"`
}

var (
	groupsT  *store.Table[Group, *Group]
	membersT *store.Table[GroupMember, *GroupMember]
	blogT    *store.Table[GroupPost, *GroupPost]
	blogKT   *store.Table[GroupPostKomm, *GroupPostKomm]
	subsT    *store.Table[GroupSubforum, *GroupSubforum]
	topicsT  *store.Table[GroupTopic, *GroupTopic]
	tpostsT  *store.Table[GroupTopicPost, *GroupTopicPost]
	chatT    *store.Table[GroupChat, *GroupChat]
	filesT   *store.Table[GroupFile, *GroupFile]
)

func i(n int64) string { return store.I(n) }

func memberOf(gid, uid int64) (GroupMember, bool) {
	if uid == 0 {
		return GroupMember{}, false
	}
	return membersT.OneBy("pair", store.Key(gid, uid))
}

type perms struct {
	IsMember       bool   `json:"is_member"`
	IsCreator      bool   `json:"is_creator"`
	IsAdmin        bool   `json:"is_admin"`
	Manage         bool   `json:"manage"` // creator | admin | site group mod
	CanBlock       bool   `json:"can_block"`
	CanDeleteGroup bool   `json:"can_delete_group"`
	ModBlog        bool   `json:"mod_blog"`
	ModForum       bool   `json:"mod_forum"`
	ModChat        bool   `json:"mod_chat"`
	ModFiles       bool   `json:"mod_files"`
	Role           string `json:"role"`
}

var appRef *core.App

func canBlockGroup(u *model.User) bool {
	return u != nil && (u.Level >= 3 || (appRef != nil && appRef.Can(u.ID, "groups_block")))
}

func canDeleteGroup(u *model.User, g Group) bool {
	return u != nil && (g.IDCreator == u.ID || u.Level >= 3 || (appRef != nil && appRef.Can(u.ID, "groups_delete")))
}

func permsOf(u *model.User, g Group) perms {
	if u == nil {
		return perms{}
	}
	siteAdm := u.Level >= 3 || (appRef != nil && appRef.Can(u.ID, "groups_edit"))
	m, ok := memberOf(g.ID, u.ID)
	isCreator := g.IDCreator == u.ID || (ok && m.Role == "creator")
	isAdmin := ok && m.Role == "admin"
	manage := isCreator || isAdmin || siteAdm
	role := ""
	if ok {
		role = m.Role
	}
	if isCreator {
		role = "creator"
	}
	return perms{
		IsMember:       ok || isCreator,
		IsCreator:      isCreator,
		IsAdmin:        isAdmin,
		Manage:         manage,
		CanBlock:       canBlockGroup(u),
		CanDeleteGroup: canDeleteGroup(u, g),
		ModBlog:        manage || (ok && m.ModBlog),
		ModForum:       manage || (ok && m.ModForum),
		ModChat:        manage || (ok && m.ModChat),
		ModFiles:       manage || (ok && m.ModFiles),
		Role:           role,
	}
}

func avatarPath(a *core.App, id int64, ext string) string {
	return filepath.Join(a.UploadDir("groups"), fmt.Sprintf("ava_%d.%s", id, ext))
}
func bannerPath(a *core.App, id int64, ext string) string {
	return filepath.Join(a.UploadDir("groups"), fmt.Sprintf("ban_%d.%s", id, ext))
}
func gfilePath(a *core.App, id int64) string {
	return filepath.Join(a.UploadDir("groups", "files"), fmt.Sprintf("%d.dat", id))
}

func groupCard(a *core.App, g Group, viewer *model.User) map[string]any {
	p := permsOf(viewer, g)
	ava := ""
	if g.AvatarExt != "" {
		ava = "/api/groups/" + i(g.ID) + "/avatar?v=" + i(g.Time)
	}
	ban := ""
	if g.BannerExt != "" {
		ban = "/api/groups/" + i(g.ID) + "/banner?v=" + i(g.Time)
	}
	pendingReq := false
	if viewer != nil && !p.IsMember {
		_, pendingReq = requestsT.OneBy("pair", store.Key(g.ID, viewer.ID))
	}
	reqCnt := 0
	if p.Manage {
		reqCnt = requestsT.CountBy("group", i(g.ID))
	}
	return map[string]any{
		"id":              g.ID,
		"name":            g.Name,
		"opis":            g.Opis,
		"opis_html":       a.Text.Render(g.Opis),
		"category":        g.Category,
		"time":            g.Time,
		"creator":         a.Ref(g.IDCreator),
		"avatar":          ava,
		"banner":          ban,
		"members":         membersT.CountBy("group", i(g.ID)),
		"enable_blog":     g.EnableBlog,
		"enable_forum":    g.EnableForum,
		"enable_chat":     g.EnableChat,
		"enable_files":    g.EnableFiles,
		"open_blog":       g.OpenBlog,
		"private":         g.Private,
		"hidden":          g.Hidden,
		"blocked":         g.Blocked,
		"block_reason":    g.BlockReason,
		"can_view":        canViewGroup(viewer, g),
		"pending_request": pendingReq,
		"requests_count":  reqCnt,
		"perms":           p,
	}
}

func deleteGroupAll(a *core.App, g Group) {
	if g.AvatarExt != "" {
		_ = os.Remove(avatarPath(a, g.ID, g.AvatarExt))
	}
	if g.BannerExt != "" {
		_ = os.Remove(bannerPath(a, g.ID, g.BannerExt))
	}
	for _, f := range filesT.By("group", i(g.ID)) {
		_ = os.Remove(gfilePath(a, f.ID))
	}
	filesT.DeleteBy("group", i(g.ID))
	for _, p := range blogT.By("group", i(g.ID)) {
		a.T.Likes.DeleteWhere(func(l *model.LikeObject) bool { return l.Type == "gpost" && l.IDObject == p.ID })
	}
	blogT.DeleteBy("group", i(g.ID))
	blogKT.DeleteBy("group", i(g.ID))
	subsT.DeleteBy("group", i(g.ID))
	topicsT.DeleteBy("group", i(g.ID))
	tpostsT.DeleteBy("group", i(g.ID))
	chatT.DeleteBy("group", i(g.ID))
	membersT.DeleteBy("group", i(g.ID))
	requestsT.DeleteBy("group", i(g.ID))
	linksT.DeleteBy("group", i(g.ID))
	invitesT.DeleteBy("group", i(g.ID))
	groupsT.Delete(g.ID)
}

// Register регистрирует таблицы, хуки и маршруты модуля сообществ.
func Register(a *core.App) {
	appRef = a
	groupsT = store.NewTable[Group](a.DB, "groups").
		Index("creator", func(g *Group) string { return i(g.IDCreator) })
	membersT = store.NewTable[GroupMember](a.DB, "group_members").
		Index("group", func(m *GroupMember) string { return i(m.IDGroup) }).
		Index("user", func(m *GroupMember) string { return i(m.IDUser) }).
		Index("pair", func(m *GroupMember) string { return store.Key(m.IDGroup, m.IDUser) })
	blogT = store.NewTable[GroupPost](a.DB, "group_blog").
		Index("group", func(p *GroupPost) string { return i(p.IDGroup) })
	blogKT = store.NewTable[GroupPostKomm](a.DB, "group_blog_komm").
		Index("post", func(k *GroupPostKomm) string { return i(k.IDPost) }).
		Index("group", func(k *GroupPostKomm) string { return i(k.IDGroup) })
	subsT = store.NewTable[GroupSubforum](a.DB, "group_forum_r").
		Index("group", func(s *GroupSubforum) string { return i(s.IDGroup) })
	topicsT = store.NewTable[GroupTopic](a.DB, "group_forum_t").
		Index("group", func(t *GroupTopic) string { return i(t.IDGroup) }).
		Index("sub", func(t *GroupTopic) string { return i(t.IDSub) })
	tpostsT = store.NewTable[GroupTopicPost](a.DB, "group_forum_p").
		Index("topic", func(p *GroupTopicPost) string { return i(p.IDTopic) }).
		Index("group", func(p *GroupTopicPost) string { return i(p.IDGroup) })
	chatT = store.NewTable[GroupChat](a.DB, "group_chat").
		Index("group", func(c *GroupChat) string { return i(c.IDGroup) })
	filesT = store.NewTable[GroupFile](a.DB, "group_files").
		Index("group", func(f *GroupFile) string { return i(f.IDGroup) })

	a.Describer("group", func(id int64) (core.Desc, bool) {
		g, ok := groupsT.Get(id)
		if !ok {
			return core.Desc{}, false
		}
		img := ""
		if g.AvatarExt != "" {
			img = "/api/groups/" + i(g.ID) + "/avatar"
		}
		return core.Desc{Title: g.Name, Text: a.Text.Plain(g.Opis, 140), Link: "/groups/" + i(g.ID), Image: img, Owner: g.IDCreator}, true
	})

	a.Counter("groups", func(a *core.App, u *model.User) string {
		n := groupsT.Len()
		if n == 0 {
			return ""
		}
		return store.I(int64(n))
	})

	a.ProfileStat(func(a *core.App, viewer, target *model.User) []core.ProfileItem {
		n := int64(membersT.CountBy("user", i(target.ID)))
		return []core.ProfileItem{{Key: "groups", Title: "Сообщества", Icon: "users", Link: "/groups?user=" + i(target.ID), Count: n}}
	})

	a.OnUserDelete(func(uid int64) {
		for _, g := range groupsT.By("creator", i(uid)) {
			deleteGroupAll(a, g)
		}
		membersT.DeleteBy("user", i(uid))
		requestsT.DeleteBy("user", i(uid))
		invitesT.DeleteBy("to", i(uid))
	})

	// ── Список сообществ (все / мои / по пользователю / поиск)
	a.GET("/api/groups", func(c *core.Ctx) error {
		tab := c.Q("tab")
		q := strings.ToLower(strings.TrimSpace(c.Q("q")))
		cat := strings.TrimSpace(c.Q("category"))
		uid := c.QInt("user")
		me := c.User()
		p := c.Page(15)

		var list []Group
		if uid > 0 {
			for _, m := range membersT.By("user", i(uid)) {
				if g, ok := groupsT.Get(m.IDGroup); ok {
					list = append(list, g)
				}
			}
		} else if tab == "my" && me != nil {
			for _, m := range membersT.By("user", i(me.ID)) {
				if g, ok := groupsT.Get(m.IDGroup); ok {
					list = append(list, g)
				}
			}
		} else {
			list = groupsT.All()
		}

		var filt []Group
		for _, g := range list {
			if (g.Hidden || g.Blocked) && tab != "my" && uid == 0 && !canBlockGroup(me) {
				continue
			}
			if cat != "" && g.Category != cat {
				continue
			}
			if q != "" && !strings.Contains(strings.ToLower(g.Name), q) && !strings.Contains(strings.ToLower(g.Opis), q) {
				continue
			}
			filt = append(filt, g)
		}
		sort.Slice(filt, func(x, y int) bool {
			mx, my := membersT.CountBy("group", i(filt[x].ID)), membersT.CountBy("group", i(filt[y].ID))
			if mx != my {
				return mx > my
			}
			return filt[x].ID > filt[y].ID
		})

		total := len(filt)
		end := p.Offset + p.Per
		if end > total {
			end = total
		}
		var sl []Group
		if p.Offset < total {
			sl = filt[p.Offset:end]
		}
		items := make([]map[string]any, 0, len(sl))
		for _, g := range sl {
			items = append(items, groupCard(a, g, me))
		}
		return c.List(items, total, p)
	})

	// ── Создать сообщество
	a.POST("/api/groups", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if b, ok := a.ActiveBan(me.ID, "all"); ok {
			return core.ErrForbidden("Вы заблокированы: " + b.Prich)
		}
		var in struct {
			Name        string `json:"name"`
			Opis        string `json:"opis"`
			Category    string `json:"category"`
			EnableBlog  *bool  `json:"enable_blog"`
			EnableForum *bool  `json:"enable_forum"`
			EnableChat  *bool  `json:"enable_chat"`
			EnableFiles *bool  `json:"enable_files"`
			Private     bool   `json:"private"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		name := a.Text.Clean(strings.TrimSpace(in.Name))
		if n := utf8.RuneCountInString(name); n < 2 || n > 64 {
			return core.ErrBad("Название сообщества: от 2 до 64 символов")
		}
		opis := a.Text.Clean(strings.TrimSpace(in.Opis))
		if utf8.RuneCountInString(opis) > 1000 {
			return core.ErrBad("Описание слишком длинное (максимум 1000 символов)")
		}
		cat := strings.TrimSpace(in.Category)
		if cat == "" {
			cat = "Общение"
		}
		bVal := func(p *bool) bool { return p == nil || *p }
		now := core.Now()
		g := Group{
			IDCreator:   me.ID,
			Name:        name,
			Opis:        opis,
			Category:    cat,
			Time:        now,
			EnableBlog:  bVal(in.EnableBlog),
			EnableForum: bVal(in.EnableForum),
			EnableChat:  bVal(in.EnableChat),
			EnableFiles: bVal(in.EnableFiles),
			Private:     in.Private,
		}
		id := groupsT.Insert(&g)
		membersT.Insert(&GroupMember{IDGroup: id, IDUser: me.ID, Role: "creator", Time: now})
		a.AddBalls(me.ID, 5, 5)
		return c.OK("id", id)
	}, core.Auth)

	// ── Карточка сообщества
	a.GET("/api/groups/{id}", func(c *core.Ctx) error {
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Сообщество не найдено")
		}
		out := groupCard(a, g, c.User())
		out["counts"] = map[string]int{
			"blog":  blogT.CountBy("group", i(g.ID)),
			"forum": topicsT.CountBy("group", i(g.ID)),
			"chat":  chatT.CountBy("group", i(g.ID)),
			"files": filesT.CountBy("group", i(g.ID)),
		}
		return c.JSON(out)
	})

	// ── Обновить настройки сообщества (создатель или администратор)
	a.PUT("/api/groups/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Сообщество не найдено")
		}
		if !permsOf(me, g).Manage {
			return core.ErrForbidden("Недостаточно прав")
		}
		var in struct {
			Name        string `json:"name"`
			Opis        string `json:"opis"`
			Category    string `json:"category"`
			EnableBlog  bool   `json:"enable_blog"`
			EnableForum bool   `json:"enable_forum"`
			EnableChat  bool   `json:"enable_chat"`
			EnableFiles bool   `json:"enable_files"`
			OpenBlog    bool   `json:"open_blog"`
			Private     bool   `json:"private"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		name := a.Text.Clean(strings.TrimSpace(in.Name))
		if n := utf8.RuneCountInString(name); n < 2 || n > 64 {
			return core.ErrBad("Название сообщества: от 2 до 64 символов")
		}
		opis := a.Text.Clean(strings.TrimSpace(in.Opis))
		if utf8.RuneCountInString(opis) > 1000 {
			return core.ErrBad("Описание слишком длинное (максимум 1000 символов)")
		}
		cat := strings.TrimSpace(in.Category)
		if cat == "" {
			cat = "Общение"
		}
		upd, _ := groupsT.Update(g.ID, func(x *Group) {
			x.Name, x.Opis, x.Category = name, opis, cat
			x.EnableBlog, x.EnableForum, x.EnableChat, x.EnableFiles, x.OpenBlog, x.Private = in.EnableBlog, in.EnableForum, in.EnableChat, in.EnableFiles, in.OpenBlog, in.Private
		})
		return c.JSON(groupCard(a, upd, me))
	}, core.Auth)

	// ── Удалить сообщество (только создатель или админ сайта)
	a.DELETE("/api/groups/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Сообщество не найдено")
		}
		if !canDeleteGroup(me, g) {
			return core.ErrForbidden("Удалить сообщество может только создатель или модератор сообществ")
		}
		deleteGroupAll(a, g)
		if me.ID != g.IDCreator {
			a.AdminLog(me.ID, "Сообщества", "Удаление", "Удалено сообщество #"+i(g.ID)+" ("+g.Name+")")
		}
		return c.OK()
	}, core.Auth)

	// ── Модерация сообщества (скрыть / заблокировать)
	a.POST("/api/groups/{id}/moderate", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if !canBlockGroup(me) {
			return core.ErrForbidden()
		}
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Сообщество не найдено")
		}
		var in struct {
			Hidden  *bool  `json:"hidden"`
			Blocked *bool  `json:"blocked"`
			Reason  string `json:"reason"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		upd, _ := groupsT.Update(g.ID, func(x *Group) {
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
		a.AdminLog(me.ID, "Сообщества", act, "Сообщество #"+i(g.ID)+" ("+g.Name+")")
		return c.JSON(groupCard(a, upd, me))
	}, core.Auth)

	// ── Админ-список сообществ для модераторов сообществ
	a.GET("/api/admin/communities", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if !canBlockGroup(me) && !a.Can(me.ID, "groups_delete") {
			return core.ErrForbidden()
		}
		filter := c.Q("filter") // all | hidden | blocked
		q := strings.ToLower(strings.TrimSpace(c.Q("q")))
		p := c.Page(20)
		all := groupsT.All()
		var filt []Group
		for _, g := range all {
			if filter == "hidden" && !g.Hidden {
				continue
			}
			if filter == "blocked" && !g.Blocked {
				continue
			}
			if q != "" && !strings.Contains(strings.ToLower(g.Name), q) && !strings.Contains(strings.ToLower(g.Opis), q) {
				continue
			}
			filt = append(filt, g)
		}
		sort.Slice(filt, func(x, y int) bool { return filt[x].ID > filt[y].ID })
		total := len(filt)
		end := p.Offset + p.Per
		if end > total {
			end = total
		}
		var sl []Group
		if p.Offset < total {
			sl = filt[p.Offset:end]
		}
		items := make([]map[string]any, 0, len(sl))
		for _, g := range sl {
			items = append(items, groupCard(a, g, me))
		}
		return c.List(items, total, p)
	}, core.Auth)

	// ── Загрузка аватарки и баннера сообщества
	uploadImg := func(kind string, w, h int) core.Handler {
		return func(c *core.Ctx) error {
			me, err := c.Need()
			if err != nil {
				return err
			}
			g, ok := groupsT.Get(c.PathInt("id"))
			if !ok {
				return core.ErrNotFound("Сообщество не найдено")
			}
			if !permsOf(me, g).Manage {
				return core.ErrForbidden()
			}
			maxMB := int64(a.Cfg.Int("max_upload_foto_mb"))
			if maxMB <= 0 {
				maxMB = 12
			}
			up, err := c.FormFile("file", maxMB<<20)
			if err != nil {
				return err
			}
			if up == nil || len(up.Data) == 0 {
				return core.ErrBad("Файл не выбран")
			}
			th, _, err := core.Thumb(up.Data, w, h, true)
			if err != nil {
				return core.ErrBad("Неподдерживаемое изображение (JPEG/PNG/GIF)")
			}
			if kind == "avatar" {
				if err := core.WriteFile(avatarPath(a, g.ID, "jpg"), th); err != nil {
					return core.Err(500, "Ошибка сохранения")
				}
				groupsT.Update(g.ID, func(x *Group) { x.AvatarExt = "jpg"; x.Time = core.Now() })
			} else {
				if err := core.WriteFile(bannerPath(a, g.ID, "jpg"), th); err != nil {
					return core.Err(500, "Ошибка сохранения")
				}
				groupsT.Update(g.ID, func(x *Group) { x.BannerExt = "jpg"; x.Time = core.Now() })
			}
			return c.OK()
		}
	}
	a.POST("/api/groups/{id}/avatar", uploadImg("avatar", 300, 300), core.Auth)
	a.POST("/api/groups/{id}/banner", uploadImg("banner", 1200, 360), core.Auth)

	a.GET("/api/groups/{id}/avatar", func(c *core.Ctx) error {
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok || g.AvatarExt == "" {
			return core.ErrNotFound()
		}
		c.W.Header().Set("Cache-Control", "public, max-age=3600")
		return c.ServeFile(avatarPath(a, g.ID, g.AvatarExt), "", "image/jpeg")
	})
	a.GET("/api/groups/{id}/banner", func(c *core.Ctx) error {
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok || g.BannerExt == "" {
			return core.ErrNotFound()
		}
		c.W.Header().Set("Cache-Control", "public, max-age=3600")
		return c.ServeFile(bannerPath(a, g.ID, g.BannerExt), "", "image/jpeg")
	})

	// ── Вступить / подать заявку / выйти
	a.POST("/api/groups/{id}/join", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Сообщество не найдено")
		}
		if _, exists := memberOf(g.ID, me.ID); exists {
			return c.OK("members", membersT.CountBy("group", i(g.ID)))
		}
		if g.Private {
			if _, hasReq := requestsT.OneBy("pair", store.Key(g.ID, me.ID)); !hasReq {
				requestsT.Insert(&GroupRequest{IDGroup: g.ID, IDUser: me.ID, Time: core.Now()})
				for _, m := range membersT.By("group", i(g.ID)) {
					if m.Role == "creator" || m.Role == "admin" {
						a.Notify(me.ID, m.IDUser, "group_req", g.ID, false)
					}
				}
			}
			return c.OK("pending", true)
		}
		membersT.Insert(&GroupMember{IDGroup: g.ID, IDUser: me.ID, Role: "member", Time: core.Now()})
		return c.OK("members", membersT.CountBy("group", i(g.ID)))
	}, core.Auth)

	a.POST("/api/groups/{id}/leave", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Сообщество не найдено")
		}
		if g.IDCreator == me.ID {
			return core.ErrBad("Создатель не может покинуть своё сообщество")
		}
		if req, ok := requestsT.OneBy("pair", store.Key(g.ID, me.ID)); ok {
			requestsT.Delete(req.ID)
		}
		if m, exists := memberOf(g.ID, me.ID); exists {
			membersT.Delete(m.ID)
		}
		return c.OK("members", membersT.CountBy("group", i(g.ID)))
	}, core.Auth)

	// ── Участники и управление ролями
	a.GET("/api/groups/{id}/members", func(c *core.Ctx) error {
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Сообщество не найдено")
		}
		if !canViewGroup(c.User(), g) {
			return core.ErrForbidden("Это закрытое сообщество")
		}
		p := c.Page(30)
		all := membersT.By("group", i(g.ID))
		roleRank := func(m GroupMember) int {
			if m.Role == "creator" {
				return 0
			}
			if m.Role == "admin" {
				return 1
			}
			if m.ModBlog || m.ModForum || m.ModChat || m.ModFiles {
				return 2
			}
			return 3
		}
		sort.Slice(all, func(x, y int) bool {
			rx, ry := roleRank(all[x]), roleRank(all[y])
			if rx != ry {
				return rx < ry
			}
			return all[x].Time < all[y].Time
		})
		total := len(all)
		end := p.Offset + p.Per
		if end > total {
			end = total
		}
		var sl []GroupMember
		if p.Offset < total {
			sl = all[p.Offset:end]
		}
		items := make([]map[string]any, 0, len(sl))
		for _, m := range sl {
			items = append(items, map[string]any{
				"user":      a.Ref(m.IDUser),
				"role":      m.Role,
				"mod_blog":  m.ModBlog,
				"mod_forum": m.ModForum,
				"mod_chat":  m.ModChat,
				"mod_files": m.ModFiles,
				"time":      m.Time,
			})
		}
		return c.List(items, total, p)
	})

	a.PUT("/api/groups/{id}/members/{uid}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Сообщество не найдено")
		}
		myP := permsOf(me, g)
		if !myP.Manage {
			return core.ErrForbidden("Недостаточно прав")
		}
		targetUID := c.PathInt("uid")
		if targetUID == g.IDCreator {
			return core.ErrBad("Нельзя менять роль создателя")
		}
		m, ok := memberOf(g.ID, targetUID)
		if !ok {
			return core.ErrNotFound("Пользователь не состоит в сообществе")
		}
		var in struct {
			Role     string `json:"role"` // admin | member
			ModBlog  bool   `json:"mod_blog"`
			ModForum bool   `json:"mod_forum"`
			ModChat  bool   `json:"mod_chat"`
			ModFiles bool   `json:"mod_files"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if in.Role != "admin" {
			in.Role = "member"
		}
		// Назначать/снимать администраторов может только создатель (или админ сайта)
		if (in.Role == "admin" || m.Role == "admin") && !myP.IsCreator && me.Level < 3 {
			return core.ErrForbidden("Назначать администраторов может только создатель сообщества")
		}
		membersT.Update(m.ID, func(x *GroupMember) {
			x.Role = in.Role
			if in.Role == "admin" {
				x.ModBlog, x.ModForum, x.ModChat, x.ModFiles = false, false, false, false
			} else {
				x.ModBlog, x.ModForum, x.ModChat, x.ModFiles = in.ModBlog, in.ModForum, in.ModChat, in.ModFiles
			}
		})
		return c.OK()
	}, core.Auth)

	a.DELETE("/api/groups/{id}/members/{uid}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		g, ok := groupsT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		myP := permsOf(me, g)
		if !myP.Manage {
			return core.ErrForbidden()
		}
		targetUID := c.PathInt("uid")
		if targetUID == g.IDCreator {
			return core.ErrBad("Нельзя исключить создателя")
		}
		m, ok := memberOf(g.ID, targetUID)
		if !ok {
			return core.ErrNotFound()
		}
		if m.Role == "admin" && !myP.IsCreator && me.Level < 3 {
			return core.ErrForbidden("Исключить администратора может только создатель")
		}
		membersT.Delete(m.ID)
		return c.OK()
	}, core.Auth)

	registerInvites(a)
	registerSections(a)
}
