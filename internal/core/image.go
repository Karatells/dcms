package core

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// ImageInfo возвращает размеры и формат ("jpeg","png","gif").
func ImageInfo(data []byte) (w, h int, format string, err error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	return cfg.Width, cfg.Height, format, err
}

// Thumb масштабирует изображение так, чтобы оно вписалось в maxW×maxH (crop=true — заполнить и обрезать по центру).
// Возвращает JPEG (или PNG для прозрачных). Не увеличивает маленькие изображения (кроме crop).
func Thumb(data []byte, maxW, maxH int, crop bool) ([]byte, string, error) {
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", err
	}
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	var dst *image.RGBA
	if crop {
		scale := maxf(float64(maxW)/float64(sw), float64(maxH)/float64(sh))
		cw, ch := int(float64(maxW)/scale), int(float64(maxH)/scale)
		x0, y0 := b.Min.X+(sw-cw)/2, b.Min.Y+(sh-ch)/2
		dst = resample(img, image.Rect(x0, y0, x0+cw, y0+ch), maxW, maxH)
	} else {
		scale := minf(float64(maxW)/float64(sw), float64(maxH)/float64(sh))
		if scale >= 1 {
			if format == "jpeg" {
				return data, "jpg", nil
			}
			scale = 1
		}
		nw, nh := maxi(1, int(float64(sw)*scale)), maxi(1, int(float64(sh)*scale))
		dst = resample(img, b, nw, nh)
	}
	var buf bytes.Buffer
	if format == "png" || format == "gif" {
		if err := png.Encode(&buf, dst); err != nil {
			return nil, "", err
		}
		return buf.Bytes(), "png", nil
	}
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 85}); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), "jpg", nil
}

// resample — билинейная выборка из src.Rect(r) в w×h.
func resample(src image.Image, r image.Rectangle, w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	// фон для прозрачных
	draw.Draw(dst, dst.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)
	sx := float64(r.Dx()) / float64(w)
	sy := float64(r.Dy()) / float64(h)
	for y := 0; y < h; y++ {
		fy := (float64(y)+0.5)*sy - 0.5
		y0 := int(fy)
		if fy < 0 {
			y0, fy = 0, 0
		}
		y1 := mini(y0+1, r.Dy()-1)
		wy := fy - float64(y0)
		for x := 0; x < w; x++ {
			fx := (float64(x)+0.5)*sx - 0.5
			x0 := int(fx)
			if fx < 0 {
				x0, fx = 0, 0
			}
			x1 := mini(x0+1, r.Dx()-1)
			wx := fx - float64(x0)
			var acc [4]float64
			for i, p := range [4]struct {
				x, y int
				w    float64
			}{{x0, y0, (1 - wx) * (1 - wy)}, {x1, y0, wx * (1 - wy)}, {x0, y1, (1 - wx) * wy}, {x1, y1, wx * wy}} {
				_ = i
				cr, cg, cb, ca := src.At(r.Min.X+p.x, r.Min.Y+p.y).RGBA()
				// композитим на белое
				a := float64(ca) / 65535
				acc[0] += p.w * (float64(cr)/257*1 + 255*(1-a)*1*0 + 0)
				acc[1] += p.w * (float64(cg) / 257)
				acc[2] += p.w * (float64(cb) / 257)
				acc[3] += p.w * a
			}
			a := acc[3]
			dst.SetRGBA(x, y, color.RGBA{uint8(clamp(acc[0]/maxf(a, 0.0001)*a + 255*(1-a))), uint8(clamp(acc[1]/maxf(a, 0.0001)*a + 255*(1-a))), uint8(clamp(acc[2]/maxf(a, 0.0001)*a + 255*(1-a))), 255})
		}
	}
	return dst
}

func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}
func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
func maxi(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func mini(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// IsGIFAnimated — true для анимированных GIF.
func IsGIFAnimated(data []byte) bool {
	g, err := gif.DecodeAll(bytes.NewReader(data))
	return err == nil && len(g.Image) > 1
}

// Upload — загруженный файл в памяти.
type Upload struct {
	Name string // исходное имя
	Ext  string // расширение в нижнем регистре
	Size int64
	Data []byte
	MIME string // определено по содержимому
}

// FormFile читает файл из multipart-формы (поле name), ограничивая размер maxBytes. Возвращает (nil, nil), если файла нет.
func (c *Ctx) FormFile(name string, maxBytes int64) (*Upload, error) {
	if c.R.MultipartForm == nil {
		c.R.Body = http.MaxBytesReader(c.W, c.R.Body, maxBytes+(1<<20))
		if err := c.R.ParseMultipartForm(8 << 20); err != nil {
			if strings.Contains(err.Error(), "request body too large") {
				return nil, ErrBad("Файл слишком большой")
			}
			if err == http.ErrNotMultipart {
				return nil, nil
			}
			return nil, ErrBad("Ошибка разбора формы")
		}
	}
	f, h, err := c.R.FormFile(name)
	if err != nil {
		return nil, nil
	}
	defer f.Close()
	return readUpload(f, h, maxBytes)
}

func readUpload(f multipart.File, h *multipart.FileHeader, maxBytes int64) (*Upload, error) {
	if h.Size > maxBytes {
		return nil, ErrBad("Файл слишком большой (максимум " + SizeStr(maxBytes) + ")")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, ErrBad("Не удалось прочитать файл")
	}
	if int64(len(data)) > maxBytes {
		return nil, ErrBad("Файл слишком большой (максимум " + SizeStr(maxBytes) + ")")
	}
	return &Upload{Name: filepath.Base(h.Filename), Ext: Ext(h.Filename), Size: int64(len(data)), Data: data, MIME: http.DetectContentType(data)}, nil
}

// IsImage — по содержимому это jpeg/png/gif.
func (u *Upload) IsImage() bool {
	return u.MIME == "image/jpeg" || u.MIME == "image/png" || u.MIME == "image/gif"
}

// FormValue — значение поля multipart/form-data или urlencoded.
func (c *Ctx) FormValue(name string) string {
	if c.R.MultipartForm == nil && strings.HasPrefix(c.R.Header.Get("Content-Type"), "multipart/") {
		_ = c.R.ParseMultipartForm(8 << 20)
	}
	return c.R.FormValue(name)
}

// WriteFile атомарно записывает файл (создаёт каталоги).
func WriteFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ServeFile отдаёт файл с заголовками кеша; name — имя для скачивания (пусто — inline).
func (c *Ctx) ServeFile(path, name, mime string) error {
	f, err := os.Open(path)
	if err != nil {
		return ErrNotFound()
	}
	defer f.Close()
	st, _ := f.Stat()
	if mime != "" {
		c.W.Header().Set("Content-Type", mime)
	}
	if name != "" {
		c.W.Header().Set("Content-Disposition", `attachment; filename*=UTF-8''`+urlEscape(name))
	}
	c.W.Header().Set("X-Content-Type-Options", "nosniff")
	c.done = true
	http.ServeContent(c.W, c.R, name, st.ModTime(), f)
	return nil
}

func urlEscape(s string) string {
	var sb strings.Builder
	for _, b := range []byte(s) {
		if b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '-' || b == '_' || b == '.' {
			sb.WriteByte(b)
		} else {
			sb.WriteString("%" + strings.ToUpper(string("0123456789abcdef"[b>>4])+string("0123456789abcdef"[b&15])))
		}
	}
	return sb.String()
}

// ServeBytes отдаёт байты как inline-ответ с типом и кешем.
func (c *Ctx) ServeBytes(data []byte, mime string, cache bool) error {
	h := c.W.Header()
	h.Set("Content-Type", mime)
	if cache {
		h.Set("Cache-Control", "public, max-age=86400")
	}
	h.Set("X-Content-Type-Options", "nosniff")
	c.done = true
	c.W.WriteHeader(200)
	_, err := c.W.Write(data)
	return err
}

// FormFiles читает до maxCount файлов из поля name (multipart), каждый не больше maxBytes.
func (c *Ctx) FormFiles(name string, maxBytes int64, maxCount int) ([]*Upload, error) {
	if c.R.MultipartForm == nil {
		c.R.Body = http.MaxBytesReader(c.W, c.R.Body, maxBytes*int64(maxCount)+(1<<20))
		if err := c.R.ParseMultipartForm(8 << 20); err != nil {
			if err == http.ErrNotMultipart {
				return nil, nil
			}
			return nil, ErrBad("Файл слишком большой или форма повреждена")
		}
	}
	var out []*Upload
	for _, h := range c.R.MultipartForm.File[name] {
		if len(out) >= maxCount {
			break
		}
		f, err := h.Open()
		if err != nil {
			continue
		}
		u, err := readUpload(f, h, maxBytes)
		f.Close()
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, nil
}

// RemoveFile удаляет файл, игнорируя ошибки.
func RemoveFile(path string) { _ = os.Remove(path) }
