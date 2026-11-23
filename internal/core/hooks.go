package core

import "dcms/internal/model"

// CaptchaCheck проверяет капчу текущего запроса (устанавливается модулем account).
var CaptchaCheck = func(c *Ctx, code string) bool { return false }

// HomeBlock регистрирует блок главной страницы (ключ → данные для фронтенда; nil — не показывать).
func (a *App) HomeBlock(key string, fn func(a *App, u *model.User) any) {
	a.homeMu.Lock()
	defer a.homeMu.Unlock()
	a.homeBlocks = append(a.homeBlocks, homeBlock{key, fn})
}

type homeBlock struct {
	key string
	fn  func(a *App, u *model.User) any
}

// HomeBlocks собирает данные всех блоков.
func (a *App) HomeBlocks(u *model.User) map[string]any {
	a.homeMu.Lock()
	bl := append([]homeBlock(nil), a.homeBlocks...)
	a.homeMu.Unlock()
	out := map[string]any{}
	for _, b := range bl {
		if v := b.fn(a, u); v != nil {
			out[b.key] = v
		}
	}
	return out
}
