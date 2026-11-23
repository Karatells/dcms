package shorts

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"dcms/internal/core"
)

// sem ограничивает число одновременных задач ffmpeg (чтобы не перегружать CPU).
var sem = make(chan struct{}, 2)

type probeInfo struct {
	Duration float64
	Width    int64
	Height   int64
}

func probeVideo(path string) (probeInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height:format=duration",
		"-of", "json",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		return probeInfo{}, err
	}
	var parsed struct {
		Streams []struct {
			Width  int64 `json:"width"`
			Height int64 `json:"height"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return probeInfo{}, err
	}
	var info probeInfo
	if d, err := strconv.ParseFloat(strings.TrimSpace(parsed.Format.Duration), 64); err == nil {
		info.Duration = d
	}
	if len(parsed.Streams) > 0 {
		info.Width = parsed.Streams[0].Width
		info.Height = parsed.Streams[0].Height
	}
	return info, nil
}

func videoPath(a *core.App, id int64) string {
	return filepath.Join(a.UploadDir("shorts"), fmt.Sprintf("%d.mp4", id))
}

func posterPath(a *core.App, id int64) string {
	return filepath.Join(a.UploadDir("shorts"), fmt.Sprintf("%d.jpg", id))
}

func rawPath(a *core.App, id int64, ext string) string {
	return filepath.Join(a.UploadDir("shorts"), fmt.Sprintf("raw_%d.%s", id, ext))
}

func processShort(a *core.App, id int64, raw string) {
	sem <- struct{}{}
	defer func() {
		<-sem
		_ = os.Remove(raw)
	}()

	fail := func(msg string) {
		shortsT.Update(id, func(s *Short) {
			s.Status = "failed"
			s.ErrMsg = msg
		})
	}

	if _, err := exec.LookPath("ffmpeg"); err != nil {
		log.Printf("shorts: ffmpeg не найден: %v", err)
		fail("На сервере не установлен ffmpeg")
		return
	}

	maxSec := a.Cfg.Int("shorts_max_sec")
	if maxSec <= 0 || maxSec > 180 {
		maxSec = 60
	}
	crf := a.Cfg.Int("shorts_crf")
	if crf < 18 || crf > 35 {
		crf = 27
	}

	outMP4 := videoPath(a, id)
	tmpMP4 := outMP4 + ".tmp.mp4"
	outJPG := posterPath(a, id)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Сжимаем до макс. 720x1280 (сохраняя пропорции и чётные стороны), H.264 + AAC, faststart для мгновенного стриминга.
	vf := "scale='min(720,iw)':'min(1280,ih)':force_original_aspect_ratio=decrease,scale=trunc(iw/2)*2:trunc(ih/2)*2"
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-y",
		"-i", raw,
		"-t", strconv.Itoa(maxSec),
		"-vf", vf,
		"-c:v", "libx264",
		"-preset", "fast",
		"-crf", strconv.Itoa(crf),
		"-pix_fmt", "yuv420p",
		"-maxrate", "1500k",
		"-bufsize", "3000k",
		"-c:a", "aac",
		"-b:a", "96k",
		"-ac", "2",
		"-movflags", "+faststart",
		tmpMP4,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.Remove(tmpMP4)
		log.Printf("shorts #%d ffmpeg error: %v\n%s", id, err, string(out))
		fail("Не удалось обработать видео (проверьте формат файла)")
		return
	}
	if err := os.Rename(tmpMP4, outMP4); err != nil {
		_ = os.Remove(tmpMP4)
		fail("Ошибка сохранения видео")
		return
	}

	// Извлекаем обложку (кадр на 0.5 сек, при неудаче — первый кадр).
	pCtx, pCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer pCancel()
	if err := exec.CommandContext(pCtx, "ffmpeg", "-y", "-ss", "0.5", "-i", outMP4, "-vframes", "1", "-q:v", "4", outJPG).Run(); err != nil {
		_ = exec.CommandContext(pCtx, "ffmpeg", "-y", "-i", outMP4, "-vframes", "1", "-q:v", "4", outJPG).Run()
	}

	st, _ := os.Stat(outMP4)
	var size int64
	if st != nil {
		size = st.Size()
	}
	info, _ := probeVideo(outMP4)
	dur := int64(info.Duration + 0.5)
	if dur < 1 {
		dur = 1
	}

	sh, ok := shortsT.Update(id, func(s *Short) {
		s.Status = "ready"
		s.ErrMsg = ""
		s.Size = size
		s.Duration = dur
		s.Width = info.Width
		s.Height = info.Height
	})
	if !ok {
		_ = os.Remove(outMP4)
		_ = os.Remove(outJPG)
		return
	}

	a.AddBalls(sh.IDUser, 5, 5)
	a.FanTape(sh.IDUser, core.KObmen, "shorts", sh.ID, core.TapeOpts{})
	a.Hub.ToUser(sh.IDUser, "shorts_ready", map[string]any{"id": sh.ID, "title": sh.Title})
}
