package core

import (
	"strconv"
	"strings"
	"sync"

	"dcms/internal/model"
)

// Defaults — настройки системы по умолчанию (из sys/dat/default.ini старого движка + новые).
var Defaults = map[string]string{
	"site_name": "DCMS Social", "meta_keywords": "DCMS Social", "meta_description": "Социальная сеть",
	"set_show_icon": "1", "p_str": "10",
	"chat_umnik": "Умник", "chat_shutnik": "Шутник", "time_chat": "30",
	"umnik_new": "30", "umnik_help": "30", "umnik_time": "90", "shutnik_new": "15",
	"echo_rassh_forum": "0", "echo_rassh": "1", "show_num_post": "1", "show_away": "0",
	"write_guest": "0", "obmen_limit_up": "50", "downloads_select": "0", "st": "1",
	"max_upload_foto_x": "2024", "max_upload_foto_y": "2024",
	"max_upload_foto_mb": "12", "max_upload_file_mb": "100", "max_upload_screen_mb": "8",
	"max_upload_forum_mb": "10", "max_upload_forum_count": "5",
	"max_upload_gift_mb": "2", "max_upload_smile_mb": "2",
	"guest_select": "0", "reg_select": "open", // open | close | open_mail (с активацией по почте)
	"loads_new_file_hour": "24", "antidos": "1", "antimat": "1",
	"bb_i": "1", "bb_u": "1", "bb_b": "1", "bb_s": "1", "bb_big": "1", "bb_small": "1", "bb_code": "1",
	"bb_red": "1", "bb_yellow": "1", "bb_green": "1", "bb_blue": "1", "bb_white": "1", "bb_size": "1",
	"bb_color": "1", "bb_bg": "1", "bb_spoiler": "1", "bb_cut": "1", "bb_rainbow": "1", "bb_mono": "1",
	"bb_quote": "1", "bb_box": "1", "bb_center": "1", "bb_right": "1", "bb_hr": "1",
	"bb_http": "1", "bb_url": "1", "bb_img": "1",
	"exit": "1", "trust_proxy": "0",
	"theme_default": "auto", "accent_default": "#6366f1",
	"mail_backup": "", "smtp_from": "",
	"new_user_balls": "0", "new_user_money": "0",
}

// Config — настройки системы (KV в БД + значения по умолчанию).
type Config struct {
	a  *App
	mu sync.RWMutex
	m  map[string]string
}

func newConfig(a *App) *Config {
	c := &Config{a: a, m: map[string]string{}}
	for k, v := range Defaults {
		c.m[k] = v
	}
	return c
}

func (c *Config) load() {
	for _, kv := range c.a.T.KV.All() {
		c.mu.Lock()
		c.m[kv.Key] = kv.Value
		c.mu.Unlock()
	}
}

// Str — строковая настройка.
func (c *Config) Str(k string) string { c.mu.RLock(); defer c.mu.RUnlock(); return c.m[k] }

// Int — числовая настройка.
func (c *Config) Int(k string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(c.Str(k)))
	return n
}

// Bool — флаг (1/true/on).
func (c *Config) Bool(k string) bool {
	switch strings.ToLower(strings.TrimSpace(c.Str(k))) {
	case "1", "true", "on", "yes":
		return true
	}
	return false
}

// Set сохраняет настройку.
func (c *Config) Set(k, v string) {
	c.mu.Lock()
	c.m[k] = v
	c.mu.Unlock()
	if kv, ok := c.a.T.KV.OneBy("key", k); ok {
		c.a.T.KV.Update(kv.ID, func(x *model.KV) { x.Value = v })
	} else {
		c.a.T.KV.Insert(&model.KV{Key: k, Value: v})
	}
}

// All — копия всех настроек.
func (c *Config) All() map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]string, len(c.m))
	for k, v := range c.m {
		out[k] = v
	}
	return out
}

// Public — настройки, безопасные для клиента.
func (c *Config) Public() map[string]any {
	return map[string]any{
		"site_name": c.Str("site_name"), "reg_select": c.Str("reg_select"), "guest_select": c.Str("guest_select"),
		"p_str": c.Int("p_str"), "theme_default": c.Str("theme_default"), "accent_default": c.Str("accent_default"),
		"write_guest": c.Bool("write_guest"), "time_chat": c.Int("time_chat"),
		"bb": map[string]bool{
			"i": c.Bool("bb_i"), "b": c.Bool("bb_b"), "u": c.Bool("bb_u"), "s": c.Bool("bb_s"),
			"big": c.Bool("bb_big"), "small": c.Bool("bb_small"),
			"red": c.Bool("bb_red"), "yellow": c.Bool("bb_yellow"), "green": c.Bool("bb_green"), "blue": c.Bool("bb_blue"), "white": c.Bool("bb_white"),
			"color": c.Bool("bb_color"), "bg": c.Bool("bb_bg"), "spoiler": c.Bool("bb_spoiler"), "cut": c.Bool("bb_cut"),
			"rainbow": c.Bool("bb_rainbow"), "mono": c.Bool("bb_mono"), "quote": c.Bool("bb_quote"), "box": c.Bool("bb_box"),
			"center": c.Bool("bb_center"), "right": c.Bool("bb_right"), "hr": c.Bool("bb_hr"),
			"size": c.Bool("bb_size"), "url": c.Bool("bb_url"), "img": c.Bool("bb_img"), "code": c.Bool("bb_code"),
		},
	}
}
