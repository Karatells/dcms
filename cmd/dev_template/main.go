// Шаблон dev-сервера для одного модуля. Скопируйте в cmd/dev_<модуль>/main.go и замените импорт/Register.
// Запуск: DCMS_DEV=1 go run ./cmd/dev_<модуль> -addr :18101 -data /tmp/dcms-<модуль>
package main

import (
	"flag"
	"log"
	"os"

	"dcms/internal/core"
	"dcms/internal/mods/account" // замените на свой модуль
)

func main() {
	addr := flag.String("addr", ":18100", "")
	data := flag.String("data", "/tmp/dcms-dev", "")
	dsn := flag.String("dsn", os.Getenv("DCMS_DSN"), "MySQL/MariaDB DSN")
	flag.Parse()
	if *dsn == "" {
		*dsn = "dcms:dcms@tcp(127.0.0.1:3306)/dcms?charset=utf8mb4"
	}
	a, err := core.New(*data, *dsn, os.DirFS("web"))
	if err != nil {
		log.Fatal(err)
	}
	account.Register(a) // замените на свой модуль (и при необходимости зарегистрируйте соседние, от которых зависите)
	if err := a.Start(); err != nil {
		log.Fatal(err)
	}
	log.Fatal(a.Run(*addr))
}
