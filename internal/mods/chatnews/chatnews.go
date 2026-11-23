// Package chatnews — чат (комнаты, Умник, Шутник), админ-чат, гостевая, новости и главная страница.
package chatnews

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

func i(n int64) string { return store.I(n) }

func lenOK(s string, min, max int, what string) error {
	n := utf8.RuneCountInString(s)
	if n < min {
		return core.ErrBad(what + " слишком короткое")
	}
	if n > max {
		return core.ErrBad(fmt.Sprintf("%s слишком длинное (максимум %d)", what, max))
	}
	return nil
}

func noMat(a *core.App, s, what string) error {
	if a.Text.Censor(s) != s {
		return core.ErrBad("В " + what + " обнаружен мат")
	}
	return nil
}

func banErr(a *core.App, u *model.User, razdel, what string) error {
	if u == nil {
		return nil
	}
	if b, ok := a.ActiveBan(u.ID, razdel); ok {
		msg := "Вам запрещён доступ " + what
		if b.Prich != "" {
			msg += ". Причина: " + b.Prich
		}
		return core.ErrForbidden(msg)
	}
	return nil
}

// onlineByURL — пользователи, чей последний URL начинается с prefix, активные за последние 100 сек.
func onlineByURL(a *core.App, prefix string) []model.User {
	now := core.Now()
	return a.T.Users.Find(func(u *model.User) bool { return u.DateLast > now-100 && strings.HasPrefix(u.URL, prefix) })
}

func Register(a *core.App) {
	registerChat(a)
	registerAdminChat(a)
	registerGuest(a)
	registerNews(a)
	registerHome(a)
	registerAdmin(a)
}
