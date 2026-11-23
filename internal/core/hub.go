package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Hub — pub/sub для SSE. Темы: "user:<id>" (личные), "chat:<room>", "all".
type Hub struct {
	mu   sync.RWMutex
	subs map[*subscriber]struct{}
}

type subscriber struct {
	ch     chan []byte
	topics map[string]bool
}

func NewHub() *Hub { return &Hub{subs: map[*subscriber]struct{}{}} }

// Publish отправляет событие подписчикам темы. data сериализуется в JSON.
func (h *Hub) Publish(topic, event string, data any) {
	b, _ := json.Marshal(data)
	msg := []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", event, b))
	h.mu.RLock()
	for s := range h.subs {
		if s.topics[topic] {
			select {
			case s.ch <- msg:
			default: // медленный клиент — пропускаем
			}
		}
	}
	h.mu.RUnlock()
}

// ToUser — событие конкретному пользователю.
func (h *Hub) ToUser(uid int64, event string, data any) {
	h.Publish(fmt.Sprintf("user:%d", uid), event, data)
}

// Online — сколько подключений слушает тему.
func (h *Hub) Listeners(topic string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	n := 0
	for s := range h.subs {
		if s.topics[topic] {
			n++
		}
	}
	return n
}

// TopicAllowed — модуль может запретить подписку на тему (например приватные комнаты).
var topicGuards = map[string]func(c *Ctx, topic string) bool{}

// GuardTopic регистрирует проверку доступа к темам с префиксом (например "chat:").
func GuardTopic(prefix string, fn func(c *Ctx, topic string) bool) { topicGuards[prefix] = fn }

func (a *App) serveEvents(c *Ctx) error {
	fl, ok := c.W.(http.Flusher)
	if !ok {
		return Err(500, "streaming unsupported")
	}
	s := &subscriber{ch: make(chan []byte, 64), topics: map[string]bool{"all": true}}
	if c.user != nil {
		s.topics[fmt.Sprintf("user:%d", c.user.ID)] = true
	}
	for _, t := range strings.Split(c.Q("topics"), ",") {
		t = strings.TrimSpace(t)
		if t == "" || strings.HasPrefix(t, "user:") {
			continue
		}
		allowed := true
		for p, g := range topicGuards {
			if strings.HasPrefix(t, p) && !g(c, t) {
				allowed = false
			}
		}
		if allowed {
			s.topics[t] = true
		}
	}
	a.Hub.mu.Lock()
	a.Hub.subs[s] = struct{}{}
	a.Hub.mu.Unlock()
	defer func() {
		a.Hub.mu.Lock()
		delete(a.Hub.subs, s)
		a.Hub.mu.Unlock()
	}()
	h := c.W.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no")
	c.done = true
	c.W.WriteHeader(200)
	fmt.Fprint(c.W, ": ok\n\n")
	fl.Flush()
	tick := time.NewTicker(25 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-c.R.Context().Done():
			return nil
		case m := <-s.ch:
			c.W.Write(m)
			fl.Flush()
		case <-tick.C:
			fmt.Fprint(c.W, ": ping\n\n")
			fl.Flush()
		}
	}
}
