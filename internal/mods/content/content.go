// Package content — блоги (notes), правила/информация, смайлы, реклама.
package content

import (
	"embed"
	"fmt"
	"unicode/utf8"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

//go:embed seed/*
var seedFS embed.FS

type Note struct {
	store.Base
	Msg         string `json:"msg"`
	Time        int64  `json:"time"`
	Name        string `json:"name"`
	Private     int64  `json:"private"` // кто может смотреть: 0 все, 1 друзья, 2 только я
	IDUser      int64  `json:"id_user"`
	PrivateKomm int64  `json:"private_komm"` // кто может комментировать
	Count       int64  `json:"count"`        // рейтинг: просмотры + лайки − дизлайки
	IDDir       int64  `json:"id_dir"`
	Share       bool   `json:"share"`
	ShareID     int64  `json:"share_id"`
	ShareText   string `json:"share_text"`
	ShareName   string `json:"share_name"`
	ShareIDUser int64  `json:"share_id_user"`
	ShareType   string `json:"share_type"` // notes | forum
}
type NoteDir struct {
	store.Base
	Msg  string `json:"msg"`
	Name string `json:"name"`
}
type NoteKomm struct {
	store.Base
	IDUser  int64  `json:"id_user"`
	IDNotes int64  `json:"id_notes"`
	Msg     string `json:"msg"`
	Time    int64  `json:"time"`
}
type NoteLike struct {
	store.Base
	IDNotes int64 `json:"id_notes"`
	IDUser  int64 `json:"id_user"`
	Like    int64 `json:"like"`
}
type NoteView struct {
	store.Base
	IDNotes int64 `json:"id_notes"`
	IDUser  int64 `json:"id_user"`
}
type Rule struct {
	store.Base
	Msg     string `json:"msg"`
	Time    int64  `json:"time"`
	IDUser  int64  `json:"id_user"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	NameURL string `json:"name_url"`
	Pos     int64  `json:"pos"`
	Type    string `json:"type"` // link | razd | msg
}
type RulePost struct {
	store.Base
	IDUser int64  `json:"id_user"`
	Msg    string `json:"msg"`
	Time   int64  `json:"time"`
	IDNews int64  `json:"id_news"`
	Pos    int64  `json:"pos"`
}
type Smile struct {
	store.Base
	Smile string `json:"smile"` // коды через |
	Dir   int64  `json:"dir"`
}
type SmileDir struct {
	store.Base
	Name string `json:"name"`
	Opis string `json:"opis"`
}
type Rekl struct {
	store.Base
	Name     string `json:"name"`
	Img      string `json:"img"`
	Link     string `json:"link"`
	TimeLast int64  `json:"time_last"`
	Sel      int64  `json:"sel"` // 1..4 — позиция
	Count    int64  `json:"count"`
	DopStr   bool   `json:"dop_str"` // через страницу-переход со счётчиком
}

var (
	notes     *store.Table[Note, *Note]
	noteDirs  *store.Table[NoteDir, *NoteDir]
	noteKomms *store.Table[NoteKomm, *NoteKomm]
	noteLikes *store.Table[NoteLike, *NoteLike]
	noteViews *store.Table[NoteView, *NoteView]
	rules     *store.Table[Rule, *Rule]
	rulePosts *store.Table[RulePost, *RulePost]
	smiles    *store.Table[Smile, *Smile]
	smileDirs *store.Table[SmileDir, *SmileDir]
	rekls     *store.Table[Rekl, *Rekl]
)

func i(n int64) string { return store.I(n) }

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

func noMat(a *core.App, s, what string) error {
	if a.Text.Censor(s) != s {
		return core.ErrBad("В " + what + " обнаружен мат")
	}
	return nil
}

func banErr(a *core.App, u *model.User) error {
	if u == nil {
		return nil
	}
	if b, ok := a.ActiveBan(u.ID, "notes"); ok {
		msg := "Вам запрещён доступ к блогам"
		if b.Prich != "" {
			msg += ". Причина: " + b.Prich
		}
		return core.ErrForbidden(msg)
	}
	return nil
}

func init() {
	core.Defaults["rekl_enabled"] = "1"
}

// Register подключает таблицы и маршруты модуля.
func Register(a *core.App) {
	db := a.DB
	notes = store.NewTable[Note](db, "notes").Index("user", func(n *Note) string { return i(n.IDUser) }).Index("dir", func(n *Note) string { return i(n.IDDir) }).
		Index("share", func(n *Note) string {
			if n.Share {
				return n.ShareType + "|" + i(n.ShareID)
			}
			return ""
		})
	noteDirs = store.NewTable[NoteDir](db, "notes_dir")
	noteKomms = store.NewTable[NoteKomm](db, "notes_komm").Index("note", func(k *NoteKomm) string { return i(k.IDNotes) })
	noteLikes = store.NewTable[NoteLike](db, "notes_like").Index("note", func(k *NoteLike) string { return i(k.IDNotes) }).Index("un", func(k *NoteLike) string { return store.Key(k.IDUser, k.IDNotes) })
	noteViews = store.NewTable[NoteView](db, "notes_count").Index("un", func(k *NoteView) string { return store.Key(k.IDUser, k.IDNotes) })
	rules = store.NewTable[Rule](db, "rules")
	rulePosts = store.NewTable[RulePost](db, "rules_p").Index("rule", func(k *RulePost) string { return i(k.IDNews) })
	smiles = store.NewTable[Smile](db, "smile").Index("dir", func(s *Smile) string { return i(s.Dir) })
	smileDirs = store.NewTable[SmileDir](db, "smile_dir")
	rekls = store.NewTable[Rekl](db, "rekl")

	registerNotes(a)
	registerRules(a)
	registerSmiles(a)
	registerRekl(a)
}
