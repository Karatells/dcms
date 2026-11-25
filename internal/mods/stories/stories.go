package stories

import (
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

const storyTTL = 86400 // 24 часа

// Story — история пользователя (живёт 24 часа).
type Story struct {
	store.Base
	IDUser    int64  `json:"id_user"`
	Caption   string `json:"caption"`
	MediaExt  string `json:"media_ext"`
	Bg        string `json:"bg"`
	Time      int64  `json:"time"`
	ExpiresAt int64  `json:"expires_at"`
}

// StoryView — отметка о просмотре истории.
type StoryView struct {
	store.Base
	IDStory int64 `json:"id_story"`
	IDUser  int64 `json:"id_user"`
	Time    int64 `json:"time"`
}

var (
	storiesT *store.Table[Story, *Story]
	viewsT   *store.Table[StoryView, *StoryView]
)

var allowedExts = map[string]string{
	".jpg":  "jpg",
	".jpeg": "jpg",
	".png":  "png",
	".webp": "webp",
	".gif":  "gif",
	".mp4":  "mp4",
	".webm": "webm",
	".mov":  "mov",
	".m4v":  "mp4",
	".3gp":  "3gp",
}

func isVideoExt(ext string) bool {
	return ext == "mp4" || ext == "webm" || ext == "mov" || ext == "3gp"
}

func i(n int64) string { return store.I(n) }

func storyFile(a *core.App, id int64, ext string) string {
	return filepath.Join(a.DataDir, "stories", i(id)+"."+ext)
}

func cleanupExpired(a *core.App) {
	now := core.Now()
	for _, s := range storiesT.All() {
		if s.ExpiresAt <= now {
			if s.MediaExt != "" {
				_ = os.Remove(storyFile(a, s.ID, s.MediaExt))
			}
			viewsT.DeleteBy("story", i(s.ID))
			storiesT.Delete(s.ID)
		}
	}
}

// Register регистрирует модуль историй (Stories на 24 часа).
func Register(a *core.App) {
	storiesT = store.NewTable[Story](a.DB, "stories").
		Index("user", func(x *Story) string { return i(x.IDUser) })
	viewsT = store.NewTable[StoryView](a.DB, "story_views").
		Index("story", func(x *StoryView) string { return i(x.IDStory) }).
		Index("pair", func(x *StoryView) string { return store.Key(x.IDStory, x.IDUser) })

	_ = os.MkdirAll(filepath.Join(a.DataDir, "stories"), 0o755)

	a.OnUserDelete(func(uid int64) {
		for _, s := range storiesT.By("user", i(uid)) {
			if s.MediaExt != "" {
				_ = os.Remove(storyFile(a, s.ID, s.MediaExt))
			}
			viewsT.DeleteBy("story", i(s.ID))
			storiesT.Delete(s.ID)
		}
	})

	// ── Лента активных историй (сгруппированных по авторам)
	a.GET("/api/stories", func(c *core.Ctx) error {
		cleanupExpired(a)
		me := c.User()
		now := core.Now()

		byUser := map[int64][]Story{}
		for _, s := range storiesT.All() {
			if s.ExpiresAt > now {
				byUser[s.IDUser] = append(byUser[s.IDUser], s)
			}
		}

		type groupItem struct {
			uid       int64
			latest    int64
			hasUnseen bool
			isMe      bool
			stories   []map[string]any
		}
		var groups []groupItem
		for uid, list := range byUser {
			if !a.CanAccessPrivacy(me, uid, a.UserSettings(uid).PrivatStories) {
				continue
			}
			sort.Slice(list, func(x, y int) bool { return list[x].Time < list[y].Time })
			var items []map[string]any
			hasUnseen := false
			for _, s := range list {
				seen := false
				if me != nil {
					_, seen = viewsT.OneBy("pair", store.Key(s.ID, me.ID))
				}
				if !seen {
					hasUnseen = true
				}
				media := ""
				mediaType := "text"
				if s.MediaExt != "" {
					media = "/api/stories/" + i(s.ID) + "/media"
					if isVideoExt(s.MediaExt) {
						mediaType = "video"
					} else {
						mediaType = "image"
					}
				}
				canDel := me != nil && (me.ID == s.IDUser || me.Level >= 2)
				items = append(items, map[string]any{
					"id":         s.ID,
					"caption":    s.Caption,
					"media":      media,
					"media_type": mediaType,
					"bg":         s.Bg,
					"time":       s.Time,
					"expires_at": s.ExpiresAt,
					"views":      viewsT.CountBy("story", i(s.ID)),
					"seen":       seen,
					"can_delete": canDel,
					"is_owner":   me != nil && me.ID == s.IDUser,
				})
			}
			groups = append(groups, groupItem{
				uid:       uid,
				latest:    list[len(list)-1].Time,
				hasUnseen: hasUnseen,
				isMe:      me != nil && me.ID == uid,
				stories:   items,
			})
		}

		sort.Slice(groups, func(x, y int) bool {
			if groups[x].isMe != groups[y].isMe {
				return groups[x].isMe
			}
			if groups[x].hasUnseen != groups[y].hasUnseen {
				return groups[x].hasUnseen
			}
			return groups[x].latest > groups[y].latest
		})

		out := make([]map[string]any, 0, len(groups))
		for _, g := range groups {
			out = append(out, map[string]any{
				"user":       a.Ref(g.uid),
				"has_unseen": g.hasUnseen,
				"is_me":      g.isMe,
				"stories":    g.stories,
			})
		}
		return c.JSON(map[string]any{"groups": out})
	})

	// ── Опубликовать историю (фото или текстовую карточку на градиенте)
	a.POST("/api/stories", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if b, ok := a.ActiveBan(me.ID, "all"); ok {
			return core.ErrForbidden("Вы заблокированы: " + b.Prich)
		}
		cleanupExpired(a)
		maxStories := 10
		if me.VIPUntil > core.Now() {
			maxStories = 25
		}
		if len(storiesT.By("user", i(me.ID))) >= maxStories {
			return core.ErrBad("Максимум " + i(int64(maxStories)) + " активных историй за 24 часа")
		}

		if err := c.R.ParseMultipartForm(52 << 20); err != nil {
			return core.ErrBad("Некорректные данные формы")
		}
		caption := a.Text.Clean(strings.TrimSpace(c.R.FormValue("caption")))
		if utf8.RuneCountInString(caption) > 200 {
			return core.ErrBad("Подпись не длиннее 200 символов")
		}
		bg := strings.TrimSpace(c.R.FormValue("bg"))
		if bg == "" || len(bg) > 120 {
			bg = "linear-gradient(135deg,#6366f1,#ec4899)"
		}

		var ext string
		var tmpFile string
		f, hdr, ferr := c.R.FormFile("file")
		if ferr == nil {
			defer f.Close()
			e := strings.ToLower(filepath.Ext(hdr.Filename))
			okExt, ok := allowedExts[e]
			if !ok {
				return core.ErrBad("Поддерживаются изображения (JPG, PNG, WebP, GIF) и видео (MP4, WebM, MOV)")
			}
			maxBytes := int64(10 << 20)
			if isVideoExt(okExt) {
				maxBytes = 50 << 20
			}
			if hdr.Size > maxBytes {
				if isVideoExt(okExt) {
					return core.ErrBad("Видео слишком большое (максимум 50 МБ)")
				}
				return core.ErrBad("Файл слишком большой (максимум 10 МБ)")
			}
			ext = okExt
			tf, err := os.CreateTemp(filepath.Join(a.DataDir, "stories"), "up-*."+ext)
			if err != nil {
				return err
			}
			tmpFile = tf.Name()
			n, copyErr := io.Copy(tf, io.LimitReader(f, maxBytes+1))
			_ = tf.Close()
			if copyErr != nil || n > maxBytes {
				_ = os.Remove(tmpFile)
				return core.ErrBad("Не удалось сохранить файл (проверьте размер)")
			}
		} else if caption == "" {
			return core.ErrBad("Добавьте изображение, видео или текст истории")
		}

		finalExt := ext
		if tmpFile != "" && isVideoExt(ext) {
			if ff, err := exec.LookPath("ffmpeg"); err == nil {
				outTmp := tmpFile + ".mp4"
				cmd := exec.Command(ff, "-y", "-i", tmpFile, "-t", "60",
					"-vf", "scale='min(720,iw)':-2",
					"-c:v", "libx264", "-preset", "veryfast", "-crf", "26", "-pix_fmt", "yuv420p",
					"-c:a", "aac", "-b:a", "96k", "-movflags", "+faststart", outTmp)
				if cmd.Run() == nil {
					_ = os.Remove(tmpFile)
					tmpFile = outTmp
					finalExt = "mp4"
				} else {
					_ = os.Remove(outTmp)
				}
			}
		}

		now := core.Now()
		id := storiesT.Insert(&Story{
			IDUser:    me.ID,
			Caption:   caption,
			MediaExt:  finalExt,
			Bg:        bg,
			Time:      now,
			ExpiresAt: now + storyTTL,
		})
		if tmpFile != "" {
			if err := os.Rename(tmpFile, storyFile(a, id, finalExt)); err != nil {
				_ = os.Remove(tmpFile)
				storiesT.Delete(id)
				return err
			}
		}
		return c.OK("id", id)
	}, core.Auth)

	// ── Отдача изображения истории
	a.GET("/api/stories/{id}/media", func(c *core.Ctx) error {
		s, ok := storiesT.Get(c.PathInt("id"))
		if !ok || s.MediaExt == "" || s.ExpiresAt <= core.Now() {
			return core.ErrNotFound()
		}
		p := storyFile(a, s.ID, s.MediaExt)
		if _, err := os.Stat(p); err != nil {
			return core.ErrNotFound()
		}
		c.W.Header().Set("Cache-Control", "public, max-age=86400")
		http.ServeFile(c.W, c.R, p)
		return nil
	})

	// ── Отметить просмотр истории
	a.POST("/api/stories/{id}/view", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		s, ok := storiesT.Get(c.PathInt("id"))
		if !ok || s.ExpiresAt <= core.Now() {
			return core.ErrNotFound()
		}
		if _, exists := viewsT.OneBy("pair", store.Key(s.ID, me.ID)); !exists {
			viewsT.Insert(&StoryView{IDStory: s.ID, IDUser: me.ID, Time: core.Now()})
		}
		return c.OK("views", viewsT.CountBy("story", i(s.ID)))
	}, core.Auth)

	// ── Кто просмотрел историю (для автора/админа)
	a.GET("/api/stories/{id}/viewers", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		s, ok := storiesT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		if me.ID != s.IDUser && me.Level < 2 {
			return core.ErrForbidden()
		}
		vs := viewsT.By("story", i(s.ID))
		sort.Slice(vs, func(x, y int) bool { return vs[x].Time > vs[y].Time })
		items := make([]map[string]any, 0, len(vs))
		for _, v := range vs {
			items = append(items, map[string]any{"user": a.Ref(v.IDUser), "time": v.Time})
		}
		return c.JSON(map[string]any{"items": items})
	}, core.Auth)

	// ── Удалить историю
	a.DELETE("/api/stories/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		s, ok := storiesT.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		if me.ID != s.IDUser && me.Level < 2 {
			return core.ErrForbidden()
		}
		if s.MediaExt != "" {
			_ = os.Remove(storyFile(a, s.ID, s.MediaExt))
		}
		viewsT.DeleteBy("story", i(s.ID))
		storiesT.Delete(s.ID)
		return c.OK()
	}, core.Auth)

	// Заглушка для неиспользуемого импорта model если потребуется в будущем
	_ = model.User{}
}
