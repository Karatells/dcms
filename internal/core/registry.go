package core

import "dcms/internal/model"

// ---- ProfileItems: блоки статистики на странице профиля (форум, фото, файлы, дневники …)

// ProfileItem — строка «раздел: количество» в профиле.
type ProfileItem struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	Icon  string `json:"icon"`
	Link  string `json:"link"`
	Count int64  `json:"count"`
}

// ProfileStat регистрирует поставщика блоков профиля. Возвращайте пустой срез, если показывать нечего
// (например, у пользователя нет фото).
func (a *App) ProfileStat(fn func(a *App, viewer, target *model.User) []ProfileItem) {
	a.profileStats = append(a.profileStats, fn)
}

// ProfileItems собирает блоки профиля от всех модулей.
func (a *App) ProfileItems(viewer, target *model.User) []ProfileItem {
	var out []ProfileItem
	for _, fn := range a.profileStats {
		out = append(out, fn(a, viewer, target)...)
	}
	return out
}

// ---- Describer: краткое описание объекта по (type,id) — для лент, обсуждений, уведомлений, закладок, жалоб

// Desc — краткое описание объекта.
type Desc struct {
	Title string         `json:"title"`           // заголовок (название темы, файла, имя альбома …)
	Text  string         `json:"text,omitempty"`  // короткий текст (уже БЕЗ HTML, до ~200 символов)
	Link  string         `json:"link"`            // путь во фронтенде, например "/forum/3/5/17"
	Image string         `json:"image,omitempty"` // превью (url)
	Owner int64          `json:"owner"`           // автор/владелец объекта (id пользователя)
	Extra map[string]any `json:"extra,omitempty"`
}

// Describer регистрирует функцию описания объектов типа typ.
// Зарегистрированные типы (владелец модуля обязан зарегистрировать): "them"(форум, тема), "obmen"(файл обменника),
// "files"(личный файл), "foto"(фото), "album"(фотоальбом), "notes"(дневник), "status"(статус), "stena"(пост на стене),
// "news"(новость), "gift"(подарок из каталога), "user"(профиль; Link="/id<ID>"), "chat"(комната чата), "guest"(гостевая).
func (a *App) Describer(typ string, fn func(id int64) (Desc, bool)) {
	if a.describers == nil {
		a.describers = map[string]func(int64) (Desc, bool){}
	}
	a.describers[typ] = fn
}

// Describe возвращает описание объекта (false — объект удалён/неизвестный тип).
func (a *App) Describe(typ string, id int64) (Desc, bool) {
	if fn := a.describers[typ]; fn != nil {
		return fn(id)
	}
	return Desc{}, false
}
