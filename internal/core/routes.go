package core

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"

	"dcms/internal/model"
)

// MyInfo — приватная карточка текущего пользователя для клиента.
func (a *App) MyInfo(u *model.User) map[string]any {
	g, _ := a.T.Groups.Get(u.GroupAccess)
	var acc []string
	for _, ga := range a.T.GroupAccess.By("group", i64(u.GroupAccess)) {
		acc = append(acc, ga.IDAccess)
	}
	sort.Strings(acc)
	us := a.UserSettings(u.ID)
	return map[string]any{
		"id": u.ID, "nick": u.Nick, "pol": u.Pol, "email": u.Email, "avatar": AvatarURL(u, 100), "group": g.Name, "group_id": u.GroupAccess,
		"level": g.Level, "balls": u.Balls, "rating": u.Rating, "money": u.Money, "medal": Medal(u.Rating),
		"theme": u.Theme, "accent": u.Accent, "p_str": u.SetPStr, "time_chat": u.SetTimeChat, "timesdvig": u.SetTimesdvig,
		"translit": u.SetTranslit, "show_icon": u.SetShowIcon, "accesses": acc, "privacy": us,
		"counters": a.Counters(u),
	}
}

// Counters — значения всех зарегистрированных счётчиков для пользователя (пустые опускаются).
func (a *App) Counters(u *model.User) map[string]string {
	out := map[string]string{}
	for _, c := range a.counters {
		if v := c.Fn(a, u); v != "" && v != "0" {
			out[c.Key] = v
		}
	}
	return out
}

func (a *App) registerCoreRoutes() {
	// счётчики ядра
	a.Counter("notification", func(a *App, u *model.User) string {
		return strconv.Itoa(a.T.Notifications.Count(func(n *model.Notification) bool { return n.IDUser == u.ID && n.Read == 0 }))
	})
	a.Counter("tape", func(a *App, u *model.User) string {
		return strconv.Itoa(a.T.Tape.Count(func(n *model.Tape) bool { return n.IDUser == u.ID && !n.Read }))
	})
	a.Counter("discussions", func(a *App, u *model.User) string {
		return strconv.Itoa(a.T.Discussions.Count(func(n *model.Discussions) bool { return n.IDUser == u.ID && n.Count > 0 }))
	})
	a.Counter("friends_new", func(a *App, u *model.User) string { return strconv.Itoa(a.T.FriendReqs.CountBy("to", i64(u.ID))) })

	a.GET("/api/config", func(c *Ctx) error { return c.JSON(a.Cfg.Public()) })
	a.GET("/api/me", func(c *Ctx) error {
		if c.user == nil {
			return c.JSON(map[string]any{"user": nil, "config": a.Cfg.Public()})
		}
		return c.JSON(map[string]any{"user": a.MyInfo(c.user), "config": a.Cfg.Public()})
	})
	a.GET("/api/counters", func(c *Ctx) error {
		if c.user == nil {
			return c.JSON(map[string]any{})
		}
		return c.JSON(a.Counters(c.user))
	})
	// главное меню (таблица menu): разделы и ссылки со счётчиками
	a.GET("/api/menu", func(c *Ctx) error {
		items := a.T.Menu.All()
		sort.SliceStable(items, func(i, j int) bool { return items[i].Pos < items[j].Pos })
		cnt := map[string]string{}
		if c.user != nil {
			cnt = a.Counters(c.user)
		}
		type it struct {
			model.Menu
			Count string `json:"count"`
			Key   string `json:"key"`
		}
		out := make([]it, 0, len(items))
		for _, m := range items {
			k := strings.Trim(m.Url, "/")
			if i := strings.LastIndex(k, "/"); i >= 0 {
				k = k[i+1:]
			}
			out = append(out, it{Menu: m, Count: cnt[k], Key: k})
		}
		return c.JSON(out)
	})
	// меню настроек пользователя
	a.GET("/api/settings-menu", func(c *Ctx) error {
		items := a.T.SettingsMenu.All()
		sort.SliceStable(items, func(i, j int) bool { return items[i].Pos < items[j].Pos })
		return c.JSON(items)
	}, Auth)
	a.Route("GET /api/events", a.serveEvents)
	if os.Getenv("DCMS_DEV") == "1" {
		a.registerDevRoutes()
	}
	a.Mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			(&Ctx{W: w, R: r, App: a}).Status(404, map[string]string{"error": "Не найдено"})
			return
		}
		a.serveWeb(w, r)
	})
}

// serveWeb отдаёт встроенный фронтенд; неизвестные пути → index.html (SPA).
func (a *App) serveWeb(w http.ResponseWriter, r *http.Request) {
	if a.Web == nil {
		http.NotFound(w, r)
		return
	}
	p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if p == "" {
		p = "index.html"
	}
	f, err := a.Web.Open(p)
	if err != nil || isDir(a.Web, p) {
		if path.Ext(p) != "" {
			http.NotFound(w, r)
			return
		}
		p = "index.html"
		f, err = a.Web.Open(p)
		if err != nil {
			http.NotFound(w, r)
			return
		}
	}
	defer f.Close()
	st, _ := f.Stat()
	if strings.HasSuffix(p, ".html") || strings.HasSuffix(p, ".js") || strings.HasSuffix(p, ".css") {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=300")
	}
	if rs, ok := f.(interface {
		Seek(int64, int) (int64, error)
		Read([]byte) (int, error)
	}); ok {
		http.ServeContent(w, r, p, st.ModTime(), rs)
		return
	}
	b, _ := fs.ReadFile(a.Web, p)
	http.ServeContent(w, r, p, st.ModTime(), strings.NewReader(string(b)))
}

func isDir(f fs.FS, p string) bool {
	st, err := fs.Stat(f, p)
	return err == nil && st.IsDir()
}

// CreateUser создаёт пользователя со всеми настройками по умолчанию (как reg.php + install step_4).
// Первый зарегистрированный пользователь получает группу «Создатель» (15), 5000 баллов и 500 монет.
func (a *App) CreateUser(nick, password string, pol int64, email string, activated bool) (model.User, error) {
	now := Now()
	u := model.User{
		Nick: nick, NickLower: strings.ToLower(nick), PassHash: HashPassword(password), Email: email,
		Pol: pol, DateReg: now, DateAut: now, DateLast: now, GroupAccess: 1, SetPStr: 0, SetTimeChat: 0,
		SetShowIcon: 1, SetTranslit: true, AddKonts: 1, Wall: 1, Theme: "auto",
		Balls: int64(a.Cfg.Int("new_user_balls")), Money: int64(a.Cfg.Int("new_user_money")),
	}
	if a.T.Users.Len() == 0 {
		u.GroupAccess, u.Level, u.Balls, u.Money = 15, 10, 5000, 500
	}
	if !activated {
		u.Activation = RandToken(16)
	}
	a.T.Users.Insert(&u)
	a.UserSettings(u.ID)
	a.TapeSettings(u.ID)
	a.DiscSettings(u.ID)
	a.NotifSettings(u.ID)
	return u, nil
}

// registerDevRoutes — только при DCMS_DEV=1: быстрый вход для тестов (POST /api/dev/login {nick, group?, pol?}).
func (a *App) registerDevRoutes() {
	a.POST("/api/dev/login", func(c *Ctx) error {
		var in struct {
			Nick  string `json:"nick"`
			Group int64  `json:"group"`
			Pol   int64  `json:"pol"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		u, ok := a.UserByNick(in.Nick)
		if !ok {
			var err error
			u, err = a.CreateUser(in.Nick, "test1234", in.Pol, strings.ToLower(in.Nick)+"@example.test", true)
			if err != nil {
				return err
			}
		}
		if in.Group > 0 {
			a.T.Users.Update(u.ID, func(x *model.User) { x.GroupAccess = in.Group })
			a.ForgetUser(u.ID)
		}
		a.Login(c, u.ID)
		return c.OK("id", u.ID)
	})
}
