// Package forum — форум: подфорумы → разделы → темы → сообщения, опросы, вложения, закладки, поиск.
package forum

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

type Forum struct {
	store.Base
	Name string `json:"name"`
	Pos  int64  `json:"pos"`
	Opis string `json:"opis"`
	Adm  bool   `json:"adm"` // только для администрации
	Icon string `json:"icon"`
}
type Razdel struct {
	store.Base
	IDForum int64  `json:"id_forum"`
	Name    string `json:"name"`
	Time    int64  `json:"time"`
	Opis    string `json:"opis"`
}
type Them struct {
	store.Base
	IDForum    int64  `json:"id_forum"`
	IDRazdel   int64  `json:"id_razdel"`
	Name       string `json:"name"`
	IDUser     int64  `json:"id_user"`
	TimeCreate int64  `json:"time_create"`
	Time       int64  `json:"time"`
	Up         bool   `json:"up"`
	Close      bool   `json:"close"`
	Text       string `json:"text"`
	IDEdit     int64  `json:"id_edit"`
	IDClose    int64  `json:"id_close"`
	TimeEdit   int64  `json:"time_edit"`
	Vote       string `json:"vote"`
	VoteClose  bool   `json:"vote_close"`
}
type Post struct {
	store.Base
	IDForum  int64  `json:"id_forum"`
	IDRazdel int64  `json:"id_razdel"`
	IDThem   int64  `json:"id_them"`
	IDUser   int64  `json:"id_user"` // 0 — системное сообщение
	Time     int64  `json:"time"`
	Msg      string `json:"msg"`
	Cit      int64  `json:"cit"`
}
type FFile struct {
	store.Base
	IDPost int64  `json:"id_post"`
	Name   string `json:"name"`
	Ras    string `json:"ras"`
	Size   int64  `json:"size"`
	Type   string `json:"type"`
	Count  int64  `json:"count"`
}
type FileRating struct {
	store.Base
	IDFile int64 `json:"id_file"`
	IDUser int64 `json:"id_user"`
	Rating int64 `json:"rating"`
}
type VoteOpt struct {
	store.Base
	Them int64  `json:"them"`
	Var  string `json:"var"`
	Num  int64  `json:"num"`
}
type VoteUser struct {
	store.Base
	Them   int64 `json:"them"`
	Var    int64 `json:"var"` // num варианта
	IDUser int64 `json:"id_user"`
	Time   int64 `json:"time"`
}

var (
	forums   *store.Table[Forum, *Forum]
	razdels  *store.Table[Razdel, *Razdel]
	thems    *store.Table[Them, *Them]
	posts    *store.Table[Post, *Post]
	ffiles   *store.Table[FFile, *FFile]
	frating  *store.Table[FileRating, *FileRating]
	voteOpts *store.Table[VoteOpt, *VoteOpt]
	voteUser *store.Table[VoteUser, *VoteUser]

	themeThrottle sync.Map // uid -> последнее создание темы
)

func init() {
	core.Defaults["forum_counter"] = "0"
}

func i(n int64) string { return store.I(n) }

func Register(a *core.App) {
	db := a.DB
	forums = store.NewTable[Forum](db, "forum_f")
	razdels = store.NewTable[Razdel](db, "forum_r").Index("forum", func(r *Razdel) string { return i(r.IDForum) })
	thems = store.NewTable[Them](db, "forum_t").
		Index("razdel", func(t *Them) string { return i(t.IDRazdel) }).
		Index("user", func(t *Them) string { return i(t.IDUser) })
	posts = store.NewTable[Post](db, "forum_p").
		Index("them", func(p *Post) string { return i(p.IDThem) }).
		Index("user", func(p *Post) string { return i(p.IDUser) })
	ffiles = store.NewTable[FFile](db, "forum_files").Index("post", func(f *FFile) string { return i(f.IDPost) })
	frating = store.NewTable[FileRating](db, "forum_files_rating").Index("file", func(f *FileRating) string { return i(f.IDFile) })
	voteOpts = store.NewTable[VoteOpt](db, "votes_forum").Index("them", func(v *VoteOpt) string { return i(v.Them) })
	voteUser = store.NewTable[VoteUser](db, "votes_user").Index("them", func(v *VoteUser) string { return i(v.Them) })

	a.Describer("them", func(id int64) (core.Desc, bool) {
		t, ok := thems.Get(id)
		if !ok {
			return core.Desc{}, false
		}
		return core.Desc{Title: t.Name, Text: a.Text.Plain(t.Text, 160), Link: themeLink(t), Owner: t.IDUser}, true
	})

	a.OnUserDelete(func(uid int64) {
		for _, t := range thems.By("user", i(uid)) {
			deleteTheme(a, t)
		}
		posts.DeleteBy("user", i(uid))
		voteUser.DeleteWhere(func(v *VoteUser) bool { return v.IDUser == uid })
		frating.DeleteWhere(func(v *FileRating) bool { return v.IDUser == uid })
		a.T.Bookmarks.DeleteWhere(func(b *model.Bookmarks) bool { return b.IDUser == uid && b.Type == "forum" })
	})

	a.ProfileStat(func(a *core.App, viewer, target *model.User) []core.ProfileItem {
		n := thems.CountBy("user", i(target.ID))
		m := posts.CountBy("user", i(target.ID))
		if n == 0 && m == 0 {
			return nil
		}
		return []core.ProfileItem{{Key: "forum", Title: "Форум", Icon: "forum", Link: "/forum/my", Count: int64(n + m)}}
	})

	a.Counter("forum", func(a *core.App, u *model.User) string {
		if a.Cfg.Int("forum_counter") != 0 {
			return ""
		}
		np, nt := posts.Len(), thems.Len()
		if !isStaff(u) {
			for _, f := range forums.Find(func(f *Forum) bool { return f.Adm }) {
				nt -= thems.Count(func(t *Them) bool { return t.IDForum == f.ID })
				np -= posts.Count(func(p *Post) bool { return p.IDForum == f.ID })
			}
		}
		return fmt.Sprintf("%d/%d", np, nt)
	})

	registerStructure(a)
	registerThemes(a)
	registerPosts(a)
	registerPoll(a)
	registerFiles(a)
	registerLists(a)
	registerAdmin(a)
}

func themeLink(t Them) string { return fmt.Sprintf("/forum/%d/%d/%d", t.IDForum, t.IDRazdel, t.ID) }

// ---------- общие проверки

func isStaff(u *model.User) bool { return u != nil && u.Level > 0 }

func visibleForum(f Forum, u *model.User) bool { return !f.Adm || isStaff(u) }

func forumBan(a *core.App, u *model.User) error {
	if u == nil {
		return nil
	}
	if b, ok := a.ActiveBan(u.ID, "forum"); ok {
		msg := "Вам запрещён доступ к форуму"
		if b.Prich != "" {
			msg += ". Причина: " + b.Prich
		}
		return core.ErrForbidden(msg)
	}
	return nil
}

func lenOK(s string, min, max int, what string) error {
	n := utf8.RuneCountInString(s)
	if n < min {
		return core.ErrBad(what + " слишком короткое")
	}
	if n > max {
		return core.ErrBad(fmt.Sprintf("%s слишком длинное (максимум %d)", what, max))
	}
	return nil
}

func sysPost(a *core.App, t Them, text string) {
	posts.Insert(&Post{IDForum: t.IDForum, IDRazdel: t.IDRazdel, IDThem: t.ID, IDUser: 0, Time: core.Now(), Msg: text})
}

func groupName(a *core.App, u *model.User) string {
	g, _ := a.T.Groups.Get(u.GroupAccess)
	return g.Name
}

func postCount(t Them) int { return posts.CountBy("them", i(t.ID)) }

func lastPost(t Them) (Post, bool) {
	var best Post
	ok := false
	for _, p := range posts.By("them", i(t.ID)) {
		if !ok || p.Time > best.Time || (p.Time == best.Time && p.ID > best.ID) {
			best, ok = p, true
		}
	}
	return best, ok
}

func themeSummary(a *core.App, t Them) map[string]any {
	m := map[string]any{"id": t.ID, "id_forum": t.IDForum, "id_razdel": t.IDRazdel, "name": t.Name, "up": t.Up, "close": t.Close,
		"author": a.Ref(t.IDUser), "time": t.Time, "time_create": t.TimeCreate, "posts": postCount(t), "poll": t.Vote != ""}
	if lp, ok := lastPost(t); ok {
		m["last"] = map[string]any{"user": a.Ref(lp.IDUser), "time": lp.Time}
	}
	return m
}

func forumOf(c *core.Ctx) (Forum, error) {
	f, ok := forums.Get(c.PathInt("f"))
	if !ok || !visibleForum(f, c.User()) {
		return f, core.ErrNotFound("Подфорум не найден")
	}
	return f, nil
}

func razdelOf(c *core.Ctx) (Forum, Razdel, error) {
	f, err := forumOf(c)
	if err != nil {
		return f, Razdel{}, err
	}
	r, ok := razdels.Get(c.PathInt("r"))
	if !ok || r.IDForum != f.ID {
		return f, r, core.ErrNotFound("Раздел не найден")
	}
	return f, r, nil
}

func themOf(c *core.Ctx) (Forum, Razdel, Them, error) {
	f, r, err := razdelOf(c)
	if err != nil {
		return f, r, Them{}, err
	}
	t, ok := thems.Get(c.PathInt("t"))
	if !ok || t.IDRazdel != r.ID {
		return f, r, t, core.ErrNotFound("Тема не найдена")
	}
	return f, r, t, nil
}

func deleteTheme(a *core.App, t Them) {
	for _, p := range posts.By("them", i(t.ID)) {
		deletePostFiles(a, p.ID)
	}
	posts.DeleteBy("them", i(t.ID))
	voteOpts.DeleteBy("them", i(t.ID))
	voteUser.DeleteBy("them", i(t.ID))
	a.T.Bookmarks.DeleteWhere(func(b *model.Bookmarks) bool { return b.Type == "forum" && b.IDObject == t.ID })
	a.T.Discussions.DeleteWhere(func(d *model.Discussions) bool { return d.Type == "them" && d.IDSim == t.ID })
	thems.Delete(t.ID)
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

var _ = strings.TrimSpace

// noMat — ошибка, если в тексте найден мат (как antimat в старом движке).
func noMat(a *core.App, s, what string) error {
	if a.Text.Censor(s) != s {
		return core.ErrBad("В " + what + " обнаружен мат")
	}
	return nil
}
