package chatnews

import (
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

type Room struct {
	store.Base
	Pos     int64  `json:"pos"`
	Name    string `json:"name"`
	Umnik   bool   `json:"umnik"`
	Shutnik bool   `json:"shutnik"`
	Opis    string `json:"opis"`
}
type ChatPost struct {
	store.Base
	Room    int64  `json:"room"`
	IDUser  int64  `json:"id_user"`
	Time    int64  `json:"time"`
	Msg     string `json:"msg"`
	Vopros  int64  `json:"vopros"`
	UmnikSt int64  `json:"umnik_st"` // 0 — обычное, 1 вопрос, 2/3 подсказки, 4 итог
	Shutnik bool   `json:"shutnik"`
	Privat  int64  `json:"privat"`
}
type Shutka struct {
	store.Base
	Anek string `json:"anek"`
}
type Vopros struct {
	store.Base
	Vopros string `json:"vopros"`
	Otvet  string `json:"otvet"`
}

var (
	rooms     *store.Table[Room, *Room]
	chatPosts *store.Table[ChatPost, *ChatPost]
	shutki    *store.Table[Shutka, *Shutka]
	voprosy   *store.Table[Vopros, *Vopros]

	chatWho sync.Map // uid -> whoEnt
	botMu   sync.Mutex
)

type whoEnt struct{ room, time int64 }

func roomOnline(room int64) int {
	n, now := 0, core.Now()
	chatWho.Range(func(_, v any) bool {
		if w := v.(whoEnt); w.room == room && w.time > now-120 {
			n++
		}
		return true
	})
	return n
}

func totalOnline() int {
	n, now := 0, core.Now()
	chatWho.Range(func(k, v any) bool {
		if v.(whoEnt).time > now-120 {
			n++
		} else {
			chatWho.Delete(k)
		}
		return true
	})
	return n
}

func registerChat(a *core.App) {
	db := a.DB
	rooms = store.NewTable[Room](db, "chat_rooms")
	chatPosts = store.NewTable[ChatPost](db, "chat_post").Index("room", func(p *ChatPost) string { return i(p.Room) })
	shutki = store.NewTable[Shutka](db, "chat_shutnik")
	voprosy = store.NewTable[Vopros](db, "chat_vopros")

	if rooms.Len() == 0 {
		rooms.Insert(&Room{Pos: 1, Name: "Общий чат", Opis: "Общение на любые темы", Umnik: false, Shutnik: false})
		rooms.Insert(&Room{Pos: 2, Name: "Викторина", Opis: "Отвечайте на вопросы Умника и зарабатывайте баллы", Umnik: true})
		rooms.Insert(&Room{Pos: 3, Name: "Юмор", Opis: "Шутник рассказывает анекдоты", Shutnik: true})
	}

	a.Describer("chat", func(id int64) (core.Desc, bool) {
		r, ok := rooms.Get(id)
		if !ok {
			return core.Desc{}, false
		}
		return core.Desc{Title: r.Name, Link: "/chat/" + i(r.ID)}, true
	})
	a.OnUserDelete(func(uid int64) {
		chatPosts.DeleteWhere(func(p *ChatPost) bool { return p.IDUser == uid || p.Privat == uid })
		chatWho.Delete(uid)
	})
	a.Counter("chat", func(a *core.App, u *model.User) string { return itoa(totalOnline()) + " чел." })

	roomView := func(r Room) map[string]any {
		return map[string]any{"id": r.ID, "name": r.Name, "opis": r.Opis, "pos": r.Pos, "umnik": r.Umnik, "shutnik": r.Shutnik,
			"bots": botsCode(r), "online": roomOnline(r.ID), "opis_html": a.Text.Render(r.Opis)}
	}

	a.GET("/api/chat", func(c *core.Ctx) error {
		list := rooms.All()
		sort.Slice(list, func(x, y int) bool { return list[x].Pos < list[y].Pos })
		out := make([]map[string]any, 0, len(list))
		for _, r := range list {
			out = append(out, roomView(r))
		}
		return c.JSON(map[string]any{"items": out, "can_manage": c.Can("chat_room"), "can_clear": c.Can("chat_clear"), "online": totalOnline()})
	})

	a.GET("/api/chat-who", func(c *core.Ctx) error {
		list := onlineByURL(a, "/chat")
		sort.Slice(list, func(x, y int) bool { return list[x].DateLast > list[y].DateLast })
		p := c.Page()
		items := []map[string]any{}
		for x := p.Offset; x < min(p.Offset+p.Per, len(list)); x++ {
			items = append(items, map[string]any{"user": a.Ref(list[x].ID), "date_last": list[x].DateLast})
		}
		return c.List(items, len(list), p)
	})

	a.GET("/api/chat/{id}", func(c *core.Ctx) error {
		if err := banErr(a, c.User(), "chat", "в чат"); err != nil {
			return err
		}
		r, ok := rooms.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Комната не найдена")
		}
		me := c.User()
		if me != nil {
			chatWho.Store(me.ID, whoEnt{r.ID, core.Now()})
		}
		runBots(a, r)
		uid := c.UID()
		p := c.Page()
		list, total := chatPosts.Select(store.Opts[ChatPost]{Index: "room", Key: i(r.ID), Desc: true, Offset: p.Offset, Limit: p.Per,
			Where: func(x *ChatPost) bool { return x.Privat == 0 || (uid != 0 && (x.Privat == uid || x.IDUser == uid)) }})
		items := make([]map[string]any, 0, len(list))
		for _, x := range list {
			m := map[string]any{"id": x.ID, "time": x.Time, "msg": a.Text.Render(x.Msg), "private": x.Privat != 0, "to_me": x.Privat == uid && uid != 0}
			switch {
			case x.Shutnik:
				m["bot"] = "shutnik"
				m["user"] = map[string]any{"id": 0, "nick": a.Cfg.Str("chat_shutnik")}
			case x.UmnikSt != 0:
				m["bot"] = "umnik"
				m["user"] = map[string]any{"id": 0, "nick": a.Cfg.Str("chat_umnik")}
			default:
				m["user"] = a.Ref(x.IDUser)
				m["can_delete"] = c.Can("chat_clear")
			}
			items = append(items, m)
		}
		out := map[string]any{"room": roomView(r), "items": items, "total": total, "page": p.Page, "per": p.Per, "pages": max(1, (total+p.Per-1)/p.Per),
			"refresh": a.Cfg.Int("time_chat"), "can_post": me != nil, "can_manage": c.Can("chat_room")}
		if me != nil {
			out["refresh"] = me.SetTimeChat
		}
		return c.JSON(out)
	})

	a.POST("/api/chat/{id}", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if err := banErr(a, me, "chat", "в чат"); err != nil {
			return err
		}
		r, ok := rooms.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound("Комната не найдена")
		}
		var in struct {
			Msg    string `json:"msg"`
			Privat int64  `json:"privat"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		msg := strings.TrimSpace(in.Msg)
		if err := noMat(a, msg, "тексте сообщения"); err != nil {
			return err
		}
		if err := lenOK(msg, 2, 512, "Сообщение"); err != nil {
			return err
		}
		now := core.Now()
		if chatPosts.Count(func(p *ChatPost) bool { return p.Room == r.ID && p.IDUser == me.ID && p.Msg == msg && p.Time > now-300 }) > 0 {
			return core.ErrBad("Ваше сообщение повторяет предыдущее")
		}
		priv := in.Privat
		if priv != 0 {
			if _, ok := a.GetUser(priv); !ok {
				priv = 0
			}
		}
		chatPosts.Insert(&ChatPost{Room: r.ID, IDUser: me.ID, Time: now, Msg: msg, Privat: priv})
		chatWho.Store(me.ID, whoEnt{r.ID, now})
		runBots(a, r)
		a.Hub.Publish("chat:"+i(r.ID), "chat", map[string]any{"room": r.ID})
		return c.OK()
	}, core.Auth)

	a.DELETE("/api/chat-post/{id}", func(c *core.Ctx) error {
		if err := c.NeedAccess("chat_clear"); err != nil {
			return err
		}
		p, ok := chatPosts.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		chatPosts.Delete(p.ID)
		a.AdminLog(c.UID(), "Чат", "Удаление сообщения", "Удалено сообщение из комнаты "+i(p.Room))
		a.Hub.Publish("chat:"+i(p.Room), "chat", map[string]any{"room": p.Room})
		return c.OK()
	}, core.Auth)

	// ---- управление комнатами
	type roomIn struct {
		Name    string `json:"name"`
		Opis    string `json:"opis"`
		Pos     int64  `json:"pos"`
		Bots    int    `json:"bots"` // 0 нет, 1 умник, 2 шутник, 3 оба
		Umnik   bool   `json:"umnik"`
		Shutnik bool   `json:"shutnik"`
	}
	apply := func(r *Room, in roomIn) error {
		name := strings.TrimSpace(in.Name)
		if utf8.RuneCountInString(name) < 3 {
			return core.ErrBad("Слишком короткое название")
		}
		if utf8.RuneCountInString(name) > 32 {
			return core.ErrBad("Слишком длинное название")
		}
		r.Name, r.Opis, r.Pos = a.Text.Clean(name), strings.TrimSpace(in.Opis), in.Pos
		r.Umnik = in.Bots == 1 || in.Bots == 3
		r.Shutnik = in.Bots == 2 || in.Bots == 3
		return nil
	}
	a.POST("/api/chat-room", func(c *core.Ctx) error {
		if err := c.NeedAccess("chat_room"); err != nil {
			return err
		}
		var in roomIn
		if err := c.Bind(&in); err != nil {
			return err
		}
		r := Room{}
		if err := apply(&r, in); err != nil {
			return err
		}
		id := rooms.Insert(&r)
		a.AdminLog(c.UID(), "Чат", "Параметры комнат", "Добавлена комната '"+r.Name+"', описание: "+r.Opis)
		return c.OK("id", id)
	}, core.Auth)
	a.PUT("/api/chat-room/{id}", func(c *core.Ctx) error {
		if err := c.NeedAccess("chat_room"); err != nil {
			return err
		}
		r, ok := rooms.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		var in roomIn
		if err := c.Bind(&in); err != nil {
			return err
		}
		if err := apply(&r, in); err != nil {
			return err
		}
		rooms.Update(r.ID, func(x *Room) { *x = r })
		a.AdminLog(c.UID(), "Чат", "Параметры комнат", "Изменение комнаты "+r.Name)
		return c.OK()
	}, core.Auth)
	a.DELETE("/api/chat-room/{id}", func(c *core.Ctx) error {
		if err := c.NeedAccess("chat_room"); err != nil {
			return err
		}
		r, ok := rooms.Get(c.PathInt("id"))
		if !ok {
			return core.ErrNotFound()
		}
		rooms.Delete(r.ID)
		chatPosts.DeleteBy("room", i(r.ID))
		a.AdminLog(c.UID(), "Чат", "Параметры комнат", "Удалена комната '"+r.Name+"'")
		return c.OK()
	}, core.Auth)
	a.POST("/api/chat-clear", func(c *core.Ctx) error {
		if err := c.NeedAccess("chat_clear"); err != nil {
			return err
		}
		chatPosts.DeleteWhere(func(*ChatPost) bool { return true })
		a.AdminLog(c.UID(), "Чат", "Очистка", "Очистка комнат от сообщений")
		return c.OK()
	}, core.Auth)
}

func botsCode(r Room) int {
	n := 0
	if r.Umnik {
		n |= 1
	}
	if r.Shutnik {
		n |= 2
	}
	return n
}

func itoa(n int) string { return store.I(int64(n)) }

// ---------- боты

func lastBot(room int64, pred func(*ChatPost) bool) (ChatPost, bool) {
	l, _ := chatPosts.Select(store.Opts[ChatPost]{Index: "room", Key: i(room), Desc: true, Limit: 1, Where: pred})
	if len(l) == 0 {
		return ChatPost{}, false
	}
	return l[0], true
}

func botPost(room int64, st int64, shut bool, voprosID int64, msg string) ChatPost {
	p := ChatPost{Room: room, Time: core.Now(), Msg: msg, UmnikSt: st, Shutnik: shut, Vopros: voprosID}
	p.ID = chatPosts.Insert(&p)
	return p
}

func randomOf[T any](list []T) T { return list[int(time.Now().UnixNano()/1000)%len(list)] }

func runBots(a *core.App, r Room) {
	if !r.Umnik && !r.Shutnik {
		return
	}
	botMu.Lock()
	defer botMu.Unlock()
	now := core.Now()
	if r.Shutnik {
		last, ok := lastBot(r.ID, func(p *ChatPost) bool { return p.Shutnik })
		if (!ok || last.Time < now-int64(a.Cfg.Int("shutnik_new"))) && shutki.Len() > 0 {
			s := randomOf(shutki.All())
			botPost(r.ID, 0, true, 0, s.Anek)
			a.Hub.Publish("chat:"+i(r.ID), "chat", map[string]any{"room": r.ID})
		}
	}
	if !r.Umnik {
		return
	}
	posted := false
	defer func() {
		if posted {
			a.Hub.Publish("chat:"+i(r.ID), "chat", map[string]any{"room": r.ID})
		}
	}()
	isUm := func(p *ChatPost) bool { return p.UmnikSt != 0 }
	last, has := lastBot(r.ID, isUm)
	umNew, umHelp, umTime := int64(a.Cfg.Int("umnik_new")), int64(a.Cfg.Int("umnik_help")), int64(a.Cfg.Int("umnik_time"))
	nm := a.Cfg.Str("chat_umnik")
	_ = nm

	if has && last.UmnikSt != 4 {
		v, vok := voprosy.Get(last.Vopros)
		if !vok { // вопрос удалён — закрываем
			last = botPost(r.ID, 4, false, 0, "Вопрос отозван.")
			posted = true
		} else {
			ans := strings.ToLower(v.Otvet)
			win, found := lastBotFirst(r.ID, last.ID, ans)
			if found {
				pts := map[int64]int64{1: 25, 2: 10, 3: 5}[last.UmnikSt]
				pods := map[int64]string{1: "не используя подсказок", 2: "используя одну подсказку", 3: "используя обе подсказки"}[last.UmnikSt]
				u := a.Ref(win.IDUser)
				msg := "Молодец, [b]" + u.Nick + "[/b].\nТы первым дал верный ответ: [b]" + v.Otvet + "[/b] " + pods + ".\n[b]" + u.Nick + "[/b] получает " + itoa(int(pts)) + " баллов.\nСледующий вопрос через " + itoa(int(umNew)) + " сек."
				a.AddBalls(win.IDUser, pts, 0)
				last = botPost(r.ID, 4, false, v.ID, msg)
				posted = true
			} else if q1, ok := lastBot(r.ID, func(p *ChatPost) bool { return p.UmnikSt == 1 }); ok && q1.Time < now-umTime {
				last = botPost(r.ID, 4, false, v.ID, "На вопрос никто не ответил.\nПравильный ответ: "+v.Otvet+".\nСледующий вопрос через "+itoa(int(umNew))+" сек.")
				posted = true
			}
		}
	}
	switch {
	case !has || (last.UmnikSt == 4 && last.Time < now-umNew):
		if voprosy.Len() == 0 {
			return
		}
		v := randomOf(voprosy.All())
		botPost(r.ID, 1, false, v.ID, "[b]Вопрос:[/b] \""+v.Vopros+"\"\n[b]Ответ:[/b] слово из "+itoa(utf8.RuneCountInString(v.Otvet))+" букв")
		posted = true
	case last.UmnikSt == 1 && last.Time < now-umHelp:
		hint(r.ID, last, 1, "Первая подсказка")
		posted = true
	case last.UmnikSt == 2 && last.Time < now-umHelp:
		hint(r.ID, last, 2, "Вторая подсказка")
		posted = true
	}
}

func hint(room int64, last ChatPost, open int, title string) {
	v, ok := voprosy.Get(last.Vopros)
	if !ok {
		return
	}
	rs := []rune(v.Otvet)
	var b strings.Builder
	for k, ch := range rs {
		if k < open {
			b.WriteRune(ch)
		} else {
			b.WriteByte('*')
		}
	}
	botPost(room, int64(open)+1, false, v.ID, "[b]Вопрос:[/b] \""+v.Vopros+"\"\n[b]"+title+":[/b] "+b.String()+" ("+itoa(len(rs))+" букв)")
}

// lastBotFirst — первое обычное сообщение после вопроса afterID, содержащее ответ.
func lastBotFirst(room, afterID int64, ans string) (ChatPost, bool) {
	if ans == "" {
		return ChatPost{}, false
	}
	l, _ := chatPosts.Select(store.Opts[ChatPost]{Index: "room", Key: i(room), Limit: 1,
		Where: func(p *ChatPost) bool {
			return p.ID > afterID && p.UmnikSt == 0 && !p.Shutnik && p.Privat == 0 && p.IDUser != 0 && strings.Contains(strings.ToLower(p.Msg), ans)
		}})
	if len(l) == 0 {
		return ChatPost{}, false
	}
	return l[0], true
}
