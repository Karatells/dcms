package admin

import (
	"archive/zip"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

type field struct {
	Key   string      `json:"key"`
	Label string      `json:"label"`
	Type  string      `json:"type"` // text|int|bool|select|color
	Opts  [][2]string `json:"opts,omitempty"`
	Min   int         `json:"min,omitempty"`
	Max   int         `json:"max,omitempty"`
	Hint  string      `json:"hint,omitempty"`
}

var (
	selTheme = [][2]string{{"auto", "Как в системе"}, {"light", "Светлая"}, {"dark", "Тёмная"}}
	sections = map[string]struct {
		Title  string
		Access string
		Fields []field
	}{
		"sys": {"Системные настройки", "adm_set_sys", []field{
			{Key: "site_name", Label: "Название сайта", Type: "text"},
			{Key: "meta_keywords", Label: "Meta keywords", Type: "text"},
			{Key: "meta_description", Label: "Meta description", Type: "text"},
			{Key: "p_str", Label: "Пунктов на страницу (по умолчанию)", Type: "int", Min: 1, Max: 100},
			{Key: "theme_default", Label: "Тема оформления по умолчанию", Type: "select", Opts: selTheme},
			{Key: "accent_default", Label: "Акцентный цвет", Type: "color"},
			{Key: "exit", Label: "Подтверждение выхода", Type: "bool"},
			{Key: "antidos", Label: "Защита от флуда запросов", Type: "bool"},
			{Key: "antimat", Label: "Фильтр нецензурных слов", Type: "bool"},
			{Key: "trust_proxy", Label: "Доверять X-Forwarded-For (за прокси)", Type: "bool"},
			{Key: "mail_backup", Label: "E-mail для резервных копий", Type: "text"},
			{Key: "smtp_from", Label: "Адрес отправителя почты", Type: "text"},
			{Key: "new_user_balls", Label: "Баллов новому пользователю", Type: "int", Min: 0, Max: 100000},
			{Key: "new_user_money", Label: "Монет новому пользователю", Type: "int", Min: 0, Max: 100000},
			{Key: "vip_price_month", Label: "Цена VIP-подписки за 1 месяц (монет)", Type: "int", Min: 1, Max: 100000},
		}},
		"user": {"Настройки пользователей", "adm_set_user", []field{
			{Key: "reg_select", Label: "Регистрация", Type: "select", Opts: [][2]string{{"open", "Открыта"}, {"open_mail", "Открыта, с подтверждением по e-mail"}, {"close", "Закрыта"}}},
			{Key: "guest_select", Label: "Гостям разрешён просмотр сайта", Type: "select", Opts: [][2]string{{"1", "Да"}, {"0", "Нет, только после входа"}}},
			{Key: "show_away", Label: "Показывать промежуточную страницу при переходе по внешним ссылкам", Type: "bool"},
			{Key: "write_guest", Label: "Гостям разрешено писать в гостевой", Type: "bool"},
			{Key: "st", Label: "Показывать статусы пользователей", Type: "bool"},
			{Key: "set_show_icon", Label: "Иконки в списках по умолчанию", Type: "bool"},
			{Key: "time_chat", Label: "Автообновление чата, сек", Type: "int", Min: 0, Max: 900},
		}},
		"uploads": {"Лимиты загрузок", "adm_set_sys", []field{
			{Key: "max_upload_foto_mb", Label: "Фотографии и аватары: макс. размер (МБ)", Type: "int", Min: 1, Max: 200},
			{Key: "max_upload_foto_x", Label: "Фотографии: макс. ширина (px)", Type: "int", Min: 200, Max: 10000},
			{Key: "max_upload_foto_y", Label: "Фотографии: макс. высота (px)", Type: "int", Min: 200, Max: 10000},
			{Key: "max_upload_file_mb", Label: "Обменник и личные файлы: макс. размер (МБ)", Type: "int", Min: 1, Max: 2048},
			{Key: "max_upload_screen_mb", Label: "Скриншоты к файлам: макс. размер (МБ)", Type: "int", Min: 1, Max: 50},
			{Key: "max_upload_forum_mb", Label: "Форум (вложения): макс. размер файла (МБ)", Type: "int", Min: 1, Max: 200},
			{Key: "max_upload_forum_count", Label: "Форум: макс. файлов в одном сообщении", Type: "int", Min: 1, Max: 20},
			{Key: "shorts_max_mb", Label: "Клипы (видео): макс. размер исходника (МБ)", Type: "int", Min: 5, Max: 1024},
			{Key: "shorts_max_sec", Label: "Клипы: макс. длительность (сек)", Type: "int", Min: 5, Max: 180},
			{Key: "shorts_crf", Label: "Клипы: степень сжатия ffmpeg CRF (18–35, больше = меньше размер)", Type: "int", Min: 18, Max: 35},
			{Key: "max_upload_gift_mb", Label: "Подарки: макс. размер картинки (МБ)", Type: "int", Min: 1, Max: 20},
			{Key: "max_upload_smile_mb", Label: "Смайлы: макс. размер картинки (МБ)", Type: "int", Min: 1, Max: 20},
		}},
		"bbcode": {"BBCode", "adm_set_sys", func() []field {
			var f []field
			for _, k := range [][2]string{
				{"b", "[b] жирный"}, {"i", "[i] курсив"}, {"u", "[u] подчёркнутый"}, {"s", "[s] зачёркнутый"},
				{"big", "[big] крупный"}, {"small", "[small] мелкий"}, {"size", "[size=N] размер шрифта"},
				{"color", "[color=#HEX] произвольный HEX-цвет текста"}, {"bg", "[bg=#HEX] маркер / HEX-цвет фона"},
				{"red", "[red] красный"}, {"yellow", "[yellow] жёлтый"}, {"green", "[green] зелёный"}, {"blue", "[blue] синий"}, {"white", "[white] белый"},
				{"spoiler", "[spoiler] спойлер (размытие до клика)"}, {"cut", "[cut=Заголовок] сворачиваемый блок"},
				{"rainbow", "[rainbow] радужный градиент"}, {"mono", "[mono] встроенный моноширинный код"},
				{"quote", "[quote] цитата"}, {"code", "[code] блок кода"}, {"box", "[box=info|ok|warn|danger] цветной инфоблок"},
				{"center", "[center] выравнивание по центру"}, {"right", "[right] выравнивание по правому краю"}, {"hr", "[hr] горизонтальная линия"},
				{"http", "Автоссылки http://"}, {"url", "[url] ссылки"}, {"img", "[img] картинки"},
			} {
				f = append(f, field{Key: "bb_" + k[0], Label: k[1], Type: "bool"})
			}
			return f
		}()},
	}
)

func registerSystem(a *core.App) {
	a.GET("/api/admin/settings/{sec}", func(c *core.Ctx) error {
		s, ok := sections[c.R.PathValue("sec")]
		if !ok {
			return core.ErrNotFound()
		}
		if err := c.NeedAccess(s.Access); err != nil {
			return err
		}
		vals := map[string]string{}
		for _, f := range s.Fields {
			v := a.Cfg.Str(f.Key)
			if f.Type == "bool" {
				v = map[bool]string{true: "1", false: "0"}[a.Cfg.Bool(f.Key)]
			}
			vals[f.Key] = v
		}
		return c.OK("title", s.Title, "fields", s.Fields, "values", vals)
	}, core.Auth)
	a.PUT("/api/admin/settings/{sec}", func(c *core.Ctx) error {
		s, ok := sections[c.R.PathValue("sec")]
		if !ok {
			return core.ErrNotFound()
		}
		if err := c.NeedAccess(s.Access); err != nil {
			return err
		}
		var in map[string]any
		if err := c.Bind(&in); err != nil {
			return err
		}
		for _, f := range s.Fields {
			raw, has := in[f.Key]
			if !has {
				continue
			}
			var v string
			switch x := raw.(type) {
			case bool:
				v = map[bool]string{true: "1", false: "0"}[x]
			case float64:
				v = store.I(int64(x))
			case string:
				v = strings.TrimSpace(x)
			default:
				continue
			}
			switch f.Type {
			case "bool":
				if v != "0" && v != "1" {
					return core.ErrBad("Неверное значение: " + f.Label)
				}
			case "int":
				n := int64(0)
				for _, r := range v {
					if r < '0' || r > '9' {
						return core.ErrBad("Нужно число: " + f.Label)
					}
					n = n*10 + int64(r-'0')
				}
				if v == "" || n < int64(f.Min) || n > int64(f.Max) {
					return core.ErrBad("Недопустимое значение: " + f.Label)
				}
			case "select":
				okv := false
				for _, o := range f.Opts {
					okv = okv || o[0] == v
				}
				if !okv {
					return core.ErrBad("Недопустимое значение: " + f.Label)
				}
			case "color":
				if len(v) != 7 || v[0] != '#' {
					return core.ErrBad("Цвет в формате #rrggbb")
				}
			case "text":
				if rlen(v) > 255 || (f.Key == "site_name" && v == "") {
					return core.ErrBad("Недопустимое значение: " + f.Label)
				}
				v = a.Text.Clean(v)
			}
			a.Cfg.Set(f.Key, v)
		}
		a.AdminLog(c.UID(), "Панель управления", "Настройки", "Изменены: "+s.Title)
		return c.OK()
	}, core.Auth)

	// общая информация о системе
	started := time.Now()
	a.GET("/api/admin/info", func(c *core.Ctx) error {
		if err := c.NeedAccess("adm_info"); err != nil {
			return err
		}
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		var size int64
		filepath.Walk(a.DataDir, func(p string, i os.FileInfo, err error) error {
			if err == nil && !i.IsDir() {
				size += i.Size()
			}
			return nil
		})
		dbStats, _ := a.DB.Stats()
		return c.OK("go", runtime.Version(), "os", runtime.GOOS+"/"+runtime.GOARCH, "cpus", runtime.NumCPU(), "goroutines", runtime.NumGoroutine(),
			"mem", ms.Alloc, "uptime", int64(time.Since(started).Seconds()), "data_size", size,
			"db_version", dbStats.Version, "db_size", dbStats.SizeBytes, "db_tables", dbStats.Tables,
			"users", a.T.Users.Len(), "bans", a.T.Bans.Len(), "spam", a.T.Spam.Len(), "sessions", a.T.Sessions.Len())
	}, core.Auth)

	// резервная копия базы данных MySQL/MariaDB (SQL-дамп в zip)
	a.GET("/api/admin/backup", func(c *core.Ctx) error {
		if err := c.NeedAccess("adm_mysql"); err != nil {
			return err
		}
		c.W.Header().Set("Content-Type", "application/zip")
		c.W.Header().Set("Content-Disposition", `attachment; filename="backup-`+time.Now().Format("2006-01-02_15-04")+`.zip"`)
		zw := zip.NewWriter(c.W)
		w, err := zw.Create("database.sql")
		if err != nil {
			_ = zw.Close()
			return core.ErrBad("Не удалось создать архив")
		}
		if err := a.DB.DumpSQL(w); err != nil {
			_ = zw.Close()
			return core.ErrBad("Не удалось выгрузить базу данных")
		}
		_ = zw.Close()
		a.AdminLog(c.UID(), "Система", "Резервная копия", "Скачан SQL-дамп базы данных")
		return nil
	}, core.Auth)

	// журнал действий администрации
	a.GET("/api/admin/log", func(c *core.Ctx) error {
		if err := c.NeedAccess("adm_log_read"); err != nil {
			return err
		}
		p := c.Page()
		uid, mod := c.QInt("user"), c.QInt("mod")
		list, total := a.T.AdminLog.Select(store.Opts[model.AdminLog]{Desc: true, Offset: p.Offset, Limit: p.Per,
			Where: func(l *model.AdminLog) bool { return (uid == 0 || l.IDUser == uid) && (mod == 0 || l.Mod == mod) }})
		out := make([]map[string]any, 0, len(list))
		for _, l := range list {
			m, _ := a.T.AdminLogMod.Get(l.Mod)
			ac, _ := a.T.AdminLogAct.Get(l.Act)
			out = append(out, map[string]any{"id": l.ID, "user": a.Ref(l.IDUser), "time": l.Time, "mod": m.Name, "act": ac.Name, "text": l.Opis})
		}
		mods := a.T.AdminLogMod.All()
		return c.OK("items", out, "total", total, "page", p.Page, "per", p.Per, "pages", (total+p.Per-1)/p.Per, "mods", mods)
	}, core.Auth)
	a.DELETE("/api/admin/log", func(c *core.Ctx) error {
		if err := c.NeedAccess("adm_log_delete"); err != nil {
			return err
		}
		a.T.AdminLog.DeleteWhere(func(*model.AdminLog) bool { return true })
		a.AdminLog(c.UID(), "Система", "Очистка журнала", "Журнал действий очищен")
		return c.OK()
	}, core.Auth)
}

// ---------- меню ----------

func registerMenu(a *core.App) {
	clean := func(s string) string { return a.Text.Clean(strings.TrimSpace(s)) }
	icon := func(s string) string {
		var b strings.Builder
		for _, r := range strings.ToLower(s) {
			if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' || r == ' ' {
				b.WriteRune(r)
			}
		}
		return b.String()
	}
	type in struct {
		Type    string `json:"type"`
		Name    string `json:"name"`
		Url     string `json:"url"`
		Counter string `json:"counter"`
		Icon    string `json:"icon"`
		Access  string `json:"accesses"`
	}
	valid := func(v *in) error {
		if v.Name = clean(v.Name); v.Name == "" || rlen(v.Name) > 64 {
			return core.ErrBad("Укажите название")
		}
		v.Url, v.Counter, v.Icon = clean(v.Url), clean(v.Counter), icon(v.Icon)
		if v.Type != "razd" {
			v.Type = "link"
			if v.Url == "" {
				return core.ErrBad("Укажите ссылку")
			}
		}
		return nil
	}
	a.GET("/api/admin/menu", func(c *core.Ctx) error {
		if err := c.NeedAccess("adm_menu"); err != nil {
			return err
		}
		l := a.T.Menu.All()
		sort.SliceStable(l, func(i, j int) bool { return l[i].Pos < l[j].Pos })
		return c.OK("items", l)
	}, core.Auth)
	a.POST("/api/admin/menu", func(c *core.Ctx) error {
		if err := c.NeedAccess("adm_menu"); err != nil {
			return err
		}
		var v in
		if err := c.Bind(&v); err != nil {
			return err
		}
		if err := valid(&v); err != nil {
			return err
		}
		var pos int64
		for _, m := range a.T.Menu.All() {
			pos = max(pos, m.Pos)
		}
		a.T.Menu.Insert(&model.Menu{Type: v.Type, Name: v.Name, Url: v.Url, Counter: v.Counter, Icon: v.Icon, Pos: pos + 1})
		return c.OK()
	}, core.Auth)
	a.PUT("/api/admin/menu/{id}", func(c *core.Ctx) error {
		if err := c.NeedAccess("adm_menu"); err != nil {
			return err
		}
		var v in
		if err := c.Bind(&v); err != nil {
			return err
		}
		if err := valid(&v); err != nil {
			return err
		}
		if _, ok := a.T.Menu.Update(c.PathInt("id"), func(m *model.Menu) { m.Name, m.Url, m.Counter, m.Icon = v.Name, v.Url, v.Counter, v.Icon }); !ok {
			return core.ErrNotFound()
		}
		return c.OK()
	}, core.Auth)
	a.POST("/api/admin/menu/{id}/move", func(c *core.Ctx) error {
		if err := c.NeedAccess("adm_menu"); err != nil {
			return err
		}
		var v struct {
			Dir string `json:"dir"`
		}
		if err := c.Bind(&v); err != nil {
			return err
		}
		l := a.T.Menu.All()
		sort.SliceStable(l, func(i, j int) bool { return l[i].Pos < l[j].Pos })
		id := c.PathInt("id")
		for i := range l {
			if l[i].ID != id {
				continue
			}
			j := i - 1
			if v.Dir == "down" {
				j = i + 1
			}
			if j >= 0 && j < len(l) {
				l[i], l[j] = l[j], l[i]
			}
			break
		}
		for i, m := range l {
			a.T.Menu.Update(m.ID, func(x *model.Menu) { x.Pos = int64(i + 1) })
		}
		return c.OK()
	}, core.Auth)
	a.DELETE("/api/admin/menu/{id}", func(c *core.Ctx) error {
		if err := c.NeedAccess("adm_menu"); err != nil {
			return err
		}
		if !a.T.Menu.Delete(c.PathInt("id")) {
			return core.ErrNotFound()
		}
		return c.OK()
	}, core.Auth)

	// меню пользовательских настроек
	a.GET("/api/admin/settings-menu", func(c *core.Ctx) error {
		if err := c.NeedAccess("user_settings"); err != nil {
			return err
		}
		l := a.T.SettingsMenu.All()
		sort.SliceStable(l, func(i, j int) bool { return l[i].Pos < l[j].Pos })
		return c.OK("items", l)
	}, core.Auth)
	a.POST("/api/admin/settings-menu", func(c *core.Ctx) error {
		if err := c.NeedAccess("user_settings"); err != nil {
			return err
		}
		var v in
		if err := c.Bind(&v); err != nil {
			return err
		}
		v.Type = "link"
		if err := valid(&v); err != nil {
			return err
		}
		var pos int64
		for _, m := range a.T.SettingsMenu.All() {
			pos = max(pos, m.Pos)
		}
		a.T.SettingsMenu.Insert(&model.UserSettings{Type: "link", Name: v.Name, Url: v.Url, Counter: v.Counter, Img: v.Icon, Accesses: clean(v.Access), Pos: pos + 1})
		return c.OK()
	}, core.Auth)
	a.PUT("/api/admin/settings-menu/{id}", func(c *core.Ctx) error {
		if err := c.NeedAccess("user_settings"); err != nil {
			return err
		}
		var v in
		if err := c.Bind(&v); err != nil {
			return err
		}
		v.Type = "link"
		if err := valid(&v); err != nil {
			return err
		}
		if _, ok := a.T.SettingsMenu.Update(c.PathInt("id"), func(m *model.UserSettings) {
			m.Name, m.Url, m.Counter, m.Img, m.Accesses = v.Name, v.Url, v.Counter, v.Icon, clean(v.Access)
		}); !ok {
			return core.ErrNotFound()
		}
		return c.OK()
	}, core.Auth)
	a.POST("/api/admin/settings-menu/{id}/move", func(c *core.Ctx) error {
		if err := c.NeedAccess("user_settings"); err != nil {
			return err
		}
		var v struct {
			Dir string `json:"dir"`
		}
		if err := c.Bind(&v); err != nil {
			return err
		}
		l := a.T.SettingsMenu.All()
		sort.SliceStable(l, func(i, j int) bool { return l[i].Pos < l[j].Pos })
		id := c.PathInt("id")
		for i := range l {
			if l[i].ID != id {
				continue
			}
			j := i - 1
			if v.Dir == "down" {
				j = i + 1
			}
			if j >= 0 && j < len(l) {
				l[i], l[j] = l[j], l[i]
			}
			break
		}
		for i, m := range l {
			a.T.SettingsMenu.Update(m.ID, func(x *model.UserSettings) { x.Pos = int64(i + 1) })
		}
		return c.OK()
	}, core.Auth)
	a.DELETE("/api/admin/settings-menu/{id}", func(c *core.Ctx) error {
		if err := c.NeedAccess("user_settings"); err != nil {
			return err
		}
		if !a.T.SettingsMenu.Delete(c.PathInt("id")) {
			return core.ErrNotFound()
		}
		return c.OK()
	}, core.Auth)
}

// ---------- статистика посещаемости и рефералы ----------

type Ref struct {
	store.Base
	URL  string `json:"url"`
	Time int64  `json:"time"`
}

var (
	refs    *store.Table[Ref, *Ref]
	hostMu  sync.Mutex
	hostDay int64
	hosts   = map[string]bool{}
)

func dayStart(t int64) int64 { return t - t%86400 }

func registerStats(a *core.App) {
	refs = store.NewTable[Ref](a.DB, "user_ref").Index("url", func(r *Ref) string { return r.URL })

	// учёт просмотра страницы (вызывается клиентом при переходах)
	a.POST("/api/hit", func(c *core.Ctx) error {
		var in struct {
			Ref string `json:"ref"`
		}
		_ = c.Bind(&in)
		now := core.Now()
		day := dayStart(now)
		ip, ua := core.IPToInt(c.IP()), c.R.UserAgent()
		key := store.Key(ip, ua)
		hostMu.Lock()
		if hostDay != day {
			hostDay = day
			hosts = map[string]bool{}
			for _, v := range a.T.VisitToday.Find(func(v *model.VisitToday) bool { return v.Time >= day }) {
				hosts[store.Key(v.IP, v.Ua)] = true
			}
		}
		newHost := !hosts[key]
		hosts[key] = true
		hostMu.Unlock()
		if newHost {
			a.T.VisitToday.Insert(&model.VisitToday{IP: ip, Ua: ua, Time: now})
		}
		row, ok := a.T.VisitDay.FindOne(func(v *model.VisitEveryday) bool { return v.Time == day })
		if !ok {
			row = model.VisitEveryday{Time: day}
			a.T.VisitDay.Insert(&row)
		}
		a.T.VisitDay.Update(row.ID, func(v *model.VisitEveryday) {
			v.Hit++
			if newHost {
				v.Host++
			}
		})
		if r := strings.TrimSpace(in.Ref); r != "" && rlen(r) < 255 {
			r = strings.TrimPrefix(strings.TrimPrefix(r, "https://"), "http://")
			refs.Insert(&Ref{URL: r, Time: now})
		}
		return c.OK()
	})

	a.GET("/api/admin/stats", func(c *core.Ctx) error {
		if err := c.NeedAccess("adm_statistic"); err != nil {
			return err
		}
		now := core.Now()
		days := 30
		type day struct {
			Day  int64 `json:"day"`
			Hit  int64 `json:"hit"`
			Host int64 `json:"host"`
			Reg  int   `json:"reg"`
		}
		out := make([]day, days)
		idx := map[int64]int{}
		for i := 0; i < days; i++ {
			d := dayStart(now) - int64(days-1-i)*86400
			out[i].Day = d
			idx[d] = i
		}
		for _, v := range a.T.VisitDay.All() {
			if i, ok := idx[dayStart(v.Time)]; ok {
				out[i].Hit, out[i].Host = v.Hit, v.Host
			}
		}
		a.T.Users.Scan(func(u *model.User) bool {
			if i, ok := idx[dayStart(u.DateReg)]; ok {
				out[i].Reg++
			}
			return true
		})
		online := a.T.Users.Count(func(u *model.User) bool { return u.DateLast > now-300 })
		return c.OK("days", out, "users", a.T.Users.Len(), "online", online, "guests", a.T.Guests.Len(),
			"male", a.T.Users.Count(func(u *model.User) bool { return u.Pol == 1 }),
			"female", a.T.Users.Count(func(u *model.User) bool { return u.Pol == 0 }))
	}, core.Auth)

	a.GET("/api/admin/refs", func(c *core.Ctx) error {
		if err := c.NeedAccess("adm_ref"); err != nil {
			return err
		}
		type agg struct {
			URL   string `json:"url"`
			Count int    `json:"count"`
			Time  int64  `json:"time"`
		}
		m := map[string]*agg{}
		refs.Scan(func(r *Ref) bool {
			g := m[r.URL]
			if g == nil {
				g = &agg{URL: r.URL}
				m[r.URL] = g
			}
			g.Count++
			g.Time = max(g.Time, r.Time)
			return true
		})
		l := make([]agg, 0, len(m))
		for _, g := range m {
			l = append(l, *g)
		}
		sort.Slice(l, func(i, j int) bool { return l[i].Count > l[j].Count })
		p := c.Page()
		total := len(l)
		if p.Offset < len(l) {
			l = l[p.Offset:]
		} else {
			l = nil
		}
		if len(l) > p.Per {
			l = l[:p.Per]
		}
		return c.List(l, total, p)
	}, core.Auth)
}
