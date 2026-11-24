package music

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"dcms/internal/core"
	"dcms/internal/store"
)

// Track — аудиозапись пользователя.
type Track struct {
	store.Base
	UserID  int64  `json:"user_id"`
	Title   string `json:"title"`
	Artist  string `json:"artist"`
	Ext     string `json:"ext"`
	Size    int64  `json:"size"`
	Plays   int64  `json:"plays"`
	Created int64  `json:"created"`
}

// Playlist — плейлист пользователя.
type Playlist struct {
	store.Base
	UserID   int64  `json:"user_id"`
	Name     string `json:"name"`
	TrackIDs string `json:"track_ids"` // comma-separated track IDs
	Created  int64  `json:"created"`
}

var (
	tracksT    *store.Table[Track, *Track]
	playlistsT *store.Table[Playlist, *Playlist]
)

func i(n int64) string { return store.I(n) }

func trackFile(a *core.App, id int64, ext string) string {
	return filepath.Join(a.DataDir, "music", i(id)+"."+ext)
}

func trackView(a *core.App, t Track, meID int64, canMod bool) map[string]any {
	return map[string]any{
		"id":         t.ID,
		"title":      t.Title,
		"artist":     t.Artist,
		"size":       t.Size,
		"plays":      t.Plays,
		"created":    t.Created,
		"user":       a.Ref(t.UserID),
		"url":        "/api/music/" + i(t.ID) + "/stream",
		"can_delete": meID > 0 && (t.UserID == meID || canMod),
	}
}

// Register регистрирует музыкальный раздел и плейлисты.
func Register(a *core.App) {
	tracksT = store.NewTable[Track](a.DB, "music_tracks").
		Index("user", func(x *Track) string { return i(x.UserID) })
	playlistsT = store.NewTable[Playlist](a.DB, "music_playlists").
		Index("user", func(x *Playlist) string { return i(x.UserID) })

	musicDir := filepath.Join(a.DataDir, "music")
	_ = os.MkdirAll(musicDir, 0o755)

	a.OnUserDelete(func(uid int64) {
		for _, t := range tracksT.By("user", i(uid)) {
			_ = os.Remove(trackFile(a, t.ID, t.Ext))
			tracksT.Delete(t.ID)
		}
		playlistsT.DeleteBy("user", i(uid))
	})

	// Список треков
	a.GET("/api/music", func(c *core.Ctx) error {
		me := c.User()
		var meID int64
		canMod := false
		if me != nil {
			meID = me.ID
			canMod = me.Level >= 5
		}
		q := strings.ToLower(strings.TrimSpace(c.Q("q")))
		uid, _ := strconv.ParseInt(c.Q("user"), 10, 64)
		var all []Track
		if uid > 0 {
			all = tracksT.By("user", i(uid))
		} else {
			all = tracksT.All()
		}
		var list []Track
		for _, t := range all {
			if q != "" && !strings.Contains(strings.ToLower(t.Title+" "+t.Artist), q) {
				continue
			}
			list = append(list, t)
		}
		sort.Slice(list, func(x, y int) bool { return list[x].ID > list[y].ID })
		pg := c.Page()
		total := len(list)
		end := min(pg.Offset+pg.Per, total)
		var sl []Track
		if pg.Offset < total {
			sl = list[pg.Offset:end]
		}
		items := make([]map[string]any, 0, len(sl))
		for _, t := range sl {
			items = append(items, trackView(a, t, meID, canMod))
		}
		return c.List(items, total, pg)
	})

	// Загрузка трека
	a.POST("/api/music", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if _, ok := a.ActiveBan(me.ID, "all"); ok {
			return core.ErrForbidden("Вы заблокированы")
		}
		if err := c.R.ParseMultipartForm(28 << 20); err != nil {
			return core.ErrBad("Не удалось прочитать файл")
		}
		file, hdr, err := c.R.FormFile("file")
		if err != nil {
			return core.ErrBad("Выберите аудиофайл")
		}
		defer file.Close()
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(hdr.Filename), "."))
		switch ext {
		case "mp3", "ogg", "wav", "m4a", "flac", "webm":
		default:
			return core.ErrBad("Поддерживаются форматы MP3, OGG, WAV, M4A, FLAC")
		}
		title := strings.TrimSpace(c.R.FormValue("title"))
		artist := strings.TrimSpace(c.R.FormValue("artist"))
		if title == "" {
			title = strings.TrimSuffix(hdr.Filename, filepath.Ext(hdr.Filename))
		}
		if utf8.RuneCountInString(title) > 100 {
			title = string([]rune(title)[:100])
		}
		if artist == "" {
			artist = me.Nick
		}
		if utf8.RuneCountInString(artist) > 80 {
			artist = string([]rune(artist)[:80])
		}
		tr := Track{UserID: me.ID, Title: title, Artist: artist, Ext: ext, Created: core.Now()}
		tracksT.Insert(&tr)
		dstPath := trackFile(a, tr.ID, ext)
		dst, err := os.Create(dstPath)
		if err != nil {
			tracksT.Delete(tr.ID)
			return core.ErrBad("Ошибка сохранения файла")
		}
		n, err := io.Copy(dst, io.LimitReader(file, 25<<20))
		_ = dst.Close()
		if err != nil || n == 0 {
			_ = os.Remove(dstPath)
			tracksT.Delete(tr.ID)
			return core.ErrBad("Пустой файл")
		}
		tracksT.Update(tr.ID, func(x *Track) { x.Size = n })
		tr.Size = n
		a.AddBalls(me.ID, 3, 0)
		return c.JSON(trackView(a, tr, me.ID, me.Level >= 5))
	}, core.Auth)

	// Стрим аудиофайла
	a.GET("/api/music/{id}/stream", func(c *core.Ctx) error {
		tr, ok := tracksT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		if c.R.Header.Get("Range") == "" || strings.HasPrefix(c.R.Header.Get("Range"), "bytes=0-") {
			tracksT.Update(tr.ID, func(x *Track) { x.Plays++ })
		}
		http.ServeFile(c.W, c.R, trackFile(a, tr.ID, tr.Ext))
		return nil
	})

	// Удалить трек
	a.DELETE("/api/music/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		tr, ok := tracksT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		if tr.UserID != me.ID && me.Level < 5 {
			return core.ErrForbidden()
		}
		_ = os.Remove(trackFile(a, tr.ID, tr.Ext))
		tracksT.Delete(tr.ID)
		return c.OK()
	}, core.Auth)

	// Плейлисты пользователя
	a.GET("/api/music/playlists", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		pls := playlistsT.By("user", i(me.ID))
		out := make([]map[string]any, 0, len(pls))
		for _, pl := range pls {
			var tids []int64
			for _, s := range strings.Split(pl.TrackIDs, ",") {
				if id, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil && id > 0 {
					tids = append(tids, id)
				}
			}
			out = append(out, map[string]any{"id": pl.ID, "name": pl.Name, "track_ids": tids, "count": len(tids)})
		}
		return c.JSON(map[string]any{"items": out})
	}, core.Auth)

	a.POST("/api/music/playlists", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		var in struct {
			Name string `json:"name"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		name := strings.TrimSpace(in.Name)
		if utf8.RuneCountInString(name) < 2 || utf8.RuneCountInString(name) > 50 {
			return core.ErrBad("Название плейлиста должно быть от 2 до 50 символов")
		}
		pl := Playlist{UserID: me.ID, Name: name, Created: core.Now()}
		playlistsT.Insert(&pl)
		return c.OK("id", pl.ID)
	}, core.Auth)

	a.POST("/api/music/playlists/{id}/toggle", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		pl, ok := playlistsT.Get(c.PathInt("id"))
		if !ok || pl.UserID != me.ID {
			return core.ErrForbidden()
		}
		var in struct {
			TrackID int64 `json:"track_id"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		tidStr := i(in.TrackID)
		var parts []string
		found := false
		for _, s := range strings.Split(pl.TrackIDs, ",") {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			if s == tidStr {
				found = true
				continue
			}
			parts = append(parts, s)
		}
		if !found {
			parts = append(parts, tidStr)
		}
		playlistsT.Update(pl.ID, func(x *Playlist) { x.TrackIDs = strings.Join(parts, ",") })
		return c.OK("added", !found)
	}, core.Auth)
}
