package account

import (
	"bytes"
	"encoding/base64"
	"image/png"
	"strings"
	"testing"
	"time"
)

func decodeDataURI(t *testing.T, uri string) (int, int) {
	t.Helper()
	const pfx = "data:image/png;base64,"
	if !strings.HasPrefix(uri, pfx) {
		t.Fatalf("expected png data uri, got %q", uri[:min(len(uri), 32)])
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(uri, pfx))
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("png decode: %v", err)
	}
	return cfg.Width, cfg.Height
}

func TestInteractiveCaptchaFlow(t *testing.T) {
	// 1. Slider
	sl := newInteractiveChallenge("slider")
	if sl["type"] != "slider" {
		t.Fatalf("expected slider, got %v", sl["type"])
	}
	w, h := decodeDataURI(t, sl["bg"].(string))
	if w != 280 || h != 136 {
		t.Fatalf("unexpected slider bg size: %dx%d", w, h)
	}
	pw, ph := decodeDataURI(t, sl["piece"].(string))
	if pw != 62 || ph != 62 {
		t.Fatalf("unexpected slider piece size: %dx%d", pw, ph)
	}
	slID := sl["id"].(string)
	capMu.Lock()
	capChallenges[slID].created = time.Now().Add(-2 * time.Second)
	targetX := capChallenges[slID].targetNum
	capMu.Unlock()

	// Мгновенный бот отклоняется
	if tok, _, msg := verifyInteractiveChallenge(slID, targetX, "", 50, 5); tok != "" || msg == "" {
		t.Fatalf("expected bot rejection, got tok=%q msg=%q", tok, msg)
	}
	// Человечное попадание в допуске ±9px
	tok, exp, msg := verifyInteractiveChallenge(slID, targetX+4, "", 900, 12)
	if exp || msg != "" || !strings.HasPrefix(tok, "cp_") {
		t.Fatalf("expected slider pass, got tok=%q exp=%v msg=%q", tok, exp, msg)
	}
	if !hasValidCaptchaPass("", tok) {
		t.Fatal("expected hasValidCaptchaPass=true")
	}
	if !checkCaptcha("", tok) {
		t.Fatal("expected checkCaptcha=true on first use")
	}
	if checkCaptcha("", tok) {
		t.Fatal("expected checkCaptcha=false on second use (one-time token)")
	}

	// 2. Rotate
	rot := newInteractiveChallenge("rotate")
	if rot["type"] != "rotate" {
		t.Fatalf("expected rotate, got %v", rot["type"])
	}
	rw, rh := decodeDataURI(t, rot["img"].(string))
	if rw != 140 || rh != 140 {
		t.Fatalf("unexpected rotate img size: %dx%d", rw, rh)
	}
	rotID := rot["id"].(string)
	capMu.Lock()
	capChallenges[rotID].created = time.Now().Add(-2 * time.Second)
	initAng := capChallenges[rotID].targetNum
	capMu.Unlock()
	tok, exp, msg = verifyInteractiveChallenge(rotID, 360-initAng, "", 1200, 5)
	if exp || msg != "" || tok == "" {
		t.Fatalf("expected rotate pass, got tok=%q exp=%v msg=%q", tok, exp, msg)
	}

	// 3. Click
	clk := newInteractiveChallenge("click")
	if clk["type"] != "click" {
		t.Fatalf("expected click, got %v", clk["type"])
	}
	items := clk["items"].([]map[string]any)
	if len(items) != 6 {
		t.Fatalf("expected 6 click items, got %d", len(items))
	}
	tw, th := decodeDataURI(t, items[0]["img"].(string))
	if tw != 76 || th != 76 {
		t.Fatalf("unexpected click tile size: %dx%d", tw, th)
	}
	clkID := clk["id"].(string)
	capMu.Lock()
	capChallenges[clkID].created = time.Now().Add(-2 * time.Second)
	targetKey := capChallenges[clkID].targetKey
	capMu.Unlock()
	tok, exp, msg = verifyInteractiveChallenge(clkID, 0, targetKey, 800, 1)
	if exp || msg != "" || tok == "" {
		t.Fatalf("expected click pass, got tok=%q exp=%v msg=%q", tok, exp, msg)
	}
}
