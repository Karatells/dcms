// Package files — обменник файлов (зона обмена) и личные файлы пользователей.
package files

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

// Dir — папка зоны обмена (obmennik_dir).
type Dir struct {
	store.Base
	Name    string `json:"name"`
	Parent  int64  `json:"parent"`
	Ras     string `json:"ras"` // разрешённые расширения через «;» или «*»
	MaxSize int64  `json:"maxsize"`
	Upload  bool   `json:"upload"`
	My      bool   `json:"my"` // системная папка «Личные файлы»
	Num     int64  `json:"num"`
}

// Folder — личная папка пользователя (user_files).
type Folder struct {
	store.Base
	IDUser int64  `json:"id_user"`
	Name   string `json:"name"`
	Time   int64  `json:"time"`
	Msg    string `json:"msg"`
	Parent int64  `json:"parent"` // 0 — корень
	Osn    int64  `json:"osn"`    // уровень вложенности; 1 — корневая папка
	Pass   string `json:"pass"`
	Privat int64  `json:"privat"` // 0 — всем, 1 — только друзьям, 2 — скрытая (только владельцу)
}

// File — файл (obmennik_files).
type File struct {
	store.Base
	IDUser   int64  `json:"id_user"`
	IDDir    int64  `json:"id_dir"` // папка зоны обмена (или системная «личная»)
	MyDir    int64  `json:"my_dir"` // личная папка пользователя
	Name     string `json:"name"`
	Ras      string `json:"ras"`
	Type     string `json:"type"`
	Time     int64  `json:"time"`
	TimeLast int64  `json:"time_last"`
	TimeGo   int64  `json:"time_go"` // когда выложен в зону обмена
	Size     int64  `json:"size"`
	KLoads   int64  `json:"k_loads"`
	Opis     string `json:"opis"`
	Metka    bool   `json:"metka"`
	Screen   bool   `json:"screen"`
	Hidden   bool   `json:"hidden"` // скрыт владельцем (виден только ему)
}

// Komm — комментарий к файлу.
type Komm struct {
	store.Base
	IDFile int64  `json:"id_file"`
	IDUser int64  `json:"id_user"`
	Time   int64  `json:"time"`
	Msg    string `json:"msg"`
}

var (
	dirs    *store.Table[Dir, *Dir]
	folders *store.Table[Folder, *Folder]
	files   *store.Table[File, *File]
	komms   *store.Table[Komm, *Komm]
	music   *store.Table[Music, *Music]

	downloaded sync.Map // uid|file -> struct{}
)

func i(n int64) string  { return store.I(n) }
func itoa(n int) string { return store.I(int64(n)) }

func init() {
	core.Defaults["echo_rassh"] = "1"
	core.Defaults["obmen_limit_up"] = "0"
	core.Defaults["loads_new_file_hour"] = "24"
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

func Register(a *core.App) {
	db := a.DB
	dirs = store.NewTable[Dir](db, "obmennik_dir").Index("parent", func(d *Dir) string { return i(d.Parent) })
	folders = store.NewTable[Folder](db, "user_files").
		Index("user", func(f *Folder) string { return i(f.IDUser) }).
		Index("parent", func(f *Folder) string { return i(f.Parent) })
	files = store.NewTable[File](db, "obmennik_files").
		Index("dir", func(f *File) string { return i(f.IDDir) }).
		Index("mydir", func(f *File) string { return i(f.MyDir) }).
		Index("user", func(f *File) string { return i(f.IDUser) })
	music = store.NewTable[Music](db, "user_music").Index("user", func(m *Music) string { return i(m.IDUser) }).Index("file", func(m *Music) string { return i(m.IDFile) })
	komms = store.NewTable[Komm](db, "obmennik_komm").Index("file", func(k *Komm) string { return i(k.IDFile) })

	seedDirs()

	describeFile := func(id int64) (core.Desc, bool) {
		f, ok := files.Get(id)
		if !ok {
			return core.Desc{}, false
		}
		d := core.Desc{Title: f.Name + "." + f.Ras, Text: a.Text.Plain(f.Opis, 160), Link: "/obmen/file/" + i(f.ID), Owner: f.IDUser}
		if f.Screen && !f.Metka {
			d.Image = fmt.Sprintf("/api/obmen/file/%d/screen?s=200", f.ID)
		}
		return d, true
	}
	a.Describer("obmen", describeFile)
	a.Describer("files", describeFile)

	a.OnUserDelete(func(uid int64) {
		for _, f := range files.By("user", i(uid)) {
			deleteFile(a, f)
		}
		folders.DeleteBy("user", i(uid))
		komms.DeleteWhere(func(k *Komm) bool { return k.IDUser == uid })
		a.T.Likes.DeleteWhere(func(l *model.LikeObject) bool { return l.IDUser == uid && l.Type == "obmen" })
	})
	a.ProfileStat(func(a *core.App, viewer, target *model.User) []core.ProfileItem {
		n := files.CountBy("user", i(target.ID))
		if n == 0 {
			return nil
		}
		return []core.ProfileItem{{Key: "files", Title: "Файлы", Icon: "folder", Link: "/user/personalfiles/" + i(target.ID), Count: int64(n)}}
	})
	a.Counter("obmen", func(a *core.App, u *model.User) string {
		my := myDir()
		total := files.Count(func(f *File) bool { return f.IDDir != my.ID })
		n := 0
		if u != nil {
			n = files.Count(func(f *File) bool { return f.IDDir != my.ID && f.TimeGo > u.DateLast })
		}
		if n > 0 {
			return itoa(total) + " +" + itoa(n)
		}
		return itoa(total)
	})

	registerDirs(a)
	registerFolders(a)
	registerFiles(a)
	registerMusic(a)
	registerAdmin(a)
}

func seedDirs() {
	if dirs.Len() > 0 {
		return
	}
	dirs.Insert(&Dir{Name: "Личные файлы", My: true, Upload: true, Ras: "*", MaxSize: 100 << 20})
	for k, d := range []struct{ n, r string }{{"Картинки", "jpg;jpeg;png;gif;webp;bmp"}, {"Музыка", "mp3;ogg;wav;flac;m4a"}, {"Видео", "mp4;webm;mkv;avi;3gp"},
		{"Документы", "txt;pdf;doc;docx;xls;xlsx;odt;rtf"}, {"Программы", "zip;rar;7z;apk;exe;jar"}} {
		dirs.Insert(&Dir{Name: d.n, Ras: d.r, MaxSize: 30 << 20, Upload: true, Num: int64(k + 1)})
	}
}

func myDir() Dir {
	l := dirs.Find(func(d *Dir) bool { return d.My })
	if len(l) == 0 {
		return Dir{}
	}
	return l[0]
}

func extAllowed(d Dir, ext string) bool {
	if d.Ras == "*" {
		return true
	}
	for _, x := range strings.Split(d.Ras, ";") {
		if x != "" && strings.EqualFold(x, ext) {
			return true
		}
	}
	return false
}

func filePath(a *core.App, id int64) string {
	return filepath.Join(a.UploadDir("obmen"), fmt.Sprintf("%d.dat", id))
}
func screenPath(a *core.App, id int64) string {
	return filepath.Join(a.UploadDir("obmen", "screens"), fmt.Sprintf("%d.jpg", id))
}

func deleteFile(a *core.App, f File) {
	core.RemoveFile(filePath(a, f.ID))
	core.RemoveFile(screenPath(a, f.ID))
	komms.DeleteBy("file", i(f.ID))
	music.DeleteBy("file", i(f.ID))
	a.T.Likes.DeleteWhere(func(l *model.LikeObject) bool { return l.Type == "obmen" && l.IDObject == f.ID })
	a.T.Bookmarks.DeleteWhere(func(b *model.Bookmarks) bool { return b.Type == "file" && b.IDObject == f.ID })
	a.T.Discussions.DeleteWhere(func(d *model.Discussions) bool { return d.Type == "obmen" && d.IDSim == f.ID })
	a.T.Notifications.DeleteWhere(func(n *model.Notification) bool { return n.Type == "obmen_komm" && n.IDObject == f.ID })
	a.T.Tape.DeleteWhere(func(t *model.Tape) bool { return t.Type == "obmen" && t.IDFile == f.ID })
	files.Delete(f.ID)
}

func banErr(a *core.App, c *core.Ctx) error {
	if me := c.User(); me != nil {
		if b, ok := a.ActiveBan(me.ID, "files"); ok {
			msg := "Вам запрещён доступ к файлам"
			if b.Prich != "" {
				msg += ". Причина: " + b.Prich
			}
			return core.ErrForbidden(msg)
		}
	}
	return nil
}

// ---- пароли личных папок

func folderToken(f Folder) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%d|%s|dcms-pf", f.ID, f.Pass)))
	return hex.EncodeToString(h[:8])
}

// folderAccess — причина ограничения доступа к папке: "" (открыта), "private", "friends", "password".
func folderAccess(a *core.App, c *core.Ctx, f Folder) string {
	me := c.User()
	if me != nil && (me.ID == f.IDUser || a.Can(me.ID, "obmen_dir_edit")) {
		return ""
	}
	if f.Privat == 2 {
		return "private"
	}
	if f.Privat == 1 && (me == nil || !a.IsFriend(f.IDUser, me.ID)) {
		return "friends"
	}
	if f.Pass != "" {
		if ck, err := c.R.Cookie(fmt.Sprintf("pf%d", f.ID)); err != nil || ck.Value != folderToken(f) {
			return "password"
		}
	}
	return ""
}

// folderChainAccess проверяет доступ ко всей цепочке папок (от текущей до корня).
func folderChainAccess(a *core.App, c *core.Ctx, f Folder) string {
	for n := 0; n < 8; n++ {
		if reason := folderAccess(a, c, f); reason != "" {
			return reason
		}
		if f.Parent == 0 {
			return ""
		}
		p, ok := folders.Get(f.Parent)
		if !ok {
			return ""
		}
		f = p
	}
	return ""
}

// folderChainOpen — открыта ли папка и все её родители.
func folderChainOpen(a *core.App, c *core.Ctx, f Folder) bool {
	return folderChainAccess(a, c, f) == ""
}

// folderChainPublic — открыта ли цепочка папок для всех (без пароля, друзей и скрытия).
func folderChainPublic(f Folder) bool {
	for n := 0; n < 8; n++ {
		if f.Privat > 0 || f.Pass != "" {
			return false
		}
		if f.Parent == 0 {
			return true
		}
		p, ok := folders.Get(f.Parent)
		if !ok {
			return true
		}
		f = p
	}
	return true
}

func filePublic(f File) bool {
	if f.Hidden {
		return false
	}
	if fo, ok := folders.Get(f.MyDir); ok {
		return folderChainPublic(fo)
	}
	return true
}

func adultOK(c *core.Ctx, f File) bool {
	me := c.User()
	if !f.Metka || (me != nil && (me.ID == f.IDUser || me.Abuld)) {
		return true
	}
	ck, err := c.R.Cookie("abuld")
	return me != nil && err == nil && ck.Value == "1"
}

func setCookie(c *core.Ctx, name, val string) {
	http.SetCookie(c.W, &http.Cookie{Name: name, Value: val, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 86400 * 30})
}

func canEditFile(a *core.App, me *model.User, f File) bool {
	return me != nil && (me.ID == f.IDUser || (a.Can(me.ID, "obmen_file_edit") && me.Level > a.Ref(f.IDUser).Level))
}
func canDeleteFile(a *core.App, me *model.User, f File) bool {
	return me != nil && (me.ID == f.IDUser || (a.Can(me.ID, "obmen_file_delete") && me.Level > a.Ref(f.IDUser).Level))
}

var reName = regexp.MustCompile(`^[\p{L}\p{N}\-_()., ]+$`)
