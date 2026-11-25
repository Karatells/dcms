package gamify

import (
	"encoding/json"
	"math/rand"
	"sort"
	"strings"
	"time"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

// UserStreak — ежедневная серия входов и выполненные задания пользователя.
type UserStreak struct {
	store.Base
	UserID        int64  `json:"user_id"`
	StreakDays    int    `json:"streak_days"`
	LastClaimDay  string `json:"last_claim_day"`  // YYYY-MM-DD
	TotalClaims   int    `json:"total_claims"`
	QuestsDay     string `json:"quests_day"`      // YYYY-MM-DD
	QuestsClaimed string `json:"quests_claimed"`  // comma-separated keys
}

// GameDuel — мини-игра / дуэль между двумя пользователями.
type GameDuel struct {
	store.Base
	Game    string `json:"game"`   // rps | ttt
	P1      int64  `json:"p1"`
	P2      int64  `json:"p2"`     // 0 = открытая дуэль
	Bet     int64  `json:"bet"`    // ставка в баллах
	Status  string `json:"status"` // open | playing | done
	Turn    int64  `json:"turn"`
	State   string `json:"state"`  // JSON состояния
	Winner  int64  `json:"winner"` // 0 = ничья
	Created int64  `json:"created"`
}

// ShopPurchase — купленный предмет оформления.
type ShopPurchase struct {
	store.Base
	UserID   int64  `json:"user_id"`
	ItemID   string `json:"item_id"`
	BoughtAt int64  `json:"bought_at"`
}

type shopItem struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"` // frame
	Name  string `json:"name"`
	Desc  string `json:"desc"`
	Price int64  `json:"price"` // в монетах
	Class string `json:"class"`
}

var shopCatalog = []shopItem{
	{ID: "frame_neon", Kind: "frame", Name: "Неоновое кольцо", Desc: "Яркое фиолетово-розовое свечение вокруг аватара", Price: 15, Class: "frame_neon"},
	{ID: "frame_fire", Kind: "frame", Name: "Огненная аура", Desc: "Пылающая оранжево-красная рамка", Price: 20, Class: "frame_fire"},
	{ID: "frame_ice", Kind: "frame", Name: "Ледяной кристалл", Desc: "Холодное бирюзовое сияние", Price: 20, Class: "frame_ice"},
	{ID: "frame_cyber", Kind: "frame", Name: "Киберпанк", Desc: "Контрастный неон-циан и маджента", Price: 25, Class: "frame_cyber"},
	{ID: "frame_cosmos", Kind: "frame", Name: "Космос", Desc: "Глубокий звёздный градиент", Price: 30, Class: "frame_cosmos"},
	{ID: "frame_gold", Kind: "frame", Name: "Императорское золото", Desc: "Роскошная золотая оправа с блеском", Price: 40, Class: "frame_gold"},
}

type dayReward struct {
	Day   int   `json:"day"`
	Balls int64 `json:"balls"`
	Coins int64 `json:"coins"`
}

var streakRewards = []dayReward{
	{1, 15, 0},
	{2, 25, 0},
	{3, 40, 0},
	{4, 60, 0},
	{5, 100, 1},
	{6, 150, 2},
	{7, 250, 5},
}

type dailyQuestDef struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	Desc  string `json:"desc"`
	Balls int64  `json:"balls"`
}

var dailyQuests = []dailyQuestDef{
	{Key: "visit_feed", Title: "Исследователь ленты", Desc: "Загляните в умную ленту и заберите ежедневный бонус активности", Balls: 20},
	{Key: "play_game", Title: "Игрок дня", Desc: "Примите участие или создайте дуэль в мини-играх", Balls: 30},
	{Key: "social_boost", Title: "Душа компании", Desc: "Поддержите сообщество — бонус за ежедневное участие", Balls: 25},
}

var (
	streaksT *store.Table[UserStreak, *UserStreak]
	duelsT   *store.Table[GameDuel, *GameDuel]
	shopT    *store.Table[ShopPurchase, *ShopPurchase]
)

func i(n int64) string { return store.I(n) }
func todayStr() string { return time.Now().UTC().Format("2006-01-02") }
func yesterdayStr() string { return time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02") }

func getOrCreateStreak(uid int64) UserStreak {
	if s, ok := streaksT.OneBy("user", i(uid)); ok {
		return s
	}
	s := UserStreak{UserID: uid}
	streaksT.Insert(&s)
	return s
}

type bsState struct {
	B1 [36]int `json:"b1"` // 0=вода, 1=корабль, 2=попадание, 3=промах
	B2 [36]int `json:"b2"`
}

func genBattleshipBoard() [36]int {
	ships := []int{3, 2, 2, 1, 1, 1}
	for attempt := 0; attempt < 50; attempt++ {
		var b [36]int
		okAll := true
		for _, sz := range ships {
			placed := false
			for tries := 0; tries < 80; tries++ {
				horiz := rand.Intn(2) == 0
				r := rand.Intn(6)
				c := rand.Intn(6)
				if horiz && c+sz > 6 {
					continue
				}
				if !horiz && r+sz > 6 {
					continue
				}
				can := true
				for k := 0; k < sz && can; k++ {
					rr, cc := r, c
					if horiz {
						cc += k
					} else {
						rr += k
					}
					for dr := -1; dr <= 1; dr++ {
						for dc := -1; dc <= 1; dc++ {
							nr, nc := rr+dr, cc+dc
							if nr >= 0 && nr < 6 && nc >= 0 && nc < 6 && b[nr*6+nc] != 0 {
								can = false
							}
						}
					}
				}
				if can {
					for k := 0; k < sz; k++ {
						rr, cc := r, c
						if horiz {
							cc += k
						} else {
							rr += k
						}
						b[rr*6+cc] = 1
					}
					placed = true
					break
				}
			}
			if !placed {
				okAll = false
				break
			}
		}
		if okAll {
			return b
		}
	}
	// детерминированный запасной вариант
	var fb [36]int
	for _, idx := range []int{0, 1, 2, 12, 13, 16, 17, 24, 29, 34} {
		fb[idx] = 1
	}
	return fb
}

func maskBSBoard(b [36]int, reveal bool) [36]int {
	if reveal {
		return b
	}
	var out [36]int
	for i, v := range b {
		if v == 1 {
			out[i] = 0
		} else {
			out[i] = v
		}
	}
	return out
}

func countShipsLeft(b [36]int) int {
	n := 0
	for _, v := range b {
		if v == 1 {
			n++
		}
	}
	return n
}

func duelView(a *core.App, d GameDuel, meID int64) map[string]any {
	var st map[string]any
	_ = json.Unmarshal([]byte(d.State), &st)
	if st == nil {
		st = map[string]any{}
	}
	// Скрываем ход соперника в RPS до завершения
	if d.Game == "rps" && d.Status != "done" {
		masked := map[string]any{}
		if v, ok := st["m1"]; ok && v != "" {
			if meID == d.P1 {
				masked["m1"] = v
			} else {
				masked["m1"] = "hidden"
			}
		}
		if v, ok := st["m2"]; ok && v != "" {
			if meID == d.P2 {
				masked["m2"] = v
			} else {
				masked["m2"] = "hidden"
			}
		}
		st = masked
	}
	// Для Морского боя скрываем неповреждённые корабли соперника до конца игры
	if d.Game == "bs" {
		var bs bsState
		_ = json.Unmarshal([]byte(d.State), &bs)
		done := d.Status == "done"
		if meID == d.P2 {
			st = map[string]any{
				"my_board":    bs.B2,
				"enemy_board": maskBSBoard(bs.B1, done),
				"my_left":     countShipsLeft(bs.B2),
				"enemy_left":  countShipsLeft(bs.B1),
			}
		} else if meID == d.P1 {
			st = map[string]any{
				"my_board":    bs.B1,
				"enemy_board": maskBSBoard(bs.B2, done),
				"my_left":     countShipsLeft(bs.B1),
				"enemy_left":  countShipsLeft(bs.B2),
			}
		} else {
			st = map[string]any{
				"my_board":    maskBSBoard(bs.B1, done),
				"enemy_board": maskBSBoard(bs.B2, done),
				"my_left":     countShipsLeft(bs.B1),
				"enemy_left":  countShipsLeft(bs.B2),
			}
		}
	}
	out := map[string]any{
		"id":      d.ID,
		"game":    d.Game,
		"p1":      a.Ref(d.P1),
		"bet":     d.Bet,
		"status":  d.Status,
		"turn":    d.Turn,
		"state":   st,
		"winner":  d.Winner,
		"created": d.Created,
	}
	if d.P2 > 0 {
		out["p2"] = a.Ref(d.P2)
	}
	return out
}

func checkTTTWinner(board [9]int) int {
	lines := [][3]int{{0, 1, 2}, {3, 4, 5}, {6, 7, 8}, {0, 3, 6}, {1, 4, 7}, {2, 5, 8}, {0, 4, 8}, {2, 4, 6}}
	for _, ln := range lines {
		if board[ln[0]] != 0 && board[ln[0]] == board[ln[1]] && board[ln[1]] == board[ln[2]] {
			return board[ln[0]]
		}
	}
	return 0
}

func resolveRPS(m1, m2 string) int {
	if m1 == m2 {
		return 0
	}
	if (m1 == "rock" && m2 == "scissors") || (m1 == "scissors" && m2 == "paper") || (m1 == "paper" && m2 == "rock") {
		return 1
	}
	return 2
}

// Register регистрирует ежедневные задания/Streak, мини-игры и магазин оформления.
func Register(a *core.App) {
	streaksT = store.NewTable[UserStreak](a.DB, "user_streaks").
		Index("user", func(x *UserStreak) string { return i(x.UserID) })
	duelsT = store.NewTable[GameDuel](a.DB, "game_duels").
		Index("status", func(x *GameDuel) string { return x.Status })
	shopT = store.NewTable[ShopPurchase](a.DB, "user_shop_items").
		Index("user", func(x *ShopPurchase) string { return i(x.UserID) }).
		Index("pair", func(x *ShopPurchase) string { return i(x.UserID) + ":" + x.ItemID })

	a.Describer("duel", func(id int64) (core.Desc, bool) {
		d, ok := duelsT.Get(id)
		if !ok {
			return core.Desc{}, false
		}
		gName := "Камень-ножницы-бумага"
		if d.Game == "ttt" {
			gName = "Крестики-нолики"
		} else if d.Game == "bs" {
			gName = "Морской бой"
		}
		txt := "Без ставки · нажмите, чтобы принять вызов"
		if d.Bet > 0 {
			txt = "Ставка: " + i(d.Bet) + " б. · нажмите, чтобы принять вызов"
		}
		return core.Desc{Title: "Дуэль · " + gName, Text: txt, Link: "/games"}, true
	})

	a.OnUserDelete(func(uid int64) {
		streaksT.DeleteBy("user", i(uid))
		shopT.DeleteBy("user", i(uid))
	})

	// ───────── Ежедневные задания и Streak
	a.GET("/api/quests", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		s := getOrCreateStreak(me.ID)
		today := todayStr()
		claimedToday := s.LastClaimDay == today
		effectiveStreak := s.StreakDays
		if !claimedToday && s.LastClaimDay != yesterdayStr() {
			effectiveStreak = 0
		}
		claimedSet := map[string]bool{}
		if s.QuestsDay == today && s.QuestsClaimed != "" {
			for _, k := range strings.Split(s.QuestsClaimed, ",") {
				claimedSet[k] = true
			}
		}
		qOut := make([]map[string]any, 0, len(dailyQuests))
		for _, q := range dailyQuests {
			qOut = append(qOut, map[string]any{
				"key":     q.Key,
				"title":   q.Title,
				"desc":    q.Desc,
				"balls":   q.Balls,
				"claimed": claimedSet[q.Key],
			})
		}
		return c.JSON(map[string]any{
			"streak":        effectiveStreak,
			"claimed_today": claimedToday,
			"total_claims":  s.TotalClaims,
			"rewards":       streakRewards,
			"quests":        qOut,
			"is_vip":        me.VIPUntil > core.Now(),
			"balls":         me.Balls,
			"money":         me.Money,
		})
	}, core.Auth)

	a.POST("/api/quests/checkin", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		s := getOrCreateStreak(me.ID)
		today := todayStr()
		if s.LastClaimDay == today {
			return core.ErrBad("Сегодняшняя награда уже получена")
		}
		nextStreak := 1
		if s.LastClaimDay == yesterdayStr() {
			nextStreak = s.StreakDays + 1
		}
		rew := streakRewards[(nextStreak-1)%len(streakRewards)]
		balls, coins := rew.Balls, rew.Coins
		if me.VIPUntil > core.Now() {
			balls = balls + balls/2
		}
		streaksT.Update(s.ID, func(x *UserStreak) {
			x.StreakDays = nextStreak
			x.LastClaimDay = today
			x.TotalClaims++
		})
		a.AddBalls(me.ID, balls, 0)
		if coins > 0 {
			a.T.Users.Update(me.ID, func(u *model.User) { u.Money += coins })
		}
		return c.OK("streak", nextStreak, "balls", balls, "coins", coins)
	}, core.Auth)

	a.POST("/api/quests/claim", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		var in struct {
			Key string `json:"key"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		var def *dailyQuestDef
		for idx := range dailyQuests {
			if dailyQuests[idx].Key == in.Key {
				def = &dailyQuests[idx]
				break
			}
		}
		if def == nil {
			return core.ErrNotFound("Задание не найдено")
		}
		s := getOrCreateStreak(me.ID)
		today := todayStr()
		var keys []string
		if s.QuestsDay == today && s.QuestsClaimed != "" {
			keys = strings.Split(s.QuestsClaimed, ",")
			for _, k := range keys {
				if k == def.Key {
					return core.ErrBad("Награда за это задание уже получена сегодня")
				}
			}
		}
		keys = append(keys, def.Key)
		streaksT.Update(s.ID, func(x *UserStreak) {
			x.QuestsDay = today
			x.QuestsClaimed = strings.Join(keys, ",")
		})
		a.AddBalls(me.ID, def.Balls, 0)
		return c.OK("balls", def.Balls)
	}, core.Auth)

	// ───────── Мини-игры и дуэли
	a.GET("/api/games/duels", func(c *core.Ctx) error {
		me := c.User()
		var meID int64
		if me != nil {
			meID = me.ID
		}
		all := duelsT.All()
		sort.Slice(all, func(x, y int) bool { return all[x].ID > all[y].ID })
		var openL, myL, recentL []map[string]any
		wins := map[int64]int{}
		for _, d := range all {
			if d.Status == "done" && d.Winner > 0 {
				wins[d.Winner]++
			}
			v := duelView(a, d, meID)
			if meID > 0 && (d.P1 == meID || d.P2 == meID) && d.Status != "done" {
				myL = append(myL, v)
			} else if d.Status == "open" && d.P2 == 0 {
				openL = append(openL, v)
			} else if d.Status == "done" && len(recentL) < 15 {
				recentL = append(recentL, v)
			}
		}
		type lbRow struct {
			uid  int64
			wins int
		}
		var lb []lbRow
		for uid, w := range wins {
			lb = append(lb, lbRow{uid, w})
		}
		sort.Slice(lb, func(x, y int) bool { return lb[x].wins > lb[y].wins })
		if len(lb) > 10 {
			lb = lb[:10]
		}
		lbOut := make([]map[string]any, 0, len(lb))
		for _, r := range lb {
			lbOut = append(lbOut, map[string]any{"user": a.Ref(r.uid), "wins": r.wins})
		}
		return c.JSON(map[string]any{
			"open":        openL,
			"mine":        myL,
			"recent":      recentL,
			"leaderboard": lbOut,
		})
	})

	a.POST("/api/games/duels", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		var in struct {
			Game         string `json:"game"`
			Bet          int64  `json:"bet"`
			OpponentNick string `json:"opponent_nick"`
			Move         string `json:"move"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if in.Game != "rps" && in.Game != "ttt" && in.Game != "bs" {
			return core.ErrBad("Выберите игру: rps, ttt или bs")
		}
		if in.Bet < 0 || in.Bet > 500 {
			return core.ErrBad("Ставка должна быть от 0 до 500 баллов")
		}
		if in.Bet > 0 && me.Balls < in.Bet {
			return core.ErrBad("Недостаточно баллов для ставки")
		}
		var p2 int64
		if nick := strings.TrimSpace(in.OpponentNick); nick != "" {
			u, ok := a.UserByNick(nick)
			if !ok || u.ID == me.ID {
				return core.ErrBad("Соперник не найден")
			}
			if !a.CanAccessPrivacy(me, u.ID, a.UserSettings(u.ID).PrivatDuels) {
				return core.ErrForbidden("Этот пользователь запретил вызывать себя на дуэли")
			}
			p2 = u.ID
		}
		var rawSt []byte
		if in.Game == "rps" {
			if in.Move != "rock" && in.Move != "paper" && in.Move != "scissors" {
				return core.ErrBad("Выберите ваш ход: камень, ножницы или бумага")
			}
			rawSt, _ = json.Marshal(map[string]any{"m1": in.Move})
		} else if in.Game == "bs" {
			rawSt, _ = json.Marshal(bsState{B1: genBattleshipBoard(), B2: genBattleshipBoard()})
		} else {
			rawSt, _ = json.Marshal(map[string]any{"board": [9]int{}})
		}
		if in.Bet > 0 {
			a.T.Users.Update(me.ID, func(u *model.User) { u.Balls -= in.Bet })
		}
		d := GameDuel{Game: in.Game, P1: me.ID, P2: p2, Bet: in.Bet, Status: "open", Turn: me.ID, State: string(rawSt), Created: core.Now()}
		duelsT.Insert(&d)
		if p2 > 0 {
			a.Notify(me.ID, p2, "duel", d.ID, true)
			a.Hub.ToUser(p2, "duel_update", duelView(a, d, p2))
		}
		return c.JSON(duelView(a, d, me.ID))
	}, core.Auth)

	a.POST("/api/games/duels/{id}/join", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		d, ok := duelsT.Get(c.PathInt("id"))
		if !ok || d.Status != "open" {
			return core.ErrBad("Дуэль уже началась или завершена")
		}
		if d.P1 == me.ID {
			return core.ErrBad("Нельзя играть с самим собой")
		}
		if d.P2 > 0 && d.P2 != me.ID {
			return core.ErrForbidden("Эта дуэль предназначена другому игроку")
		}
		if d.Bet > 0 && me.Balls < d.Bet {
			return core.ErrBad("Недостаточно баллов для ставки")
		}
		var in struct {
			Move string `json:"move"`
		}
		_ = c.Bind(&in)
		if d.Game == "rps" {
			if in.Move != "rock" && in.Move != "paper" && in.Move != "scissors" {
				return core.ErrBad("Выберите ход: rock, paper или scissors")
			}
		}
		if d.Bet > 0 {
			a.T.Users.Update(me.ID, func(u *model.User) { u.Balls -= d.Bet })
		}
		if d.Game == "rps" {
			var st map[string]string
			_ = json.Unmarshal([]byte(d.State), &st)
			if st == nil {
				st = map[string]string{}
			}
			st["m2"] = in.Move
			res := resolveRPS(st["m1"], st["m2"])
			var winID int64
			if res == 1 {
				winID = d.P1
				if d.Bet > 0 {
					a.AddBalls(d.P1, d.Bet*2, 0)
				}
			} else if res == 2 {
				winID = me.ID
				if d.Bet > 0 {
					a.AddBalls(me.ID, d.Bet*2, 0)
				}
			} else if d.Bet > 0 {
				a.AddBalls(d.P1, d.Bet, 0)
				a.AddBalls(me.ID, d.Bet, 0)
			}
			rawSt, _ := json.Marshal(st)
			duelsT.Update(d.ID, func(x *GameDuel) { x.P2 = me.ID; x.Status = "done"; x.State = string(rawSt); x.Winner = winID })
		} else {
			duelsT.Update(d.ID, func(x *GameDuel) { x.P2 = me.ID; x.Status = "playing"; x.Turn = d.P1 })
		}
		upd, _ := duelsT.Get(d.ID)
		a.Hub.ToUser(upd.P1, "duel_update", duelView(a, upd, upd.P1))
		a.Hub.ToUser(upd.P2, "duel_update", duelView(a, upd, upd.P2))
		return c.JSON(duelView(a, upd, me.ID))
	}, core.Auth)

	a.POST("/api/games/duels/{id}/move", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		d, ok := duelsT.Get(c.PathInt("id"))
		if !ok || d.Status != "playing" || (d.Game != "ttt" && d.Game != "bs") {
			return core.ErrBad("Игра недоступна для хода")
		}
		if d.Turn != me.ID {
			return core.ErrBad("Сейчас ход соперника")
		}
		var in struct {
			Cell int `json:"cell"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if d.Game == "bs" {
			if in.Cell < 0 || in.Cell >= 36 {
				return core.ErrBad("Некорректная клетка")
			}
			var bs bsState
			_ = json.Unmarshal([]byte(d.State), &bs)
			target := &bs.B2
			oppID := d.P2
			if me.ID == d.P2 {
				target = &bs.B1
				oppID = d.P1
			}
			if target[in.Cell] == 2 || target[in.Cell] == 3 {
				return core.ErrBad("Вы уже стреляли в эту клетку")
			}
			nextTurn := oppID
			hit := false
			if target[in.Cell] == 1 {
				target[in.Cell] = 2
				hit = true
				nextTurn = me.ID // при попадании — повторный выстрел!
			} else {
				target[in.Cell] = 3
			}
			status := "playing"
			var winID int64
			if hit && countShipsLeft(*target) == 0 {
				status = "done"
				winID = me.ID
				if d.Bet > 0 {
					a.AddBalls(me.ID, d.Bet*2, 0)
				}
			}
			rawSt, _ := json.Marshal(bs)
			duelsT.Update(d.ID, func(x *GameDuel) {
				x.State = string(rawSt)
				x.Status = status
				x.Turn = nextTurn
				x.Winner = winID
			})
			upd, _ := duelsT.Get(d.ID)
			a.Hub.ToUser(upd.P1, "duel_update", duelView(a, upd, upd.P1))
			a.Hub.ToUser(upd.P2, "duel_update", duelView(a, upd, upd.P2))
			return c.JSON(duelView(a, upd, me.ID))
		}
		if in.Cell < 0 || in.Cell > 8 {
			return core.ErrBad("Некорректная клетка")
		}
		var st struct {
			Board [9]int `json:"board"`
		}
		_ = json.Unmarshal([]byte(d.State), &st)
		if st.Board[in.Cell] != 0 {
			return core.ErrBad("Клетка уже занята")
		}
		mark := 1
		nextTurn := d.P2
		if me.ID == d.P2 {
			mark = 2
			nextTurn = d.P1
		}
		st.Board[in.Cell] = mark
		w := checkTTTWinner(st.Board)
		full := true
		for _, v := range st.Board {
			if v == 0 {
				full = false
				break
			}
		}
		status := "playing"
		var winID int64
		if w == 1 {
			status = "done"
			winID = d.P1
			if d.Bet > 0 {
				a.AddBalls(d.P1, d.Bet*2, 0)
			}
		} else if w == 2 {
			status = "done"
			winID = d.P2
			if d.Bet > 0 {
				a.AddBalls(d.P2, d.Bet*2, 0)
			}
		} else if full {
			status = "done"
			if d.Bet > 0 {
				a.AddBalls(d.P1, d.Bet, 0)
				a.AddBalls(d.P2, d.Bet, 0)
			}
		}
		rawSt, _ := json.Marshal(st)
		duelsT.Update(d.ID, func(x *GameDuel) {
			x.State = string(rawSt)
			x.Status = status
			x.Turn = nextTurn
			x.Winner = winID
		})
		upd, _ := duelsT.Get(d.ID)
		a.Hub.ToUser(upd.P1, "duel_update", duelView(a, upd, upd.P1))
		a.Hub.ToUser(upd.P2, "duel_update", duelView(a, upd, upd.P2))
		return c.JSON(duelView(a, upd, me.ID))
	}, core.Auth)

	// ───────── Магазин оформления (рамки аватаров)
	a.GET("/api/shop", func(c *core.Ctx) error {
		me := c.User()
		isVIP := me != nil && me.VIPUntil > core.Now()
		owned := map[string]bool{}
		var activeFrame string
		var money int64
		if me != nil {
			activeFrame = me.AvatarFrame
			money = me.Money
			for _, p := range shopT.By("user", i(me.ID)) {
				owned[p.ItemID] = true
			}
		}
		items := make([]map[string]any, 0, len(shopCatalog))
		for _, it := range shopCatalog {
			price := it.Price
			if isVIP {
				price = max(1, price*70/100)
			}
			items = append(items, map[string]any{
				"id":        it.ID,
				"kind":      it.Kind,
				"name":      it.Name,
				"desc":      it.Desc,
				"base_price": it.Price,
				"price":     price,
				"class":     it.Class,
				"owned":     owned[it.ID],
				"equipped":  activeFrame == it.ID,
			})
		}
		return c.JSON(map[string]any{"items": items, "is_vip": isVIP, "money": money, "active_frame": activeFrame})
	})

	a.POST("/api/shop/buy", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		var in struct {
			ItemID string `json:"item_id"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		var item *shopItem
		for idx := range shopCatalog {
			if shopCatalog[idx].ID == in.ItemID {
				item = &shopCatalog[idx]
				break
			}
		}
		if item == nil {
			return core.ErrNotFound("Товар не найден")
		}
		if _, ok := shopT.OneBy("pair", i(me.ID)+":"+item.ID); ok {
			return core.ErrBad("Этот предмет уже куплен")
		}
		price := item.Price
		if me.VIPUntil > core.Now() {
			price = max(1, price*70/100)
		}
		if me.Money < price {
			return core.ErrBad("Недостаточно монет")
		}
		a.T.Users.Update(me.ID, func(u *model.User) {
			u.Money -= price
			u.AvatarFrame = item.ID
		})
		shopT.Insert(&ShopPurchase{UserID: me.ID, ItemID: item.ID, BoughtAt: core.Now()})
		a.DropUserCache(me.ID)
		return c.OK("equipped", item.ID)
	}, core.Auth)

	a.POST("/api/shop/equip", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		var in struct {
			ItemID string `json:"item_id"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if in.ItemID != "" {
			if _, ok := shopT.OneBy("pair", i(me.ID)+":"+in.ItemID); !ok {
				return core.ErrForbidden("Сначала купите эту рамку")
			}
		}
		a.T.Users.Update(me.ID, func(u *model.User) { u.AvatarFrame = in.ItemID })
		a.DropUserCache(me.ID)
		return c.OK("equipped", in.ItemID)
	}, core.Auth)
}
