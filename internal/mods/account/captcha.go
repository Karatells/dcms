package account

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"math/rand"
	"os"
	"sync"
	"time"
)

// 5x7 растровый шрифт цифр (оставлен для обратной совместимости с GET /api/captcha)
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

type capChallenge struct {
	kind      string // slider | rotate | click
	targetNum float64
	targetKey string
	created   time.Time
	exp       time.Time
	tries     int
}

type capPass struct {
	exp time.Time
}

var (
	capMu         sync.Mutex
	capData       = map[string]capEntry{}
	capChallenges = map[string]*capChallenge{}
	capPasses     = map[string]capPass{}
)

func cleanupCaptchasLocked(now time.Time) {
	for k, v := range capData {
		if now.After(v.exp) {
			delete(capData, k)
		}
	}
	for k, v := range capChallenges {
		if now.After(v.exp) {
			delete(capChallenges, k)
		}
	}
	for k, v := range capPasses {
		if now.After(v.exp) {
			delete(capPasses, k)
		}
	}
}

func newCaptcha() (token, code string) {
	b := make([]byte, 5)
	for i := range b {
		b[i] = byte('0' + rand.Intn(10))
	}
	code = string(b)
	token = randTok()
	now := time.Now()
	capMu.Lock()
	defer capMu.Unlock()
	cleanupCaptchasLocked(now)
	capData[token] = capEntry{code, now.Add(15 * time.Minute)}
	return
}

// checkCaptcha проверяет либо новый одноразовый pass-токен интерактивной капчи,
// либо (для обратной совместимости) пару cookie-токен + цифровой код.
func checkCaptcha(cookieTok, code string) bool {
	if os.Getenv("DCMS_DEV") == "1" && code == "00000" {
		return true
	}
	if code == "" {
		return false
	}
	now := time.Now()
	capMu.Lock()
	defer capMu.Unlock()
	if p, ok := capPasses[code]; ok {
		delete(capPasses, code)
		return now.Before(p.exp)
	}
	if cookieTok != "" {
		e, ok := capData[cookieTok]
		delete(capData, cookieTok)
		return ok && now.Before(e.exp) && e.code == code
	}
	return false
}

// hasValidCaptchaPass проверяет наличие действительного pass-токена без его списания.
func hasValidCaptchaPass(cookieTok, code string) bool {
	if os.Getenv("DCMS_DEV") == "1" && code == "00000" {
		return true
	}
	if code == "" {
		return false
	}
	now := time.Now()
	capMu.Lock()
	defer capMu.Unlock()
	if p, ok := capPasses[code]; ok && now.Before(p.exp) {
		return true
	}
	if cookieTok != "" {
		if e, ok := capData[cookieTok]; ok && now.Before(e.exp) && e.code == code {
			return true
		}
	}
	return false
}

// newInteractiveChallenge создаёт интерактивную капчу одного из 3 типов: slider, rotate, click.
func newInteractiveChallenge(wantType string) map[string]any {
	kinds := []string{"slider", "rotate", "click"}
	kind := wantType
	valid := false
	for _, k := range kinds {
		if k == kind {
			valid = true
			break
		}
	}
	if !valid {
		kind = kinds[rand.Intn(len(kinds))]
	}
	id := randTok()
	now := time.Now()

	var ch *capChallenge
	var resp map[string]any
	switch kind {
	case "slider":
		ch, resp = buildSliderChallenge(id, now)
	case "rotate":
		ch, resp = buildRotateChallenge(id, now)
	default:
		ch, resp = buildClickChallenge(id, now)
	}

	capMu.Lock()
	cleanupCaptchasLocked(now)
	capChallenges[id] = ch
	capMu.Unlock()
	return resp
}

// verifyInteractiveChallenge проверяет ответ пользователя и при успехе выдаёт одноразовый pass-токен.
func verifyInteractiveChallenge(id string, numAns float64, strAns string, durationMs int64, moves int) (passTok string, expired bool, errMsg string) {
	now := time.Now()
	capMu.Lock()
	defer capMu.Unlock()
	ch, ok := capChallenges[id]
	if !ok || now.After(ch.exp) {
		delete(capChallenges, id)
		return "", true, "Время задания истекло, попробуйте новое"
	}
	// Серверная проверка минимального времени прохождения (защита от мгновенных ботов)
	elapsed := now.Sub(ch.created).Milliseconds()
	if elapsed < 180 || durationMs < 180 {
		ch.tries++
		if ch.tries >= 3 {
			delete(capChallenges, id)
			return "", true, "Слишком быстро. Попробуйте ещё раз"
		}
		return "", false, "Слишком быстро. Попробуйте чуть плавнее"
	}

	passed := false
	switch ch.kind {
	case "slider":
		if moves >= 2 && math.Abs(numAns-ch.targetNum) <= 9 {
			passed = true
		}
	case "rotate":
		total := math.Mod(ch.targetNum+numAns, 360)
		if total < 0 {
			total += 360
		}
		diff := math.Min(total, 360-total)
		if diff <= 20 {
			passed = true
		}
	case "click":
		if strAns != "" && strAns == ch.targetKey {
			passed = true
		}
	}

	if !passed {
		ch.tries++
		if ch.kind == "click" || ch.tries >= 3 {
			delete(capChallenges, id)
			return "", true, "Неверно. Попробуйте новое задание"
		}
		return "", false, "Почти! Попробуйте ещё раз"
	}

	delete(capChallenges, id)
	passTok = "cp_" + randTok()
	capPasses[passTok] = capPass{exp: now.Add(10 * time.Minute)}
	return passTok, false, ""
}

// ─────────────────────────────── helpers

func encodePNGDataURI(img image.Image) string {
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

func lerpU8(a, b uint8, t float64) uint8 {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return uint8(float64(a) + (float64(b)-float64(a))*t)
}

func blendOver(dst *image.NRGBA, x, y int, c color.NRGBA) {
	if !image.Pt(x, y).In(dst.Bounds()) || c.A == 0 {
		return
	}
	if c.A == 255 {
		dst.SetNRGBA(x, y, c)
		return
	}
	bg := dst.NRGBAAt(x, y)
	sa := float64(c.A) / 255.0
	da := float64(bg.A) / 255.0
	outA := sa + da*(1-sa)
	if outA <= 0 {
		return
	}
	r := (float64(c.R)*sa + float64(bg.R)*da*(1-sa)) / outA
	g := (float64(c.G)*sa + float64(bg.G)*da*(1-sa)) / outA
	b := (float64(c.B)*sa + float64(bg.B)*da*(1-sa)) / outA
	dst.SetNRGBA(x, y, color.NRGBA{uint8(r), uint8(g), uint8(b), uint8(outA * 255)})
}

func drawAACircle(img *image.NRGBA, cx, cy, r float64, c color.NRGBA) {
	minX := int(math.Floor(cx - r - 1))
	maxX := int(math.Ceil(cx + r + 1))
	minY := int(math.Floor(cy - r - 1))
	maxY := int(math.Ceil(cy + r + 1))
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy) - r
			if d < 0.75 {
				cov := clamp01(0.75 - d)
				col := c
				col.A = uint8(float64(c.A) * cov)
				blendOver(img, x, y, col)
			}
		}
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// ─────────────────────────────── 1. SLIDER PUZZLE

func buildSliderChallenge(id string, now time.Time) (*capChallenge, map[string]any) {
	const (
		W   = 280
		H   = 136
		PS  = 42 // основной квадрат пазла
		PAD = 10 // отступ для ушка и тени
		PW  = PS + PAD*2
	)

	// 1. Рисуем приятный процедурный пейзаж
	bg := image.NewNRGBA(image.Rect(0, 0, W, H))
	palettes := [][4]color.NRGBA{
		{{58, 123, 213, 255}, {142, 197, 252, 255}, {44, 92, 150, 255}, {255, 224, 130, 255}},  // небо
		{{255, 117, 140, 255}, {255, 184, 140, 255}, {114, 59, 108, 255}, {255, 241, 184, 255}}, // закат
		{{28, 167, 138, 255}, {140, 226, 184, 255}, {21, 99, 86, 255}, {255, 244, 163, 255}},   // мята
		{{106, 90, 205, 255}, {182, 164, 245, 255}, {58, 46, 128, 255}, {255, 232, 165, 255}},  // лаванда
		{{32, 58, 102, 255}, {76, 118, 178, 255}, {22, 39, 72, 255}, {250, 240, 202, 255}},     // ночь
	}
	pal := palettes[rand.Intn(len(palettes))]
	for y := 0; y < H; y++ {
		t := float64(y) / float64(H-1)
		c := color.NRGBA{lerpU8(pal[0].R, pal[1].R, t), lerpU8(pal[0].G, pal[1].G, t), lerpU8(pal[0].B, pal[1].B, t), 255}
		for x := 0; x < W; x++ {
			bg.SetNRGBA(x, y, c)
		}
	}

	// Солнце/луна
	sunX := float64(36 + rand.Intn(W-72))
	sunY := float64(24 + rand.Intn(36))
	drawAACircle(bg, sunX, sunY, 24, color.NRGBA{pal[3].R, pal[3].G, pal[3].B, 60})
	drawAACircle(bg, sunX, sunY, 14, pal[3])

	// Декоративные облачка / круги
	for i := 0; i < 4; i++ {
		cx := float64(20 + rand.Intn(W-40))
		cy := float64(18 + rand.Intn(50))
		r := float64(10 + rand.Intn(12))
		drawAACircle(bg, cx, cy, r, color.NRGBA{255, 255, 255, 42})
		drawAACircle(bg, cx+r*0.75, cy+2, r*0.75, color.NRGBA{255, 255, 255, 36})
	}

	// Два слоя плавных холмов
	ph1 := rand.Float64() * 6.28
	ph2 := rand.Float64() * 6.28
	h1Col := color.NRGBA{lerpU8(pal[2].R, 255, 0.22), lerpU8(pal[2].G, 255, 0.22), lerpU8(pal[2].B, 255, 0.22), 230}
	for x := 0; x < W; x++ {
		hy1 := 76 + 18*math.Sin(float64(x)*0.025+ph1) + 7*math.Cos(float64(x)*0.055+ph2)
		hy2 := 98 + 14*math.Sin(float64(x)*0.033+ph2+1.2)
		for y := int(hy1); y < H; y++ {
			if y >= 0 {
				blendOver(bg, x, y, h1Col)
			}
		}
		for y := int(hy2); y < H; y++ {
			if y >= 0 {
				bg.SetNRGBA(x, y, pal[2])
			}
		}
	}

	// Несколько аккуратных звёздочек/бликов для текстуры
	for i := 0; i < 12; i++ {
		drawAACircle(bg, float64(10+rand.Intn(W-20)), float64(10+rand.Intn(H-20)), float64(2+rand.Intn(4)), color.NRGBA{255, 255, 255, 55})
	}

	// 2. Позиция пазла (левый верхний угол квадрата внутри bg)
	tx := 105 + rand.Intn(W-105-PS-16)
	ty := 20 + rand.Intn(H-PS-34)
	tabRight := rand.Intn(2) == 0 // ушко справа или слева
	tabTop := rand.Intn(2) == 0   // ушко сверху или снизу

	// SDF для формы пазла относительно левого верхнего угла кусочка (0..PW)
	puzzleSDF := func(lx, ly float64) float64 {
		// Основной скруглённый квадрат [PAD .. PAD+PS]
		cx := float64(PAD) + float64(PS)/2
		cy := float64(PAD) + float64(PS)/2
		half := float64(PS)/2 - 4
		dx := math.Abs(lx-cx) - half
		dy := math.Abs(ly-cy) - half
		boxD := math.Hypot(math.Max(dx, 0), math.Max(dy, 0)) + math.Min(math.Max(dx, dy), 0) - 4

		// Выступающее ушко
		var earX, earY float64
		if tabRight {
			earX = float64(PAD + PS + 3)
			earY = cy
		} else {
			earX = float64(PAD - 3)
			earY = cy
		}
		earD := math.Hypot(lx-earX, ly-earY) - 7.5
		d := math.Min(boxD, earD)

		// Второе ушко сверху или снизу
		if tabTop {
			ear2D := math.Hypot(lx-cx, ly-float64(PAD-3)) - 7.0
			d = math.Min(d, ear2D)
		} else {
			ear2D := math.Hypot(lx-cx, ly-float64(PAD+PS+3)) - 7.0
			d = math.Min(d, ear2D)
		}
		return d
	}

	// 3. Вырезаем кусочек и рисуем выемку на фоне
	piece := image.NewNRGBA(image.Rect(0, 0, PW, PW))
	ox := tx - PAD
	oy := ty - PAD

	for ly := 0; ly < PW; ly++ {
		for lx := 0; lx < PW; lx++ {
			d := puzzleSDF(float64(lx)+0.5, float64(ly)+0.5)
			bx, by := ox+lx, oy+ly
			if d < 1.2 {
				// Копируем пиксель в кусочек
				src := bg.NRGBAAt(bx, by)
				if d < -1.2 {
					piece.SetNRGBA(lx, ly, src)
				} else if d < 0.8 {
					// Светлая аккуратная обводка кусочка
					cov := clamp01((0.8 - d) / 1.0)
					piece.SetNRGBA(lx, ly, color.NRGBA{255, 255, 255, uint8(235 * cov)})
				}

				// Рисуем выемку на основном фоне
				if d < -0.6 {
					blendOver(bg, bx, by, color.NRGBA{15, 20, 35, 130})
				} else if d < 1.0 {
					cov := clamp01(1.0 - math.Abs(d-0.2)/0.9)
					blendOver(bg, bx, by, color.NRGBA{255, 255, 255, uint8(200 * cov)})
				}
			}
		}
	}

	ch := &capChallenge{
		kind:      "slider",
		targetNum: float64(ox),
		created:   now,
		exp:       now.Add(5 * time.Minute),
	}
	return ch, map[string]any{
		"id":     id,
		"type":   "slider",
		"prompt": "Передвиньте пазл в выемку",
		"bg":     encodePNGDataURI(bg),
		"piece":  encodePNGDataURI(piece),
		"y":      oy,
		"w":      W,
		"h":      H,
		"pw":     PW,
	}
}

// ─────────────────────────────── 2. ROTATE IMAGE

func buildRotateChallenge(id string, now time.Time) (*capChallenge, map[string]any) {
	const S = 140
	// Сначала рисуем вертикальную картинку в ровном положении
	src := image.NewNRGBA(image.Rect(0, 0, S, S))
	scene := rand.Intn(4) // 0=домик, 1=ракета, 2=ёлочка, 3=кораблик

	// Мягкий небесный фон
	for y := 0; y < S; y++ {
		t := float64(y) / float64(S-1)
		c := color.NRGBA{lerpU8(215, 240, t), lerpU8(234, 246, t), lerpU8(255, 255, t), 255}
		if scene == 1 { // космос для ракеты
			c = color.NRGBA{lerpU8(26, 56, t), lerpU8(36, 68, t), lerpU8(78, 128, t), 255}
		}
		for x := 0; x < S; x++ {
			src.SetNRGBA(x, y, c)
		}
	}

	fillRect := func(x0, y0, x1, y1 int, c color.NRGBA) {
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				blendOver(src, x, y, c)
			}
		}
	}
	fillTri := func(x1, y1, x2, y2, x3, y3 float64, c color.NRGBA) {
		minX := int(math.Floor(math.Min(x1, math.Min(x2, x3))))
		maxX := int(math.Ceil(math.Max(x1, math.Max(x2, x3))))
		minY := int(math.Floor(math.Min(y1, math.Min(y2, y3))))
		maxY := int(math.Ceil(math.Max(y1, math.Max(y2, y3))))
		sign := func(px, py, ax, ay, bx, by float64) float64 {
			return (px-bx)*(ay-by) - (ax-bx)*(py-by)
		}
		for y := minY; y <= maxY; y++ {
			for x := minX; x <= maxX; x++ {
				px, py := float64(x)+0.5, float64(y)+0.5
				d1 := sign(px, py, x1, y1, x2, y2)
				d2 := sign(px, py, x2, y2, x3, y3)
				d3 := sign(px, py, x3, y3, x1, y1)
				hasNeg := (d1 < 0) || (d2 < 0) || (d3 < 0)
				hasPos := (d1 > 0) || (d2 > 0) || (d3 > 0)
				if !(hasNeg && hasPos) {
					blendOver(src, x, y, c)
				}
			}
		}
	}

	switch scene {
	case 0: // Домик
		fillRect(0, 102, S, S, color.NRGBA{96, 186, 104, 255})                       // травка
		drawAACircle(src, 108, 30, 12, color.NRGBA{255, 206, 68, 255})               // солнышко
		fillRect(88, 36, 98, 56, color.NRGBA{148, 86, 68, 255})                      // труба
		fillRect(42, 66, 98, 106, color.NRGBA{250, 232, 198, 255})                   // стены
		fillTri(32, 68, 70, 32, 108, 68, color.NRGBA{214, 72, 66, 255})              // крыша
		fillRect(56, 76, 74, 92, color.NRGBA{90, 178, 235, 255})                     // окно
		fillRect(78, 78, 92, 106, color.NRGBA{138, 88, 52, 255})                     // дверь
	case 1: // Ракета
		for i := 0; i < 10; i++ {
			drawAACircle(src, float64(18+(i*29)%104), float64(18+(i*37)%104), 2, color.NRGBA{255, 255, 255, 190})
		}
		fillTri(70, 118, 58, 94, 82, 94, color.NRGBA{255, 138, 42, 255})             // пламя
		fillTri(70, 110, 63, 94, 77, 94, color.NRGBA{255, 224, 92, 255})             // центр пламени
		fillTri(54, 74, 38, 98, 54, 94, color.NRGBA{228, 74, 74, 255})               // левое крыло
		fillTri(86, 74, 102, 98, 86, 94, color.NRGBA{228, 74, 74, 255})              // правое крыло
		fillRect(54, 48, 86, 96, color.NRGBA{242, 246, 255, 255})                    // корпус
		fillTri(54, 49, 70, 22, 86, 49, color.NRGBA{228, 74, 74, 255})               // нос
		drawAACircle(src, 70, 66, 10, color.NRGBA{72, 168, 240, 255})                // иллюминатор
		drawAACircle(src, 70, 66, 6, color.NRGBA{166, 224, 255, 255})                // блик
	case 2: // Ёлочка
		fillRect(0, 106, S, S, color.NRGBA{230, 242, 252, 255})                      // снег
		fillRect(62, 94, 78, 114, color.NRGBA{134, 82, 48, 255})                     // ствол
		fillTri(34, 96, 70, 56, 106, 96, color.NRGBA{34, 136, 78, 255})              // нижний ярус
		fillTri(42, 74, 70, 40, 98, 74, color.NRGBA{44, 158, 92, 255})               // средний ярус
		fillTri(50, 54, 70, 26, 90, 54, color.NRGBA{56, 180, 106, 255})              // верхний ярус
		drawAACircle(src, 70, 24, 7, color.NRGBA{255, 206, 56, 255})                 // звезда
	case 3: // Кораблик
		fillRect(0, 96, S, S, color.NRGBA{52, 146, 220, 255})                        // море
		drawAACircle(src, 108, 30, 11, color.NRGBA{255, 208, 66, 255})               // солнце
		fillRect(68, 28, 73, 86, color.NRGBA{124, 82, 52, 255})                      // мачта
		fillTri(73, 30, 106, 76, 73, 76, color.NRGBA{255, 255, 255, 255})            // правый парус
		fillTri(68, 36, 40, 76, 68, 76, color.NRGBA{234, 82, 76, 255})               // левый парус
		fillRect(36, 82, 104, 100, color.NRGBA{146, 84, 50, 255})                    // корпус
		fillTri(28, 82, 36, 82, 36, 100, color.NRGBA{146, 84, 50, 255})
		fillTri(104, 82, 112, 82, 104, 100, color.NRGBA{146, 84, 50, 255})
	}

	// Поворачиваем на случайный угол [45..315] (кратно 15°) и обрезаем в круг
	steps := 3 + rand.Intn(19) // 45..315 градусов
	initAngle := float64(steps * 15)
	rad := -initAngle * math.Pi / 180.0
	cosA, sinA := math.Cos(rad), math.Sin(rad)
	cx, cy := float64(S)/2.0, float64(S)/2.0
	radLimit := float64(S)/2.0 - 2.0

	dst := image.NewNRGBA(image.Rect(0, 0, S, S))
	for y := 0; y < S; y++ {
		for x := 0; x < S; x++ {
			dx := float64(x) + 0.5 - cx
			dy := float64(y) + 0.5 - cy
			dist := math.Hypot(dx, dy)
			if dist > radLimit+1 {
				continue
			}
			sx := cx + dx*cosA - dy*sinA
			sy := cy + dx*sinA + dy*cosA
			ix, iy := int(math.Floor(sx)), int(math.Floor(sy))
			if ix < 0 || iy < 0 || ix >= S-1 || iy >= S-1 {
				continue
			}
			// Билинейная интерполяция для плавных линий
			fx, fy := sx-float64(ix), sy-float64(iy)
			c00 := src.NRGBAAt(ix, iy)
			c10 := src.NRGBAAt(ix+1, iy)
			c01 := src.NRGBAAt(ix, iy+1)
			c11 := src.NRGBAAt(ix+1, iy+1)
			bilerp := func(v00, v10, v01, v11 uint8) uint8 {
				top := float64(v00)*(1-fx) + float64(v10)*fx
				bot := float64(v01)*(1-fx) + float64(v11)*fx
				return uint8(top*(1-fy) + bot*fy)
			}
			alpha := uint8(255)
			if dist > radLimit-1 {
				alpha = uint8(255 * clamp01(radLimit+1-dist) / 2.0)
			}
			dst.SetNRGBA(x, y, color.NRGBA{
				bilerp(c00.R, c10.R, c01.R, c11.R),
				bilerp(c00.G, c10.G, c01.G, c11.G),
				bilerp(c00.B, c10.B, c01.B, c11.B),
				alpha,
			})
		}
	}

	ch := &capChallenge{
		kind:      "rotate",
		targetNum: initAngle,
		created:   now,
		exp:       now.Add(5 * time.Minute),
	}
	return ch, map[string]any{
		"id":     id,
		"type":   "rotate",
		"prompt": "Поверните картинку так, чтобы она стояла ровно",
		"img":    encodePNGDataURI(dst),
	}
}

// ─────────────────────────────── 3. CLICK SHAPE

type shapeSpec struct {
	id     string
	acc    string // винительный падеж: "звезду", "сердце", "треугольник", "круг", "ромб"
	gender string // f, n, m
}

type colorSpec struct {
	id   string
	rgba color.NRGBA
	accF string
	accN string
	accM string
}

var capShapes = []shapeSpec{
	{"star", "звезду", "f"},
	{"heart", "сердце", "n"},
	{"triangle", "треугольник", "m"},
	{"circle", "круг", "m"},
	{"diamond", "ромб", "m"},
}

var capColors = []colorSpec{
	{"red", color.NRGBA{232, 68, 76, 255}, "красную", "красное", "красный"},
	{"blue", color.NRGBA{52, 132, 238, 255}, "синюю", "синее", "синий"},
	{"green", color.NRGBA{46, 178, 106, 255}, "зелёную", "зелёное", "зелёный"},
	{"amber", color.NRGBA{242, 174, 36, 255}, "жёлтую", "жёлтое", "жёлтый"},
}

func buildClickChallenge(id string, now time.Time) (*capChallenge, map[string]any) {
	type pair struct {
		si, ci int
	}
	all := make([]pair, 0, len(capShapes)*len(capColors))
	for si := range capShapes {
		for ci := range capColors {
			all = append(all, pair{si, ci})
		}
	}
	rand.Shuffle(len(all), func(i, j int) { all[i], all[j] = all[j], all[i] })
	picked := all[:6]
	targetIdx := rand.Intn(6)
	targetPair := picked[targetIdx]

	sh := capShapes[targetPair.si]
	co := capColors[targetPair.ci]
	colWord := co.accM
	if sh.gender == "f" {
		colWord = co.accF
	} else if sh.gender == "n" {
		colWord = co.accN
	}
	prompt := fmt.Sprintf("Нажмите на %s %s", colWord, sh.acc)

	items := make([]map[string]any, 6)
	targetKey := ""
	for i, p := range picked {
		k := randTok()[:10]
		if i == targetIdx {
			targetKey = k
		}
		img := renderShapeTile(capShapes[p.si].id, capColors[p.ci].rgba)
		items[i] = map[string]any{
			"key": k,
			"img": encodePNGDataURI(img),
		}
	}

	ch := &capChallenge{
		kind:      "click",
		targetKey: targetKey,
		created:   now,
		exp:       now.Add(5 * time.Minute),
	}
	return ch, map[string]any{
		"id":     id,
		"type":   "click",
		"prompt": prompt,
		"items":  items,
	}
}

func renderShapeTile(shape string, col color.NRGBA) *image.NRGBA {
	const S = 76
	img := image.NewNRGBA(image.Rect(0, 0, S, S))
	// Мягкий светлый фон с лёгким шумом
	bgR := uint8(242 + rand.Intn(10))
	bgG := uint8(244 + rand.Intn(10))
	bgB := uint8(248 + rand.Intn(7))
	for y := 0; y < S; y++ {
		for x := 0; x < S; x++ {
			img.SetNRGBA(x, y, color.NRGBA{bgR, bgG, bgB, 255})
		}
	}
	// Небольшие декоративные точки на фоне
	for i := 0; i < 6; i++ {
		drawAACircle(img, float64(6+rand.Intn(S-12)), float64(6+rand.Intn(S-12)), float64(2+rand.Intn(4)), color.NRGBA{col.R, col.G, col.B, 24})
	}

	// Небольшой случайный поворот (-20..+20 град)
	ang := (rand.Float64()*40 - 20) * math.Pi / 180.0
	if shape == "heart" {
		ang *= 0.45 // сердце держим почти ровно
	}
	cosA, sinA := math.Cos(ang), math.Sin(ang)
	cx, cy := float64(S)/2.0, float64(S)/2.0

	inShape := func(px, py float64) bool {
		// Нормализованные координаты [-1..1]
		nx := (px*cosA - py*sinA) / 24.0
		ny := (px*sinA + py*cosA) / 24.0
		switch shape {
		case "circle":
			return nx*nx+ny*ny <= 0.86
		case "diamond":
			return math.Abs(nx)+math.Abs(ny) <= 1.02
		case "triangle":
			// Равносторонний треугольник остриём вверх
			if ny > 0.78 || ny < -0.92 {
				return false
			}
			halfW := (ny + 0.92) / 1.70 * 1.05
			return math.Abs(nx) <= halfW
		case "heart":
			// Классическое сердце: (x^2 + y^2 - 1)^3 - x^2*y^3 <= 0 (ось Y вверх -> инвертируем ny)
			x := nx * 1.15
			y := -ny*1.15 + 0.12
			a := x*x + y*y - 0.85
			return a*a*a-x*x*y*y*y <= 0
		case "star":
			// 5-конечная звезда в полярных координатах
			r := math.Hypot(nx, ny)
			if r > 1.08 {
				return false
			}
			a := math.Atan2(ny, nx) + math.Pi/2
			sec := math.Mod(a+2*math.Pi, 2*math.Pi/5)
			if sec > math.Pi/5 {
				sec = 2*math.Pi/5 - sec
			}
			// Линейная интерполяция радиуса от внешнего 1.05 к внутреннему 0.44
			rLimit := 1.05 - (1.05-0.44)*(sec/(math.Pi/5))
			return r <= rLimit
		}
		return false
	}

	// 2x2 суперсэмплинг для гладких краёв
	for y := 0; y < S; y++ {
		for x := 0; x < S; x++ {
			hits := 0
			for sy := 0; sy < 2; sy++ {
				for sx := 0; sx < 2; sx++ {
					px := float64(x) + 0.25 + float64(sx)*0.5 - cx
					py := float64(y) + 0.25 + float64(sy)*0.5 - cy
					if inShape(px, py) {
						hits++
					}
				}
			}
			if hits > 0 {
				c := col
				c.A = uint8(float64(col.A) * float64(hits) / 4.0)
				blendOver(img, x, y, c)
			}
		}
	}
	return img
}

// renderCaptcha — старый PNG-рендерер (оставлен как fallback для GET /api/captcha).
func renderCaptcha(code string) []byte {
	const sc = 4
	w, h := len(code)*(6*sc)+20, 7*sc+20
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{240, 242, 250, 255}}, image.Point{}, draw.Src)
	for i := 0; i < 260; i++ {
		img.Set(rand.Intn(w), rand.Intn(h), color.RGBA{uint8(150 + rand.Intn(100)), uint8(150 + rand.Intn(100)), uint8(150 + rand.Intn(100)), 255})
	}
	for i := 0; i < 5; i++ {
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
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
