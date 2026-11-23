// Package foto — фотоальбомы, фотографии, оценки, комментарии, аватары.
package foto

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

type Gallery struct {
	store.Base
	IDUser     int64  `json:"id_user"`
	Name       string `json:"name"`
	TimeCreate int64  `json:"time_create"`
	Time       int64  `json:"time"`
	Opis       string `json:"opis"`
	Pass       string `json:"pass"`
	Privat     int64  `json:"privat"`      // 0 все, 1 друзья, 2 только я
	PrivatKomm int64  `json:"privat_komm"` // кто может комментировать
	My         bool   `json:"my"`          // системный альбом «Фото со страницы»
}
type Foto struct {
	store.Base
	IDGallery int64  `json:"id_gallery"`
	IDUser    int64  `json:"id_user"`
	Name      string `json:"name"`
	Ras       string `json:"ras"`
	Type      string `json:"type"`
	Opis      string `json:"opis"`
	Rating    int64  `json:"rating"`
	Avatar    bool   `json:"avatar"`
	Time      int64  `json:"time"`
	Metka     bool   `json:"metka"` // 18+
	W         int64  `json:"w"`
	H         int64  `json:"h"`
}
type Komm struct {
	store.Base
	IDFoto int64  `json:"id_foto"`
	IDUser int64  `json:"id_user"`
	Time   int64  `json:"time"`
	Msg    string `json:"msg"`
}
type Rating struct {
	store.Base
	IDFoto int64 `json:"id_foto"`
	IDUser int64 `json:"id_user"`
	Like   int64 `json:"like"`
	Avtor  int64 `json:"avtor"`
	Time   int64 `json:"time"`
}

var (
	galleries *store.Table[Gallery, *Gallery]
	fotos     *store.Table[Foto, *Foto]
	komms     *store.Table[Komm, *Komm]
	ratings   *store.Table[Rating, *Rating]
	thumbMu   sync.Mutex
)

func i(n int64) string  { return store.I(n) }
func itoa(n int) string { return store.I(int64(n)) }

func init() {
	core.Defaults["max_upload_foto_x"] = "6000"
	core.Defaults["max_upload_foto_y"] = "6000"
}

var reName = regexp.MustCompile(`^[\p{L}\p{N}\-_ (),.!]+$`)

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
	galleries = store.NewTable[Gallery](db, "gallery").Index("user", func(g *Gallery) string { return i(g.IDUser) })
	fotos = store.NewTable[Foto](db, "gallery_foto").
		Index("gallery", func(f *Foto) string { return i(f.IDGallery) }).
		Index("user", func(f *Foto) string { return i(f.IDUser) })
	komms = store.NewTable[Komm](db, "gallery_komm").Index("foto", func(k *Komm) string { return i(k.IDFoto) })
	ratings = store.NewTable[Rating](db, "gallery_rating").
		Index("foto", func(r *Rating) string { return i(r.IDFoto) }).
		Index("uf", func(r *Rating) string { return store.Key(r.IDUser, r.IDFoto) })

	a.Describer("foto", func(id int64) (core.Desc, bool) {
		f, ok := fotos.Get(id)
		if !ok {
			return core.Desc{}, false
		}
		g, _ := galleries.Get(f.IDGallery)
		img := imgURL(f, 200)
		if f.Metka {
			img = ""
		}
		return core.Desc{Title: f.Name, Text: a.Text.Plain(f.Opis, 160), Link: fmt.Sprintf("/foto/%d/%d/%d", f.IDUser, g.ID, f.ID), Image: img, Owner: f.IDUser}, true
	})
	a.Describer("album", func(id int64) (core.Desc, bool) {
		g, ok := galleries.Get(id)
		if !ok {
			return core.Desc{}, false
		}
		d := core.Desc{Title: g.Name, Text: a.Text.Plain(g.Opis, 160), Link: fmt.Sprintf("/foto/%d/%d", g.IDUser, g.ID), Owner: g.IDUser}
		if cf, ok := cover(g); ok && g.Pass == "" && g.Privat == 0 && !cf.Metka {
			d.Image = imgURL(cf, 200)
		}
		return d, true
	})
	a.OnUserDelete(func(uid int64) {
		for _, g := range galleries.By("user", i(uid)) {
			deleteGallery(a, g)
		}
		ratings.DeleteWhere(func(r *Rating) bool { return r.IDUser == uid })
		komms.DeleteWhere(func(k *Komm) bool { return k.IDUser == uid })
	})
	a.ProfileStat(func(a *core.App, viewer, target *model.User) []core.ProfileItem {
		n := fotos.CountBy("user", i(target.ID))
		return []core.ProfileItem{{Key: "foto", Title: "Фото", Icon: "camera", Link: "/foto/user/" + i(target.ID), Count: int64(n)}}
	})
	a.Counter("foto", func(a *core.App, u *model.User) string {
		total := fotos.Len()
		n := 0
		if u != nil {
			n = fotos.Count(func(f *Foto) bool { return f.Time > u.DateLast })
		}
		if n > 0 {
			return itoa(total) + " +" + itoa(n)
		}
		return itoa(total)
	})

	registerAlbums(a)
	registerPhotos(a)
	registerImages(a)
	registerAvatar(a)
	registerAdmin(a)
}

func imgURL(f Foto, s int) string {
	if s <= 0 {
		return fmt.Sprintf("/api/foto/img/%d.%s", f.ID, f.Ras)
	}
	return fmt.Sprintf("/api/foto/img/%d.%s?s=%d", f.ID, f.Ras, s)
}

func cover(g Gallery) (Foto, bool) {
	l, _ := fotos.Select(store.Opts[Foto]{Index: "gallery", Key: i(g.ID), Desc: true, Limit: 1})
	if len(l) == 0 {
		return Foto{}, false
	}
	return l[0], true
}

func fotoPath(a *core.App, id int64, ras string) string {
	return filepath.Join(a.UploadDir("foto"), fmt.Sprintf("%d.%s", id, ras))
}

func removeFotoFiles(a *core.App, f Foto) {
	core.RemoveFile(fotoPath(a, f.ID, f.Ras))
	if m, _ := filepath.Glob(filepath.Join(a.UploadDir("foto", "cache"), i(f.ID)+"_*")); m != nil {
		for _, p := range m {
			core.RemoveFile(p)
		}
	}
}

func deleteFoto(a *core.App, f Foto) {
	if f.Avatar {
		clearAvatar(a, f.IDUser, f.ID)
	}
	removeFotoFiles(a, f)
	komms.DeleteBy("foto", i(f.ID))
	ratings.DeleteBy("foto", i(f.ID))
	a.T.Bookmarks.DeleteWhere(func(b *model.Bookmarks) bool { return b.Type == "foto" && b.IDObject == f.ID })
	a.T.Discussions.DeleteWhere(func(d *model.Discussions) bool { return d.Type == "foto" && d.IDSim == f.ID })
	a.T.Notifications.DeleteWhere(func(n *model.Notification) bool { return n.Type == "foto_komm" && n.IDObject == f.ID })
	fotos.Delete(f.ID)
}

func deleteGallery(a *core.App, g Gallery) {
	for _, f := range fotos.By("gallery", i(g.ID)) {
		deleteFoto(a, f)
	}
	a.T.Tape.DeleteWhere(func(t *model.Tape) bool { return t.Type == "album" && t.IDFile == g.ID })
	galleries.Delete(g.ID)
}

func clearAvatar(a *core.App, uid, fotoID int64) {
	a.T.Users.Update(uid, func(u *model.User) {
		if u.AvatarID == fotoID {
			u.AvatarID, u.AvatarExt = 0, ""
		}
	})
	a.ForgetUser(uid)
}

// ---------- доступ

type access struct {
	ok     bool
	reason string // friends | private | password | ""
}

func passToken(g Gallery) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%d|%s|dcms-foto", g.ID, g.Pass)))
	return hex.EncodeToString(h[:8])
}

func unlocked(c *core.Ctx, g Gallery) bool {
	ck, err := c.R.Cookie(fmt.Sprintf("fp%d", g.ID))
	return err == nil && ck.Value == passToken(g)
}

// galleryAccess — может ли текущий пользователь смотреть альбом g.
func galleryAccess(a *core.App, c *core.Ctx, g Gallery) access {
	me := c.User()
	if me != nil && me.ID == g.IDUser {
		return access{ok: true}
	}
	owner, ok := a.GetUser(g.IDUser)
	if !ok {
		return access{reason: "private"}
	}
	var lvl int64
	var uid int64
	if me != nil {
		lvl, uid = me.Level, me.ID
	}
	higher := me != nil && lvl > owner.Level
	friend := uid != 0 && a.IsFriend(uid, g.IDUser)
	if !higher {
		switch a.UserSettings(g.IDUser).PrivatStr {
		case 0:
			return access{reason: "private"}
		case 2:
			if !friend {
				return access{reason: "friends"}
			}
		}
		switch g.Privat {
		case 1:
			if !friend {
				return access{reason: "friends"}
			}
		case 2:
			return access{reason: "private"}
		}
	}
	if g.Pass != "" && !unlocked(c, g) {
		return access{reason: "password"}
	}
	return access{ok: true}
}

func canManage(a *core.App, me *model.User, g Gallery, perm string) bool {
	if me == nil {
		return false
	}
	if me.ID == g.IDUser {
		return true
	}
	owner, _ := a.GetUser(g.IDUser)
	return a.Can(me.ID, perm) && me.Level > owner.Level
}

func adultOK(c *core.Ctx, f Foto) bool {
	me := c.User()
	if !f.Metka || (me != nil && (me.ID == f.IDUser || me.Abuld)) {
		return true
	}
	ck, err := c.R.Cookie("abuld")
	return me != nil && err == nil && ck.Value == "1"
}

func setSecureCookie(c *core.Ctx, name, val string) {
	http.SetCookie(c.W, &http.Cookie{Name: name, Value: val, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 86400 * 30})
}

var _ = os.Remove
var _ = sort.Ints
var _ = strings.TrimSpace

type modelUser = model.User

func osStat(p string) (os.FileInfo, error) { return os.Stat(p) }
func osRead(p string) ([]byte, error)      { return os.ReadFile(p) }
