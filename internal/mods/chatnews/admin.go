package chatnews

import (
	"io"
	"strings"
	"unicode/utf8"

	"dcms/internal/core"
)

func init() {
	for k, v := range map[string]string{"time_chat": "10", "umnik_new": "60", "umnik_help": "30", "umnik_time": "100", "shutnik_new": "300",
		"chat_umnik": "Умник", "chat_shutnik": "Шутник", "write_guest": "0"} {
		core.Defaults[k] = v
	}
}

func seedBots() {
	if voprosy.Len() == 0 {
		for _, q := range [][2]string{{"Столица России", "москва"}, {"Самая большая планета Солнечной системы", "юпитер"}, {"Спутник Земли", "луна"},
			{"Язык программирования с талисманом-сусликом", "go"}, {"Единица измерения частоты", "герц"}, {"Белый медведь живёт на ... полюсе", "северном"}} {
			voprosy.Insert(&Vopros{Vopros: q[0], Otvet: q[1]})
		}
	}
	if shutki.Len() == 0 {
		for _, s := range []string{"— Доктор, я забываю всё на свете! — С какого времени? — С какого времени что?", "Программист ушёл в магазин за хлебом и молоком. Если будут яйца — взять десяток. Вернулся с десятью буханками: яйца были.", "Лучший способ ускорить компьютер — это бросить его с высоты 9,8 м/с²."} {
			shutki.Insert(&Shutka{Anek: s})
		}
	}
}

func registerAdmin(a *core.App) {
	seedBots()
	const acc = "adm_set_chat"
	a.GET("/api/admin/chat/settings", func(c *core.Ctx) error {
		if err := c.NeedAccess(acc); err != nil {
			return err
		}
		out := map[string]any{"vopros": voprosy.Len(), "shutki": shutki.Len()}
		for _, k := range []string{"time_chat", "umnik_new", "umnik_help", "umnik_time", "shutnik_new"} {
			out[k] = a.Cfg.Int(k)
		}
		out["chat_umnik"], out["chat_shutnik"], out["write_guest"] = a.Cfg.Str("chat_umnik"), a.Cfg.Str("chat_shutnik"), a.Cfg.Int("write_guest")
		return c.JSON(out)
	}, core.Auth)
	a.PUT("/api/admin/chat/settings", func(c *core.Ctx) error {
		if err := c.NeedAccess(acc); err != nil {
			return err
		}
		var in struct {
			TimeChat    int    `json:"time_chat"`
			UmnikNew    int    `json:"umnik_new"`
			UmnikHelp   int    `json:"umnik_help"`
			UmnikTime   int    `json:"umnik_time"`
			ShutnikNew  int    `json:"shutnik_new"`
			ChatUmnik   string `json:"chat_umnik"`
			ChatShutnik string `json:"chat_shutnik"`
			WriteGuest  int    `json:"write_guest"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		for _, n := range []string{in.ChatUmnik, in.ChatShutnik} {
			if utf8.RuneCountInString(strings.TrimSpace(n)) < 2 || utf8.RuneCountInString(n) > 32 {
				return core.ErrBad("Некорректное имя бота")
			}
		}
		set := func(k string, v int) { a.Cfg.Set(k, itoa(max(v, 1))) }
		set("time_chat", in.TimeChat)
		set("umnik_new", in.UmnikNew)
		set("umnik_help", in.UmnikHelp)
		set("umnik_time", in.UmnikTime)
		set("shutnik_new", in.ShutnikNew)
		a.Cfg.Set("chat_umnik", a.Text.Clean(in.ChatUmnik))
		a.Cfg.Set("chat_shutnik", a.Text.Clean(in.ChatShutnik))
		a.Cfg.Set("write_guest", itoa(min(max(in.WriteGuest, 0), 1)))
		a.AdminLog(c.UID(), "Чат", "Настройки", "Изменены настройки чата и гостевой")
		return c.OK()
	}, core.Auth)

	// импорт: multipart file (txt, по строке), kind=vopros|shutki, sep (для вопросов), replace
	a.POST("/api/admin/chat/import", func(c *core.Ctx) error {
		if err := c.NeedAccess(acc); err != nil {
			return err
		}
		f, _, err := c.R.FormFile("file")
		if err != nil {
			return core.ErrBad("Файл не выбран")
		}
		defer f.Close()
		raw, _ := io.ReadAll(io.LimitReader(f, 5<<20))
		text := strings.ReplaceAll(string(raw), "\r", "")
		kind, sep := c.R.FormValue("kind"), c.R.FormValue("sep")
		if sep == "" {
			sep = "|"
		}
		replace := c.R.FormValue("replace") == "1"
		n := 0
		switch kind {
		case "vopros":
			if replace {
				voprosy.DeleteWhere(func(*Vopros) bool { return true })
			}
			for _, ln := range strings.Split(text, "\n") {
				p := strings.SplitN(strings.TrimSpace(ln), sep, 2)
				if len(p) == 2 && strings.TrimSpace(p[0]) != "" && strings.TrimSpace(p[1]) != "" {
					voprosy.Insert(&Vopros{Vopros: strings.TrimSpace(p[0]), Otvet: strings.TrimSpace(p[1])})
					n++
				}
			}
		case "shutki":
			if replace {
				shutki.DeleteWhere(func(*Shutka) bool { return true })
			}
			for _, ln := range strings.Split(text, "\n") {
				if s := strings.TrimSpace(ln); utf8.RuneCountInString(s) >= 10 {
					shutki.Insert(&Shutka{Anek: s})
					n++
				}
			}
		default:
			return core.ErrBad("Неизвестный тип")
		}
		a.AdminLog(c.UID(), "Чат", "Импорт", "Загружено записей: "+itoa(n))
		return c.OK("added", n)
	}, core.Auth)
	a.POST("/api/admin/chat/clear-bots", func(c *core.Ctx) error {
		if err := c.NeedAccess(acc); err != nil {
			return err
		}
		switch c.Q("kind") {
		case "vopros":
			voprosy.DeleteWhere(func(*Vopros) bool { return true })
		case "shutki":
			shutki.DeleteWhere(func(*Shutka) bool { return true })
		}
		return c.OK()
	}, core.Auth)
}
