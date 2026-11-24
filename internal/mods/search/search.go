// Package search — единый глобальный поиск по пользователям, сообществам и клипам.
package search

import (
	"sort"
	"strings"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/mods/groups"
	"dcms/internal/mods/shorts"
)

func Register(a *core.App) {
	a.GET("/api/search", func(c *core.Ctx) error {
		q := strings.TrimSpace(c.Q("q"))
		ql := strings.ToLower(q)
		if ql == "" {
			return c.JSON(map[string]any{"q": "", "users": []any{}, "groups": []any{}, "shorts": []any{}})
		}
		// Пользователи
		var matchedUsers []model.User
		for _, u := range a.T.Users.All() {
			if u.Activation != "" {
				continue
			}
			if strings.Contains(u.NickLower, ql) || strings.Contains(strings.ToLower(u.AnkName), ql) || strings.Contains(strings.ToLower(u.AnkCity), ql) {
				matchedUsers = append(matchedUsers, u)
			}
		}
		sort.Slice(matchedUsers, func(i, j int) bool { return matchedUsers[i].Rating > matchedUsers[j].Rating })
		if len(matchedUsers) > 15 {
			matchedUsers = matchedUsers[:15]
		}
		users := make([]model.UserRef, 0, len(matchedUsers))
		for _, u := range matchedUsers {
			users = append(users, a.Ref(u.ID))
		}
		// Сообщества
		grps, _ := groups.APIListGroups(a, c.User(), false, q, "", 0, 15)
		if grps == nil {
			grps = []map[string]any{}
		}
		// Клипы
		sh := shorts.SearchShorts(a, q, 12)
		if sh == nil {
			sh = []map[string]any{}
		}
		return c.JSON(map[string]any{
			"q":      q,
			"users":  users,
			"groups": grps,
			"shorts": sh,
		})
	})
}
