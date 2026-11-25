// Package calls — сигналинг 1-на-1 аудио/видеозвонков WebRTC поверх SSE + REST и выдача ICE (STUN/TURN) конфигурации.
package calls

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"dcms/internal/core"
)

type callSession struct {
	ID        string
	Caller    int64
	Callee    int64
	Kind      string // audio | video
	State     string // ringing | active
	CreatedAt time.Time
	UpdatedAt time.Time
}

var (
	mu    sync.Mutex
	calls = map[string]*callSession{}
)

func init() {
	core.Defaults["calls_stun"] = "stun:stun.l.google.com:19302"
	core.Defaults["calls_turn_url"] = ""
	core.Defaults["calls_turn_user"] = ""
	core.Defaults["calls_turn_pass"] = ""
}

func newCallID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func cleanupLocked(now time.Time) {
	for id, s := range calls {
		if s.State == "ringing" && now.Sub(s.CreatedAt) > 70*time.Second {
			delete(calls, id)
		} else if now.Sub(s.UpdatedAt) > 6*time.Hour {
			delete(calls, id)
		}
	}
}

func userBusyLocked(uid int64) bool {
	for _, s := range calls {
		if s.Caller == uid || s.Callee == uid {
			return true
		}
	}
	return false
}

func Register(a *core.App) {
	// ICE-серверы (STUN + опциональный TURN из настроек админки)
	a.GET("/api/calls/ice", func(c *core.Ctx) error {
		if _, err := c.Need(); err != nil {
			return err
		}
		servers := []map[string]any{}
		stunRaw := strings.TrimSpace(a.Cfg.Str("calls_stun"))
		if stunRaw != "" {
			var urls []string
			for _, u := range strings.Split(stunRaw, ",") {
				u = strings.TrimSpace(u)
				if u != "" {
					urls = append(urls, u)
				}
			}
			if len(urls) > 0 {
				servers = append(servers, map[string]any{"urls": urls})
			}
		}
		turnURL := strings.TrimSpace(a.Cfg.Str("calls_turn_url"))
		if turnURL != "" {
			t := map[string]any{"urls": turnURL}
			if u := strings.TrimSpace(a.Cfg.Str("calls_turn_user")); u != "" {
				t["username"] = u
			}
			if p := strings.TrimSpace(a.Cfg.Str("calls_turn_pass")); p != "" {
				t["credential"] = p
			}
			servers = append(servers, t)
		}
		return c.JSON(map[string]any{"ice_servers": servers})
	}, core.Auth)

	// Начать звонок пользователю (передаёт SDP offer собеседнику через SSE call_signal)
	a.POST("/api/calls/start", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		if b, ok := a.ActiveBan(me.ID, "all"); ok {
			return core.ErrForbidden("Вы заблокированы: " + b.Prich)
		}
		var in struct {
			To   int64  `json:"to"`
			Kind string `json:"kind"` // audio | video
			SDP  any    `json:"sdp"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		if in.To <= 0 || in.To == me.ID {
			return core.ErrBad("Некорректный получатель звонка")
		}
		if _, ok := a.T.Users.Get(in.To); !ok {
			return core.ErrNotFound("Пользователь не найден")
		}
		kind := "audio"
		if in.Kind == "video" {
			kind = "video"
		}
		now := time.Now()
		mu.Lock()
		cleanupLocked(now)
		if userBusyLocked(me.ID) {
			mu.Unlock()
			return core.ErrBad("У вас уже есть активный звонок")
		}
		if userBusyLocked(in.To) {
			mu.Unlock()
			return core.ErrBad("Пользователь сейчас разговаривает по другой линии")
		}
		id := newCallID()
		calls[id] = &callSession{
			ID:        id,
			Caller:    me.ID,
			Callee:    in.To,
			Kind:      kind,
			State:     "ringing",
			CreatedAt: now,
			UpdatedAt: now,
		}
		mu.Unlock()

		a.Hub.ToUser(in.To, "call_signal", map[string]any{
			"type":    "incoming",
			"call_id": id,
			"kind":    kind,
			"from":    a.Ref(me.ID),
			"sdp":     in.SDP,
		})
		return c.JSON(map[string]any{"call_id": id})
	}, core.Auth)

	// Обмен сигналами WebRTC: answer, ice, reject, hangup, busy, toggle_video
	a.POST("/api/calls/signal", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		var in struct {
			CallID    string `json:"call_id"`
			Type      string `json:"type"`
			SDP       any    `json:"sdp"`
			Candidate any    `json:"candidate"`
			VideoOn   *bool  `json:"video_on"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		now := time.Now()
		mu.Lock()
		cleanupLocked(now)
		s, ok := calls[in.CallID]
		if !ok || (s.Caller != me.ID && s.Callee != me.ID) {
			mu.Unlock()
			return c.OK("gone", true)
		}
		peerID := s.Callee
		if me.ID == s.Callee {
			peerID = s.Caller
		}
		switch in.Type {
		case "answer":
			s.State = "active"
			s.UpdatedAt = now
		case "ice", "toggle_video":
			s.UpdatedAt = now
		case "reject", "hangup", "busy":
			delete(calls, in.CallID)
		default:
			mu.Unlock()
			return core.ErrBad("Неизвестный тип сигнала")
		}
		mu.Unlock()

		payload := map[string]any{
			"type":    in.Type,
			"call_id": in.CallID,
			"from_id": me.ID,
		}
		if in.SDP != nil {
			payload["sdp"] = in.SDP
		}
		if in.Candidate != nil {
			payload["candidate"] = in.Candidate
		}
		if in.VideoOn != nil {
			payload["video_on"] = *in.VideoOn
		}
		a.Hub.ToUser(peerID, "call_signal", payload)
		return c.OK()
	}, core.Auth)
}
