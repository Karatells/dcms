package apps

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"dcms/internal/core"
)

type manifestItem struct {
	File      string `json:"file"`
	Platform  string `json:"platform"`
	Arch      string `json:"arch"`
	Label     string `json:"label"`
	Note      string `json:"note"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
	UpdatedAt int64  `json:"updated_at"`
}

type appsManifest struct {
	AppName   string         `json:"app_name"`
	ServerURL string         `json:"server_url"`
	Version   string         `json:"version"`
	Rev       string         `json:"rev"`
	BuiltAt   string         `json:"built_at"`
	Items     []manifestItem `json:"items"`
}

// compareSemver возвращает 1 если a > b, -1 если a < b, 0 если равны.
func compareSemver(a, b string) int {
	pa := strings.Split(strings.TrimPrefix(strings.TrimSpace(a), "v"), ".")
	pb := strings.Split(strings.TrimPrefix(strings.TrimSpace(b), "v"), ".")
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		var va, vb int
		if i < len(pa) {
			va, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			vb, _ = strconv.Atoi(pb[i])
		}
		if va > vb {
			return 1
		}
		if va < vb {
			return -1
		}
	}
	return 0
}

func Register(a *core.App) {
	appsDir := func() string {
		return filepath.Join(a.DataDir, "apps")
	}

	a.GET("/api/apps", func(c *core.Ctx) error {
		raw, err := os.ReadFile(filepath.Join(appsDir(), "manifest.json"))
		if err != nil {
			return c.JSON(map[string]any{
				"app_name": a.Cfg.Str("site_name"),
				"items":    []any{},
			})
		}
		var data map[string]any
		if err := json.Unmarshal(raw, &data); err != nil {
			return c.JSON(map[string]any{"items": []any{}})
		}
		return c.JSON(data)
	})

	a.GET("/api/apps/check", func(c *core.Ctx) error {
		plat := strings.ToLower(strings.TrimSpace(c.Q("platform")))
		if plat == "win32" {
			plat = "windows"
		}
		arch := strings.ToLower(strings.TrimSpace(c.Q("arch")))
		cur := strings.TrimSpace(c.Q("current"))

		raw, err := os.ReadFile(filepath.Join(appsDir(), "manifest.json"))
		if err != nil {
			return c.JSON(map[string]any{"update_available": false})
		}
		var m appsManifest
		if err := json.Unmarshal(raw, &m); err != nil || m.Version == "" {
			return c.JSON(map[string]any{"update_available": false})
		}
		var match *manifestItem
		for i := range m.Items {
			it := &m.Items[i]
			if it.Platform != plat {
				continue
			}
			if plat == "android" || arch == "" || it.Arch == arch || it.Arch == "universal" {
				match = it
				break
			}
		}
		if match == nil {
			return c.JSON(map[string]any{"update_available": false, "latest": m.Version})
		}
		avail := cur != "" && compareSemver(m.Version, cur) > 0
		return c.JSON(map[string]any{
			"update_available": avail,
			"current":          cur,
			"latest":           m.Version,
			"rev":              m.Rev,
			"built_at":         m.BuiltAt,
			"file":             match.File,
			"label":            match.Label,
			"size":             match.Size,
			"sha256":           match.SHA256,
			"download_url":     "/api/apps/download/" + match.File,
		})
	})

	a.GET("/api/apps/download/{file}", func(c *core.Ctx) error {
		name := c.Path("file")
		if name == "" || filepath.Base(name) != name || strings.Contains(name, "..") {
			return core.ErrBad("Некорректное имя файла")
		}
		okExt := false
		for _, ext := range []string{".apk", ".zip", ".tar.gz", ".AppImage", ".deb", ".exe"} {
			if strings.HasSuffix(name, ext) {
				okExt = true
				break
			}
		}
		if !okExt {
			return core.ErrNotFound("Файл не найден")
		}
		full := filepath.Join(appsDir(), name)
		st, err := os.Stat(full)
		if err != nil || st.IsDir() {
			return core.ErrNotFound("Сборка не найдена")
		}
		c.W.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
		if strings.HasSuffix(name, ".apk") {
			c.W.Header().Set("Content-Type", "application/vnd.android.package-archive")
		}
		http.ServeFile(c.W, c.R, full)
		return nil
	})
}
