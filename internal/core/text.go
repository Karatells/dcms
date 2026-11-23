package core

import (
	"html"
	"regexp"
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
	reURL   = regexp.MustCompile(`(^|[\s>(])((?:https?|ftp)://[^\s<>"']+)`)
	reTag   = regexp.MustCompile(`(?is)\[(i|b|u|big|small|red|yellow|green|blue|white)\](.*?)\[/(?:i|b|u|big|small|red|yellow|green|blue|white)\]`)
	reSize  = regexp.MustCompile(`(?is)\[size=(\d{1,2})\](.*?)\[/size\]`)
	reURLBB = regexp.MustCompile(`(?is)\[url=((?:https?|ftp)://[^\]\s"']+|/[^\]\s"']*)\](.*?)\[/url\]`)
	reURLB2 = regexp.MustCompile(`(?is)\[url\]((?:https?|ftp)://[^\[\s"']+|/[^\[\s"']*)\[/url\]`)
	reImg   = regexp.MustCompile(`(?is)\[img\]((?:https?://|/)[^\[\s"'<>]+)\[/img\]`)
	reCode  = regexp.MustCompile(`(?is)\[code\](.*?)\[/code\]`)
	reQuote = regexp.MustCompile(`(?is)\[quote(?:=([^\]]{0,40}))?\](.*?)\[/quote\]`)
	reBR    = regexp.MustCompile(`(?i)\[br\]`)
	mat     = regexp.MustCompile(`(?i)(\b|[а-яё]*)(хуй|хуе|хуё|хуя|пизд|ебан|ебат|ёбан|ебал|ебну|блядь|бляд|сука|сучк|мудак|мудил|залуп|пидор|пидар|гондон)([а-яё]*)`)
)

var tagHTML = map[string]string{
	"i": `<em>%s</em>`, "b": `<strong>%s</strong>`, "u": `<span style="text-decoration:underline">%s</span>`,
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
	var codes []string
	if bb && cfg.Bool("bb_code") {
		s = reCode.ReplaceAllStringFunc(s, func(m string) string {
			inner := reCode.FindStringSubmatch(m)[1]
			codes = append(codes, `<pre class="bb-code"><code>`+inner+`</code></pre>`)
			return "\x00C" + string(rune('A'+len(codes)-1)) + "\x00"
		})
	}
	if bb && cfg.Bool("bb_img") {
		s = reImg.ReplaceAllString(s, `<img class="bb-img" src="$1" alt="" loading="lazy">`)
	}
	if bb && cfg.Bool("bb_url") {
		s = reURLBB.ReplaceAllString(s, `<a href="$1" target="_blank" rel="noopener nofollow ugc">$2</a>`)
		s = reURLB2.ReplaceAllString(s, `<a href="$1" target="_blank" rel="noopener nofollow ugc">$1</a>`)
	}
	if links && cfg.Bool("bb_http") {
		s = reURL.ReplaceAllString(s, `$1<a href="$2" target="_blank" rel="noopener nofollow ugc">$2</a>`)
	}
	if smiles {
		s = t.applySmiles(s)
	}
	if bb {
		s = reBR.ReplaceAllString(s, "<br>")
		// несколько проходов для вложенных тегов
		for i := 0; i < 3; i++ {
			s = reTag.ReplaceAllStringFunc(s, func(m string) string {
				sm := reTag.FindStringSubmatch(m)
				tag := strings.ToLower(sm[1])
				if !cfg.Bool("bb_" + tag) {
					return m
				}
				return strings.Replace(tagHTML[tag], "%s", sm[2], 1)
			})
		}
		if cfg.Bool("bb_size") {
			s = reSize.ReplaceAllString(s, `<span style="font-size:$1px">$2</span>`)
		}
		s = reQuote.ReplaceAllStringFunc(s, func(m string) string {
			sm := reQuote.FindStringSubmatch(m)
			who := ""
			if sm[1] != "" {
				who = `<cite>` + sm[1] + `</cite>`
			}
			return `<blockquote class="bb-quote">` + who + sm[2] + `</blockquote>`
		})
	}
	s = strings.ReplaceAll(s, "\n", "<br>")
	for i, c := range codes {
		s = strings.Replace(s, "\x00C"+string(rune('A'+i))+"\x00", c, 1)
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
