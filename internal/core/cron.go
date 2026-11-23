package core

import (
	"log"
	"time"

	"dcms/internal/model"
)

type cronJob struct {
	name  string
	every time.Duration
	fn    func(a *App)
}

// Cron регистрирует периодическую задачу (вызывается из Register модуля).
func (a *App) Cron(name string, every time.Duration, fn func(a *App)) {
	a.crons = append(a.crons, &cronJob{name, every, fn})
}

func (a *App) startCrons() {
	// ежедневная очистка (аналог cron 'everyday' старого движка)
	a.Cron("core.everyday", 24*time.Hour, func(a *App) {
		now := Now()
		a.T.Guests.DeleteWhere(func(g *model.Guests) bool { return g.DateLast < now-600 })
		a.T.VisitToday.DeleteWhere(func(v *model.VisitToday) bool { return v.Time < now-86400 })
		// неактивированные аккаунты старше суток
		for _, u := range a.T.Users.Find(func(u *model.User) bool { return u.Activation != "" && u.DateReg < now-86400 }) {
			a.DeleteUser(u.ID)
		}
		// просроченные сессии (год без активности)
		a.T.Sessions.DeleteWhere(func(s *model.Session) bool { return s.Last < now-365*86400 })
	})
	for _, j := range a.crons {
		j := j
		go func() {
			// отложенный первый запуск, чтобы не нагружать старт
			time.Sleep(5 * time.Second)
			for {
				func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("cron %s panic: %v", j.name, r)
						}
					}()
					j.fn(a)
				}()
				time.Sleep(j.every)
			}
		}()
	}
}
