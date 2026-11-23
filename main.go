// DCMS Social на Go — социальная сеть: один бинарник, встроенное хранилище, REST API + SPA.
package main

import (
	"context"
	"embed"
	"flag"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"syscall"

	"dcms/internal/core"
	"dcms/internal/mods/account"
	"dcms/internal/mods/admin"
	"dcms/internal/mods/chatnews"
	"dcms/internal/mods/content"
	"dcms/internal/mods/extras"
	"dcms/internal/mods/feeds"
	"dcms/internal/mods/files"
	"dcms/internal/mods/forum"
	"dcms/internal/mods/foto"
	"dcms/internal/mods/profile"
)

//go:embed all:web
var webFS embed.FS

func main() {
	addr := flag.String("addr", envOr("DCMS_ADDR", ":8080"), "адрес HTTP")
	data := flag.String("data", envOr("DCMS_DATA", "data"), "каталог данных")
	webDir := flag.String("web", os.Getenv("DCMS_WEB"), "каталог фронтенда (для разработки; по умолчанию вшитый)")
	flag.Parse()

	if os.Getenv("DCMS_TRUST_PROXY") == "1" {
		core.Defaults["trust_proxy"] = "1" // за nginx: доверять X-Forwarded-For
	}
	var web fs.FS
	if *webDir != "" {
		web = os.DirFS(*webDir)
	} else {
		sub, err := fs.Sub(webFS, "web")
		if err != nil {
			log.Fatal(err)
		}
		web = sub
	}
	a, err := core.New(*data, web)
	if err != nil {
		log.Fatal(err)
	}
	// порядок важен только для зависимостей между таблицами; все модули независимы по маршрутам
	for _, reg := range []func(*core.App){
		account.Register, profile.Register, feeds.Register, extras.Register, forum.Register,
		chatnews.Register, foto.Register, files.Register, content.Register, admin.Register,
	} {
		reg(a)
	}
	if err := a.Start(); err != nil {
		log.Fatal(err)
	}
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		<-ch
		log.Println("останавливаюсь…")
		a.Shutdown(context.Background())
		os.Exit(0)
	}()
	log.Fatal(a.Run(*addr))
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
