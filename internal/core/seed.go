package core

import (
	"dcms/internal/model"
)

// seed заполняет пустую БД начальными данными (группы, привилегии, меню).
func (a *App) seed() {
	if a.T.Groups.Len() == 0 {
		for _, g := range seedGroups {
			a.T.Groups.Insert(&model.UserGroup{Base: baseID(g.ID), Name: g.Name, Level: g.Level})
		}
		for _, x := range seedAccesses {
			a.T.AllAccesses.Insert(&model.AllAccesses{Type: x[0], Name: x[1]})
		}
		for _, x := range seedGroupAccess {
			a.T.GroupAccess.Insert(&model.UserGroupAccess{IDGroup: x.Group, IDAccess: x.Access})
		}
		// «Создатель» (15) и «Главный администратор» (9) имеют все привилегии
		for _, gid := range []int64{15, 9} {
			have := map[string]bool{}
			for _, ga := range a.T.GroupAccess.By("group", i64(gid)) {
				have[ga.IDAccess] = true
			}
			for _, x := range seedAccesses {
				if !have[x[0]] {
					a.T.GroupAccess.Insert(&model.UserGroupAccess{IDGroup: gid, IDAccess: x[0]})
				}
			}
		}
	}
	if a.T.Menu.Len() == 0 {
		for _, m := range seedMenu {
			a.T.Menu.Insert(&model.Menu{Base: baseID(m.ID), Type: m.Type, Name: m.Name, Url: m.URL, Counter: m.Counter, Pos: m.Pos, Icon: m.Icon})
		}
	} else {
		// Миграция меню для уже существующих БД: добавляем «Клипы» и «Сообщества», если их ещё нет
		hasShorts, hasGroups := false, false
		for _, m := range a.T.Menu.All() {
			if m.Url == "/shorts/" || m.Url == "/shorts" {
				hasShorts = true
			}
			if m.Url == "/groups/" || m.Url == "/groups" {
				hasGroups = true
			}
		}
		if !hasShorts {
			a.T.Menu.Insert(&model.Menu{Type: "link", Name: "Клипы", Url: "/shorts/", Counter: "shorts/count.php", Pos: 4, Icon: "video"})
		}
		if !hasGroups {
			a.T.Menu.Insert(&model.Menu{Type: "link", Name: "Сообщества", Url: "/groups/", Counter: "groups/count.php", Pos: 5, Icon: "account-multiple"})
		}
	}
	if a.T.SettingsMenu.Len() == 0 {
		for _, m := range seedUserSettings {
			a.T.SettingsMenu.Insert(&model.UserSettings{Base: baseID(m.ID), Type: m.Type, Name: m.Name, Url: m.URL, Counter: m.Counter, Pos: m.Pos, Img: m.Img, Accesses: m.Accesses, Time: Now()})
		}
	}
}
