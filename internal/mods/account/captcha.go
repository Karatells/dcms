package account

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math/rand"
	"os"
	"sync"
	"time"
)

// 5x7 растровый шрифт цифр
var digits = [10][7]string{
	{"01110", "10001", "10011", "10101", "11001", "10001", "01110"},
	{"00100", "01100", "00100", "00100", "00100", "00100", "01110"},
	{"01110", "10001", "00001", "00010", "00100", "01000", "11111"},
	{"11110", "00001", "00001", "01110", "00001", "00001", "11110"},
	{"00010", "00110", "01010", "10010", "11111", "00010", "00010"},
	{"11111", "10000", "11110", "00001", "00001", "10001", "01110"},
	{"00110", "01000", "10000", "11110", "10001", "10001", "01110"},
	{"11111", "00001", "00010", "00100", "01000", "01000", "01000"},
	{"01110", "10001", "10001", "01110", "10001", "10001", "01110"},
	{"01110", "10001", "10001", "01111", "00001", "00010", "01100"},
}

type capEntry struct {
	code string
	exp  time.Time
}

var (
	capMu   sync.Mutex
	capData = map[string]capEntry{}
)

func newCaptcha() (token, code string) {
	b := make([]byte, 5)
	for i := range b {
		b[i] = byte('0' + rand.Intn(10))
	}
	code = string(b)
	token = randTok()
	capMu.Lock()
	defer capMu.Unlock()
	for k, v := range capData {
		if time.Now().After(v.exp) {
			delete(capData, k)
		}
	}
	capData[token] = capEntry{code, time.Now().Add(15 * time.Minute)}
	return
}

// checkCaptcha проверяет код (одноразово).
func checkCaptcha(token, code string) bool {
	if os.Getenv("DCMS_DEV") == "1" && code == "00000" {
		return true
	}
	capMu.Lock()
	defer capMu.Unlock()
	e, ok := capData[token]
	delete(capData, token)
	return ok && time.Now().Before(e.exp) && e.code == code && code != ""
}

func renderCaptcha(code string) []byte {
	const sc = 4
	w, h := len(code)*(6*sc)+20, 7*sc+20
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{240, 242, 250, 255}}, image.Point{}, draw.Src)
	// шум
	for i := 0; i < 260; i++ {
		img.Set(rand.Intn(w), rand.Intn(h), color.RGBA{uint8(150 + rand.Intn(100)), uint8(150 + rand.Intn(100)), uint8(150 + rand.Intn(100)), 255})
	}
	for i := 0; i < 5; i++ { // линии
		x0, y0, x1, y1 := 0, rand.Intn(h), w, rand.Intn(h)
		for t := 0; t <= 100; t++ {
			x, y := x0+(x1-x0)*t/100, y0+(y1-y0)*t/100
			img.Set(x, y, color.RGBA{120, 130, 180, 255})
		}
	}
	for i, ch := range code {
		d := digits[ch-'0']
		ox, oy := 10+i*6*sc+rand.Intn(5)-2, 10+rand.Intn(7)-3
		col := color.RGBA{uint8(20 + rand.Intn(80)), uint8(20 + rand.Intn(80)), uint8(90 + rand.Intn(100)), 255}
		for y, row := range d {
			for x, c := range row {
				if c == '1' {
					for dy := 0; dy < sc; dy++ {
						for dx := 0; dx < sc; dx++ {
							img.Set(ox+x*sc+dx, oy+y*sc+dy, col)
						}
					}
				}
			}
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}
