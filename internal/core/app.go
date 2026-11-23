// Package core — ядро приложения: HTTP-обвязка, авторизация, права, настройки, текст, ленты, SSE.
package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"dcms/internal/model"
	"dcms/internal/store"
)

// App — корневой объект приложения; передаётся в Register(a) каждого модуля.
type App struct {
	homeMu     sync.Mutex
	homeBlocks []homeBlock
	DB         *store.DB
	T          *T
	Mux        *http.ServeMux
	Cfg        *Config
	Hub        *Hub
	Text       *Text
	DataDir    string // каталог данных (uploads лежат в DataDir/uploads/<модуль>)
	Web        fs.FS

	mu           sync.RWMutex
	userCache    map[int64]*cachedRef
	accessCache  map[int64]map[string]bool
	onDelete     []func(uid int64)
	counters     []counter
	crons        []*cronJob
	loaded       bool
	profileStats []func(a *App, viewer, target *model.User) []ProfileItem
	describers   map[string]func(int64) (Desc, bool)
}

type counter struct {
	Key string
	Fn  func(a *App, u *model.User) string
}

// Handler — обработчик API. Возвращает ошибку (Err/ErrNotFound/...) или nil после записи ответа.
type Handler func(c *Ctx) error

// Middleware оборачивает обработчик.
type Middleware func(Handler) Handler

// New создаёт приложение. Модули создают свои таблицы через a.DB до вызова Start().
func New(dataDir, dsn string, web fs.FS) (*App, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	db, err := store.Open(dsn)
	if err != nil {
		return nil, err
	}
	a := &App{DB: db, Mux: http.NewServeMux(), DataDir: dataDir, Hub: NewHub(), userCache: map[int64]*cachedRef{}, accessCache: map[int64]map[string]bool{}}
	a.T = newTables(db)
	a.Cfg = newConfig(a)
	a.Text = newText(a)
	a.Web = web
	return a, nil
}

// Start загружает данные, сидит начальные данные, вешает статику. Вызывать после регистрации всех модулей.
func (a *App) Start() error {
	if err := a.DB.Load(); err != nil {
		return err
	}
	a.loaded = true
	a.Cfg.load()
	a.seed()
	a.registerCoreRoutes()
	a.startCrons()
	return nil
}

// UploadDir возвращает (и создаёт) каталог загрузок модуля.
func (a *App) UploadDir(parts ...string) string {
	p := filepath.Join(append([]string{a.DataDir, "uploads"}, parts...)...)
	_ = os.MkdirAll(p, 0o755)
	return p
}

// OnUserDelete регистрирует очистку данных модуля при удалении пользователя.
func (a *App) OnUserDelete(fn func(uid int64)) { a.onDelete = append(a.onDelete, fn) }

// Counter регистрирует счётчик пункта меню/раздела (например "forum" → "12/3"). Значение отдаётся в /api/menu.
func (a *App) Counter(key string, fn func(a *App, u *model.User) string) {
	a.counters = append(a.counters, counter{key, fn})
}

// ------------------------------------------------------------------ Ctx

// Ctx — контекст запроса.
type Ctx struct {
	W    http.ResponseWriter
	R    *http.Request
	App  *App
	user *model.User
	done bool
	Sess *model.Session
}

// HTTPError — ошибка с кодом ответа.
type HTTPError struct {
	Code int
	Msg  string
}

func (e *HTTPError) Error() string { return e.Msg }

func Err(code int, msg string) error { return &HTTPError{code, msg} }
func ErrBad(msg string) error        { return &HTTPError{400, msg} }
func ErrAuth() error                 { return &HTTPError{401, "Требуется авторизация"} }
func ErrForbidden(msg ...string) error {
	m := "Недостаточно прав"
	if len(msg) > 0 {
		m = msg[0]
	}
	return &HTTPError{403, m}
}
func ErrNotFound(msg ...string) error {
	m := "Не найдено"
	if len(msg) > 0 {
		m = msg[0]
	}
	return &HTTPError{404, m}
}

// User — текущий пользователь или nil (гость).
func (c *Ctx) User() *model.User { return c.user }

// UID — id текущего пользователя или 0.
func (c *Ctx) UID() int64 {
	if c.user == nil {
		return 0
	}
	return c.user.ID
}

// Need возвращает пользователя или ошибку 401.
func (c *Ctx) Need() (*model.User, error) {
	if c.user == nil {
		return nil, ErrAuth()
	}
	return c.user, nil
}

// Can — есть ли у текущего пользователя привилегия (как user_access() старого движка).
func (c *Ctx) Can(access string) bool { return c.user != nil && c.App.Can(c.user.ID, access) }

// NeedAccess — 403, если нет привилегии.
func (c *Ctx) NeedAccess(access string) error {
	if _, err := c.Need(); err != nil {
		return err
	}
	if !c.Can(access) {
		return ErrForbidden()
	}
	return nil
}

// NeedLevel — 403, если уровень группы ниже level (only_level).
func (c *Ctx) NeedLevel(level int64) error {
	if _, err := c.Need(); err != nil {
		return err
	}
	if c.user.Level < level {
		return ErrForbidden()
	}
	return nil
}

// Path — значение переменной пути ({id}).
func (c *Ctx) Path(name string) string { return c.R.PathValue(name) }

// PathInt — числовая переменная пути (0, если не число).
func (c *Ctx) PathInt(name string) int64 {
	n, _ := strconv.ParseInt(c.R.PathValue(name), 10, 64)
	return n
}

// Q — параметр запроса.
func (c *Ctx) Q(name string) string { return c.R.URL.Query().Get(name) }

// QInt — числовой параметр запроса.
func (c *Ctx) QInt(name string) int64 {
	n, _ := strconv.ParseInt(c.R.URL.Query().Get(name), 10, 64)
	return n
}

// Bind разбирает JSON-тело (до 8 МБ).
func (c *Ctx) Bind(v any) error {
	c.R.Body = http.MaxBytesReader(c.W, c.R.Body, 8<<20)
	dec := json.NewDecoder(c.R.Body)
	if err := dec.Decode(v); err != nil {
		return ErrBad("Некорректный JSON: " + err.Error())
	}
	return nil
}

// Pager — параметры постраничного вывода.
type Pager struct {
	Page, Per, Offset int
}

// Page читает ?page=, размер страницы берёт из настройки пользователя (set_p_str) или системной p_str.
func (c *Ctx) Page(def ...int) Pager {
	per := c.App.Cfg.Int("p_str")
	if len(def) > 0 && def[0] > 0 {
		per = def[0]
	} else if c.user != nil && c.user.SetPStr > 0 {
		per = int(c.user.SetPStr)
	}
	if per <= 0 {
		per = 10
	}
	if per > 100 {
		per = 100
	}
	p, _ := strconv.Atoi(c.Q("page"))
	if p < 1 {
		p = 1
	}
	return Pager{Page: p, Per: per, Offset: (p - 1) * per}
}

// Paged — стандартный конверт списка.
type Paged struct {
	Items any `json:"items"`
	Total int `json:"total"`
	Page  int `json:"page"`
	Per   int `json:"per"`
	Pages int `json:"pages"`
}

// List формирует ответ со списком и пагинацией.
func (c *Ctx) List(items any, total int, p Pager) error {
	pages := (total + p.Per - 1) / p.Per
	if pages < 1 {
		pages = 1
	}
	return c.JSON(Paged{Items: items, Total: total, Page: p.Page, Per: p.Per, Pages: pages})
}

// JSON отправляет ответ 200 с JSON.
func (c *Ctx) JSON(v any) error { return c.Status(200, v) }

// Status отправляет JSON с кодом.
func (c *Ctx) Status(code int, v any) error {
	c.done = true
	h := c.W.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "SAMEORIGIN")
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
	c.W.WriteHeader(code)
	if v == nil {
		v = map[string]any{}
	}
	return json.NewEncoder(c.W).Encode(v)
}

// OK — {"ok":true, ...extra}.
func (c *Ctx) OK(extra ...any) error {
	m := map[string]any{"ok": true}
	for i := 0; i+1 < len(extra); i += 2 {
		m[fmt.Sprint(extra[i])] = extra[i+1]
	}
	return c.JSON(m)
}

// IP — IP клиента.
func (c *Ctx) IP() string {
	if xf := c.R.Header.Get("X-Forwarded-For"); xf != "" && c.App.Cfg.Bool("trust_proxy") {
		return strings.TrimSpace(strings.Split(xf, ",")[0])
	}
	h := c.R.RemoteAddr
	if i := strings.LastIndex(h, ":"); i > 0 {
		h = h[:i]
	}
	return strings.Trim(h, "[]")
}

// Now — текущее время unix.
func Now() int64 { return time.Now().Unix() }

// ------------------------------------------------------------------ routing

func (a *App) wrap(h Handler, mw []Middleware) http.HandlerFunc {
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		c := &Ctx{W: w, R: r, App: a}
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic %s %s: %v", r.Method, r.URL.Path, rec)
				if !c.done {
					c.Status(500, map[string]string{"error": "Внутренняя ошибка сервера"})
				}
			}
		}()
		a.authenticate(c)
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get("X-DCMS") == "" {
			c.Status(403, map[string]string{"error": "CSRF: нет заголовка X-DCMS"})
			return
		}
		if err := a.banGate(c); err != nil {
			a.writeErr(c, err)
			return
		}
		if err := h(c); err != nil {
			a.writeErr(c, err)
		}
	}
}

func (a *App) writeErr(c *Ctx, err error) {
	if c.done {
		return
	}
	var he *HTTPError
	if errors.As(err, &he) {
		c.Status(he.Code, map[string]string{"error": he.Msg})
		return
	}
	log.Printf("error %s %s: %v", c.R.Method, c.R.URL.Path, err)
	c.Status(500, map[string]string{"error": "Внутренняя ошибка сервера"})
}

// Route регистрирует маршрут: pattern как в net/http ("GET /api/forum/{id}").
func (a *App) Route(pattern string, h Handler, mw ...Middleware) {
	a.Mux.HandleFunc(pattern, a.wrap(h, mw))
}
func (a *App) GET(path string, h Handler, mw ...Middleware)    { a.Route("GET "+path, h, mw...) }
func (a *App) POST(path string, h Handler, mw ...Middleware)   { a.Route("POST "+path, h, mw...) }
func (a *App) PUT(path string, h Handler, mw ...Middleware)    { a.Route("PUT "+path, h, mw...) }
func (a *App) DELETE(path string, h Handler, mw ...Middleware) { a.Route("DELETE "+path, h, mw...) }

// Auth — middleware: только авторизованные.
func Auth(h Handler) Handler {
	return func(c *Ctx) error {
		if c.user == nil {
			return ErrAuth()
		}
		return h(c)
	}
}

// Level — middleware: уровень группы не ниже n.
func Level(n int64) Middleware {
	return func(h Handler) Handler {
		return func(c *Ctx) error {
			if err := c.NeedLevel(n); err != nil {
				return err
			}
			return h(c)
		}
	}
}

// Access — middleware: нужна привилегия группы.
func Access(name string) Middleware {
	return func(h Handler) Handler {
		return func(c *Ctx) error {
			if err := c.NeedAccess(name); err != nil {
				return err
			}
			return h(c)
		}
	}
}

// Run запускает HTTP-сервер (блокирует).
func (a *App) Run(addr string) error {
	srv := &http.Server{Addr: addr, Handler: a.Mux, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second}
	log.Printf("DCMS слушает %s", addr)
	return srv.ListenAndServe()
}

// Shutdown корректно завершает работу.
func (a *App) Shutdown(ctx context.Context) { _ = a.DB.Close() }
