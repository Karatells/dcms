package shorts

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"dcms/internal/core"
)

const workerCount = 2

type videoJob struct {
	app      *core.App
	id       int64
	uid      int64
	raw      string
	duration float64
}

var (
	jobOnce sync.Once
	jobCh   = make(chan videoJob, 256)

	procMu     sync.RWMutex
	queueOrder []int64       // IDs ожидающих в очереди (FIFO)
	activeProg = map[int64]int{} // ID -> 0..99% прямо сейчас обрабатывается
)

func startWorkers() {
	jobOnce.Do(func() {
		for w := 0; w < workerCount; w++ {
			go func() {
				for job := range jobCh {
					runVideoJob(job)
				}
			}()
		}
	})
}

func enqueueShort(a *core.App, id, uid int64, raw string, dur float64) (queuePos, queueTotal int) {
	startWorkers()
	procMu.Lock()
	queueOrder = append(queueOrder, id)
	queuePos = len(queueOrder)
	queueTotal = len(queueOrder) + len(activeProg)
	procMu.Unlock()

	a.Hub.ToUser(uid, "shorts_progress", map[string]any{
		"id":          id,
		"state":       "queued",
		"progress":    0,
		"queue_pos":   queuePos,
		"queue_total": queueTotal,
	})
	jobCh <- videoJob{app: a, id: id, uid: uid, raw: raw, duration: dur}
	return queuePos, queueTotal
}

// shortProcInfo возвращает текущее состояние обработки клипа (queued|processing, %, позиция в очереди, всего в очереди).
func shortProcInfo(id int64) (state string, progress, queuePos, queueTotal int) {
	procMu.RLock()
	defer procMu.RUnlock()
	queueTotal = len(queueOrder) + len(activeProg)
	if pct, ok := activeProg[id]; ok {
		return "processing", pct, 0, queueTotal
	}
	for idx, qid := range queueOrder {
		if qid == id {
			return "queued", 0, idx + 1, queueTotal
		}
	}
	return "processing", 0, 0, queueTotal
}

func removeFromQueue(id int64) {
	for idx, qid := range queueOrder {
		if qid == id {
			queueOrder = append(queueOrder[:idx], queueOrder[idx+1:]...)
			break
		}
	}
}

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

func runVideoJob(job videoJob) {
	a, id, uid, raw := job.app, job.id, job.uid, job.raw

	procMu.Lock()
	removeFromQueue(id)
	activeProg[id] = 1
	qTotal := len(queueOrder) + len(activeProg)
	procMu.Unlock()

	emitProgress := func(pct int) {
		procMu.Lock()
		activeProg[id] = pct
		qt := len(queueOrder) + len(activeProg)
		procMu.Unlock()
		a.Hub.ToUser(uid, "shorts_progress", map[string]any{
			"id":          id,
			"state":       "processing",
			"progress":    pct,
			"queue_pos":   0,
			"queue_total": qt,
		})
	}

	defer func() {
		procMu.Lock()
		delete(activeProg, id)
		removeFromQueue(id)
		procMu.Unlock()
		_ = os.Remove(raw)
	}()

	a.Hub.ToUser(uid, "shorts_progress", map[string]any{
		"id":          id,
		"state":       "processing",
		"progress":    1,
		"queue_pos":   0,
		"queue_total": qTotal,
	})

	fail := func(msg string) {
		shortsT.Update(id, func(s *Short) {
			s.Status = "failed"
			s.ErrMsg = msg
		})
		a.Hub.ToUser(uid, "shorts_progress", map[string]any{
			"id":     id,
			"state":  "failed",
			"err":    msg,
			"status": "failed",
		})
	}

	if _, err := exec.LookPath("ffmpeg"); err != nil {
		log.Printf("shorts: ffmpeg не найден: %v", err)
		fail("На сервере не установлен обработчик видео")
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

	targetDur := job.duration
	if targetDur <= 0 {
		if info, err := probeVideo(raw); err == nil && info.Duration > 0 {
			targetDur = info.Duration
		}
	}
	if targetDur > float64(maxSec) {
		targetDur = float64(maxSec)
	}

	outMP4 := videoPath(a, id)
	tmpMP4 := outMP4 + ".tmp.mp4"
	outJPG := posterPath(a, id)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Сжимаем до макс. 720x1280 (сохраняя пропорции и чётные стороны), H.264 + AAC, faststart + вывод прогресса.
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
		"-progress", "pipe:1",
		"-nostats",
		"-loglevel", "error",
		tmpMP4,
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fail("Не удалось запустить обработку видео")
		return
	}
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		log.Printf("shorts #%d start error: %v", id, err)
		fail("Не удалось запустить обработку видео")
		return
	}

	lastPct := 1
	lastEmit := time.Now()
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "out_time_us=") || strings.HasPrefix(line, "out_time_ms=") {
			rawVal := strings.SplitN(line, "=", 2)[1]
			if us, err := strconv.ParseInt(rawVal, 10, 64); err == nil && us > 0 && targetDur > 0 {
				pct := int(float64(us) / (targetDur * 1_000_000.0) * 94.0)
				if pct < 1 {
					pct = 1
				}
				if pct > 94 {
					pct = 94
				}
				if pct > lastPct && (pct-lastPct >= 2 || time.Since(lastEmit) >= 400*time.Millisecond) {
					lastPct = pct
					lastEmit = time.Now()
					emitProgress(pct)
				}
			}
		}
	}

	if err := cmd.Wait(); err != nil {
		_ = os.Remove(tmpMP4)
		log.Printf("shorts #%d ffmpeg error: %v\n%s", id, err, stderrBuf.String())
		fail("Не удалось обработать видео (проверьте формат файла)")
		return
	}

	emitProgress(96)
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
	emitProgress(99)

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
