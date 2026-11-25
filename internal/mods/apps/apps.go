package apps

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"dcms/internal/core"
)

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
