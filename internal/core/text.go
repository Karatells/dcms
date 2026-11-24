package core

import (
	"html"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// Smile — смайл для подстановки.
type Smile struct{ Code, URL string }

// Text — рендер пользовательского текста (аналог output_text(): экранирование, ссылки, смайлы, BBCode, переносы).
type Text struct {
	a       *App
	mu      sync.RWMutex
	smiles  func() []Smile
	smCache []Smile
	smAt    int64
}

func newText(a *App) *Text { return &Text{a: a} }

// SetSmiles регистрирует поставщика смайлов (модуль smiles).
func (t *Text) SetSmiles(fn func() []Smile) { t.smiles = fn }

var (
	reURL     = regexp.MustCompile(`(^|[\s>(])((?:https?)://[^\s<>"']+)`)
	reTag     = regexp.MustCompile(`(?is)\[(i|b|u|s|big|small|red|yellow|green|blue|white|mono|spoiler|rainbow|center|right)\](.*?)\[/(?:i|b|u|s|big|small|red|yellow|green|blue|white|mono|spoiler|rainbow|center|right)\]`)
	reColor   = regexp.MustCompile(`(?is)\[color=(#[0-9a-f]{3}(?:[0-9a-f]{3})?)\](.*?)\[/color\]`)
	reBg      = regexp.MustCompile(`(?is)\[bg=(#[0-9a-f]{3}(?:[0-9a-f]{3})?)\](.*?)\[/bg\]`)
	reCut     = regexp.MustCompile(`(?is)\[cut(?:=([^\]]{0,60}))?\](.*?)\[/cut\]`)
	reBox     = regexp.MustCompile(`(?is)\[box(?:=(info|ok|warn|danger))?\](.*?)\[/box\]`)
	reSize    = regexp.MustCompile(`(?is)\[size=(\d{1,2})\](.*?)\[/size\]`)
	reURLBB   = regexp.MustCompile(`(?is)\[url=((?:https?://|/[^/])[^\]\s"']*)\](.*?)\[/url\]`)
	reURLB2   = regexp.MustCompile(`(?is)\[url\]((?:https?://|/[^/])[^\[\s"']*)\[/url\]`)
	reImg     = regexp.MustCompile(`(?is)\[img\]((?:https?://|/[^/])[^\[\s"'<>]*)\[/img\]`)
	reCode    = regexp.MustCompile(`(?is)\[code\](.*?)\[/code\]`)
	reQuote   = regexp.MustCompile(`(?is)\[quote(?:=([^\]]{0,40}))?\](.*?)\[/quote\]`)
	reBR      = regexp.MustCompile(`(?i)\[br\]`)
	reHR      = regexp.MustCompile(`(?i)\[hr\]`)
	mat       = regexp.MustCompile(`(?i)(\b|[а-яё]*)(хуй|хуе|хуё|хуя|пизд|ебан|ебат|ёбан|ебал|ебну|блядь|бляд|сука|сучк|мудак|мудил|залуп|пидор|пидар|гондон)([а-яё]*)`)
)

var tagHTML = map[string]string{
	"i": `<em>%s</em>`, "b": `<strong>%s</strong>`, "u": `<span style="text-decoration:underline">%s</span>`,
	"s": `<del>%s</del>`, "mono": `<code class="bb-mono">%s</code>`,
	"spoiler": `<span class="bb-spoiler" tabindex="0" title="Нажмите, чтобы показать">%s</span>`,
	"rainbow": `<span class="bb-rainbow">%s</span>`,
	"center":  `<div class="bb-center">%s</div>`, "right": `<div class="bb-right">%s</div>`,
	"big": `<span style="font-size:larger">%s</span>`, "small": `<span style="font-size:smaller">%s</span>`,
	"red": `<span style="color:#e11d48">%s</span>`, "yellow": `<span style="color:#ca8a04">%s</span>`,
	"green": `<span style="color:#16a34a">%s</span>`, "blue": `<span style="color:#2563eb">%s</span>`, "white": `<span style="color:#fff;text-shadow:0 0 2px #000">%s</span>`,
}

// Censor заменяет нецензурные слова на звёздочки (настройка antimat).
func (t *Text) Censor(s string) string {
	if !t.a.Cfg.Bool("antimat") {
		return s
	}
	return mat.ReplaceAllStringFunc(s, func(m string) string { return strings.Repeat("*", len([]rune(m))) })
}

// Render превращает сырой текст в безопасный HTML.
func (t *Text) Render(raw string) string { return t.RenderOpts(raw, true, true, true) }

// RenderOpts — рендер с флагами: bbcode, smiles, links.
func (t *Text) RenderOpts(raw string, bb, smiles, links bool) string {
	cfg := t.a.Cfg
	s := html.EscapeString(raw)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	var slots []string
	hold := func(frag string) string {
		slots = append(slots, frag)
		return "\x00S" + strconv.Itoa(len(slots)-1) + "\x00"
	}
	if bb && cfg.Bool("bb_code") {
		s = reCode.ReplaceAllStringFunc(s, func(m string) string {
			inner := reCode.FindStringSubmatch(m)[1]
			return hold(`<pre class="bb-code"><code>` + inner + `</code></pre>`)
		})
	}
	if bb && cfg.Bool("bb_img") {
		s = reImg.ReplaceAllStringFunc(s, func(m string) string {
			sm := reImg.FindStringSubmatch(m)
			return hold(`<img class="bb-img" src="` + sm[1] + `" alt="" loading="lazy">`)
		})
	}
	if bb && cfg.Bool("bb_url") {
		s = reURLBB.ReplaceAllStringFunc(s, func(m string) string {
			sm := reURLBB.FindStringSubmatch(m)
			return hold(`<a href="` + sm[1] + `" target="_blank" rel="noopener nofollow ugc">` + sm[2] + `</a>`)
		})
		s = reURLB2.ReplaceAllStringFunc(s, func(m string) string {
			sm := reURLB2.FindStringSubmatch(m)
			return hold(`<a href="` + sm[1] + `" target="_blank" rel="noopener nofollow ugc">` + sm[1] + `</a>`)
		})
	}
	if links && cfg.Bool("bb_http") {
		s = reURL.ReplaceAllStringFunc(s, func(m string) string {
			sm := reURL.FindStringSubmatch(m)
			return sm[1] + hold(`<a href="`+sm[2]+`" target="_blank" rel="noopener nofollow ugc">`+sm[2]+`</a>`)
		})
	}
	if smiles {
		s = t.applySmiles(s)
	}
	if bb {
		s = reBR.ReplaceAllString(s, "<br>")
		if cfg.Bool("bb_hr") {
			s = reHR.ReplaceAllString(s, `<hr class="bb-hr">`)
		}
		// несколько проходов для вложенных тегов
		for i := 0; i < 4; i++ {
			s = reTag.ReplaceAllStringFunc(s, func(m string) string {
				sm := reTag.FindStringSubmatch(m)
				tag := strings.ToLower(sm[1])
				if !cfg.Bool("bb_" + tag) {
					return m
				}
				return strings.Replace(tagHTML[tag], "%s", sm[2], 1)
			})
			if cfg.Bool("bb_color") {
				s = reColor.ReplaceAllStringFunc(s, func(m string) string {
					sm := reColor.FindStringSubmatch(m)
					return `<span style="color:` + sm[1] + `">` + sm[2] + `</span>`
				})
			}
			if cfg.Bool("bb_bg") {
				s = reBg.ReplaceAllStringFunc(s, func(m string) string {
					sm := reBg.FindStringSubmatch(m)
					return `<span class="bb-bg" style="background:` + sm[1] + `">` + sm[2] + `</span>`
				})
			}
		}
		if cfg.Bool("bb_size") {
			s = reSize.ReplaceAllStringFunc(s, func(m string) string {
				sm := reSize.FindStringSubmatch(m)
				sz, _ := strconv.Atoi(sm[1])
				if sz < 10 {
					sz = 10
				} else if sz > 36 {
					sz = 36
				}
				return `<span style="font-size:` + strconv.Itoa(sz) + `px">` + sm[2] + `</span>`
			})
		}
		if cfg.Bool("bb_cut") {
			s = reCut.ReplaceAllStringFunc(s, func(m string) string {
				sm := reCut.FindStringSubmatch(m)
				title := strings.TrimSpace(sm[1])
				if title == "" {
					title = "Показать скрытый текст"
				}
				return `<details class="bb-cut"><summary>` + title + `</summary><div class="bb-cut-body">` + sm[2] + `</div></details>`
			})
		}
		if cfg.Bool("bb_box") {
			s = reBox.ReplaceAllStringFunc(s, func(m string) string {
				sm := reBox.FindStringSubmatch(m)
				kind := strings.ToLower(sm[1])
				if kind == "" {
					kind = "info"
				}
				return `<div class="bb-box bb-box-` + kind + `">` + sm[2] + `</div>`
			})
		}
		if cfg.Bool("bb_quote") {
			s = reQuote.ReplaceAllStringFunc(s, func(m string) string {
				sm := reQuote.FindStringSubmatch(m)
				who := ""
				if sm[1] != "" {
					who = `<cite>` + sm[1] + `</cite>`
				}
				return `<blockquote class="bb-quote">` + who + sm[2] + `</blockquote>`
			})
		}
	}
	s = strings.ReplaceAll(s, "\n", "<br>")
	for i := len(slots) - 1; i >= 0; i-- {
		s = strings.Replace(s, "\x00S"+strconv.Itoa(i)+"\x00", slots[i], 1)
	}
	return s
}

func (t *Text) applySmiles(s string) string {
	if t.smiles == nil {
		return s
	}
	now := Now()
	t.mu.RLock()
	list, at := t.smCache, t.smAt
	t.mu.RUnlock()
	if list == nil || now-at > 60 {
		list = t.smiles()
		t.mu.Lock()
		t.smCache, t.smAt = list, now
		t.mu.Unlock()
	}
	for _, sm := range list {
		code := html.EscapeString(sm.Code)
		if code == "" || !strings.Contains(s, code) {
			continue
		}
		s = strings.ReplaceAll(s, code, `<img class="smile" src="`+html.EscapeString(sm.URL)+`" alt="`+code+`">`)
	}
	return s
}

// ResetSmiles сбрасывает кеш смайлов после правки в админке.
func (t *Text) ResetSmiles() { t.mu.Lock(); t.smCache = nil; t.mu.Unlock() }

var reStripBB = regexp.MustCompile(`\[/?[a-z]+(?:=[^\]]*)?\]`)

// Plain — текст без BBCode, обрезанный до n символов (для превью в лентах).
func (t *Text) Plain(raw string, n int) string {
	s := reStripBB.ReplaceAllString(raw, "")
	s = strings.Join(strings.Fields(s), " ")
	return trunc(s, n)
}

// Clean — нормализует ввод: убирает управляющие символы, обрезает пробелы, цензурит.
func (t *Text) Clean(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if r == '\n' || r == '\t' || r >= 32 && r != 127 {
			sb.WriteRune(r)
		}
	}
	return t.Censor(strings.TrimSpace(sb.String()))
}
