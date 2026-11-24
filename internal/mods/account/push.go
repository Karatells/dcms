package account

import (
	"strings"

	"dcms/internal/core"
	"dcms/internal/store"
)

// PushSub — подписка браузера пользователя на системные Push-уведомления (PWA).
type PushSub struct {
	store.Base
	UserID   int64  `json:"user_id"`
	Endpoint string `json:"endpoint"`
	P256dh   string `json:"p256dh"`
	Auth     string `json:"auth"`
	Enabled  bool   `json:"enabled"`
	Created  int64  `json:"created"`
}

var pushSubs *store.Table[PushSub, *PushSub]

func registerPush(a *core.App) {
	pushSubs = store.NewTable[PushSub](a.DB, "push_subs").
		Index("user", func(x *PushSub) string { return store.I(x.UserID) })

	a.OnUserDelete(func(uid int64) {
		pushSubs.DeleteBy("user", store.I(uid))
	})

	a.GET("/api/push/status", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		subs := pushSubs.By("user", store.I(me.ID))
		return c.JSON(map[string]any{"enabled": len(subs) > 0, "count": len(subs)})
	}, core.Auth)

	a.POST("/api/push/subscribe", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		var in struct {
			Endpoint string `json:"endpoint"`
			P256dh   string `json:"p256dh"`
			Auth     string `json:"auth"`
		}
		if err := c.Bind(&in); err != nil {
			return err
		}
		ep := strings.TrimSpace(in.Endpoint)
		if ep == "" {
			ep = "browser-notification:" + store.I(me.ID)
		}
		for _, s := range pushSubs.By("user", store.I(me.ID)) {
			if s.Endpoint == ep {
				return c.OK("enabled", true)
			}
		}
		pushSubs.Insert(&PushSub{UserID: me.ID, Endpoint: ep, P256dh: in.P256dh, Auth: in.Auth, Enabled: true, Created: core.Now()})
		return c.OK("enabled", true)
	}, core.Auth)

	a.DELETE("/api/push/subscribe", func(c *core.Ctx) error {
		me, err := c.Need()
		if err != nil {
			return err
		}
		pushSubs.DeleteBy("user", store.I(me.ID))
		return c.OK("enabled", false)
	}, core.Auth)
}
