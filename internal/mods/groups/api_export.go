package groups

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"dcms/internal/core"
	"dcms/internal/model"
	"dcms/internal/store"
)

func ensureSubforum(gid int64) GroupSubforum {
	list := subsT.By("group", i(gid))
	if len(list) > 0 {
		sort.Slice(list, func(x, y int) bool { return list[x].Pos < list[y].Pos || (list[x].Pos == list[y].Pos && list[x].ID < list[y].ID) })
		return list[0]
	}
	s := GroupSubforum{IDGroup: gid, Name: "Общий раздел", Opis: "Основные обсуждения сообщества", Pos: 1, Time: core.Now()}
	s.ID = subsT.Insert(&s)
	return s
}

func paginate[T any](all []T, offset, per int) ([]T, int) {
	total := len(all)
	if offset < 0 {
		offset = 0
	}
	end := offset + per
	if end > total {
		end = total
	}
	if offset >= total {
		return nil, total
	}
	return all[offset:end], total
}

// APIListGroups возвращает каталог сообществ (или сообщества пользователя, если onlyMine=true).
func APIListGroups(a *core.App, viewer *model.User, onlyMine bool, q, cat string, offset, per int) ([]map[string]any, int) {
	q = strings.ToLower(strings.TrimSpace(q))
	cat = strings.TrimSpace(cat)
	var all []Group
	if onlyMine && viewer != nil {
		for _, m := range membersT.By("user", i(viewer.ID)) {
			if g, ok := groupsT.Get(m.IDGroup); ok && !g.Blocked {
				all = append(all, g)
			}
		}
	} else {
		for _, g := range groupsT.All() {
			if g.Blocked || g.Hidden {
				continue
			}
			all = append(all, g)
		}
	}
	var filt []Group
	for _, g := range all {
		if cat != "" && g.Category != cat {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(g.Name), q) && !strings.Contains(strings.ToLower(g.Opis), q) {
			continue
		}
		filt = append(filt, g)
	}
	sort.Slice(filt, func(x, y int) bool { return filt[x].ID > filt[y].ID })
	sl, total := paginate(filt, offset, per)
	items := make([]map[string]any, 0, len(sl))
	for _, g := range sl {
		items = append(items, groupCard(a, g, viewer))
	}
	return items, total
}

// APIGetGroup возвращает карточку сообщества и счётчики разделов.
func APIGetGroup(a *core.App, viewer *model.User, gid int64) (map[string]any, error) {
	g, ok := groupsT.Get(gid)
	if !ok {
		return nil, core.ErrNotFound("Сообщество не найдено")
	}
	out := groupCard(a, g, viewer)
	out["counts"] = map[string]int{
		"blog":  blogT.CountBy("group", i(g.ID)),
		"forum": topicsT.CountBy("group", i(g.ID)),
		"chat":  chatT.CountBy("group", i(g.ID)),
		"files": filesT.CountBy("group", i(g.ID)),
	}
	return out, nil
}

// APIJoinGroup вступает в сообщество (или подаёт заявку в приватное).
func APIJoinGroup(a *core.App, me *model.User, gid int64) (map[string]any, error) {
	g, ok := groupsT.Get(gid)
	if !ok {
		return nil, core.ErrNotFound("Сообщество не найдено")
	}
	if g.Blocked {
		return nil, core.ErrForbidden("Сообщество заблокировано")
	}
	if _, exists := memberOf(g.ID, me.ID); exists {
		return map[string]any{"joined": true, "members": membersT.CountBy("group", i(g.ID))}, nil
	}
	if g.Private {
		if _, hasReq := requestsT.OneBy("pair", store.Key(g.ID, me.ID)); !hasReq {
			requestsT.Insert(&GroupRequest{IDGroup: g.ID, IDUser: me.ID, Time: core.Now()})
			for _, m := range membersT.By("group", i(g.ID)) {
				if m.Role == "creator" || m.Role == "admin" {
					a.Notify(me.ID, m.IDUser, "group_req", g.ID, false)
				}
			}
		}
		return map[string]any{"pending": true}, nil
	}
	membersT.Insert(&GroupMember{IDGroup: g.ID, IDUser: me.ID, Role: "member", Time: core.Now()})
	return map[string]any{"joined": true, "members": membersT.CountBy("group", i(g.ID))}, nil
}

// APILeaveGroup выходит из сообщества или отменяет заявку.
func APILeaveGroup(me *model.User, gid int64) error {
	g, ok := groupsT.Get(gid)
	if !ok {
		return core.ErrNotFound("Сообщество не найдено")
	}
	if g.IDCreator == me.ID {
		return core.ErrBad("Создатель не может покинуть своё сообщество")
	}
	if req, ok := requestsT.OneBy("pair", store.Key(g.ID, me.ID)); ok {
		requestsT.Delete(req.ID)
	}
	if m, ok := memberOf(g.ID, me.ID); ok {
		membersT.Delete(m.ID)
	}
	return nil
}

// APIListMembers возвращает участников сообщества и их роли.
func APIListMembers(a *core.App, viewer *model.User, gid int64, offset, per int) ([]map[string]any, int, error) {
	g, ok := groupsT.Get(gid)
	if !ok {
		return nil, 0, core.ErrNotFound("Сообщество не найдено")
	}
	if !canViewGroup(viewer, g) {
		return nil, 0, core.ErrForbidden("Нет доступа к закрытому сообществу")
	}
	all := membersT.By("group", i(g.ID))
	sort.Slice(all, func(x, y int) bool { return all[x].Time < all[y].Time })
	sl, total := paginate(all, offset, per)
	items := make([]map[string]any, 0, len(sl))
	for _, m := range sl {
		items = append(items, map[string]any{
			"user":         a.Ref(m.IDUser),
			"role":         m.Role,
			"mod_blog":     m.ModBlog,
			"mod_forum":    m.ModForum,
			"mod_chat":     m.ModChat,
			"mod_files":    m.ModFiles,
			"mod_messages": m.ModMessages,
			"time":         m.Time,
		})
	}
	return items, total, nil
}

// APIListBlog возвращает посты блога сообщества.
func APIListBlog(a *core.App, viewer *model.User, gid int64, offset, per int) ([]map[string]any, int, error) {
	g, ok := groupsT.Get(gid)
	if !ok || !g.EnableBlog {
		return nil, 0, core.ErrNotFound("Блог сообщества недоступен")
	}
	if !canViewGroup(viewer, g) {
		return nil, 0, core.ErrForbidden("Нет доступа к закрытому сообществу")
	}
	all := blogT.By("group", i(g.ID))
	sort.Slice(all, func(x, y int) bool {
		if all[x].Pinned != all[y].Pinned {
			return all[x].Pinned
		}
		return all[x].ID > all[y].ID
	})
	sl, total := paginate(all, offset, per)
	items := make([]map[string]any, 0, len(sl))
	for _, post := range sl {
		items = append(items, map[string]any{
			"id":     post.ID,
			"title":  post.Title,
			"text":   post.Msg,
			"html":   a.Text.Render(post.Msg),
			"author": a.Ref(post.IDUser),
			"pinned": post.Pinned,
			"likes":  a.T.Likes.CountBy("obj", store.Key("gblog", post.ID)),
			"komms":  blogKT.CountBy("post", i(post.ID)),
			"time":   post.Time,
		})
	}
	return items, total, nil
}

// APICreateBlogPost публикует запись в блоге сообщества.
func APICreateBlogPost(a *core.App, me *model.User, gid int64, title, msg string) (map[string]any, error) {
	g, ok := groupsT.Get(gid)
	if !ok || !g.EnableBlog || g.Blocked {
		return nil, core.ErrNotFound("Блог сообщества недоступен")
	}
	pr := permsOf(me, g)
	if !pr.ModBlog && !(g.OpenBlog && pr.IsMember) {
		return nil, core.ErrForbidden("Публиковать в блог могут только модераторы и администраторы сообщества")
	}
	title = a.Text.Clean(strings.TrimSpace(title))
	msg = a.Text.Clean(strings.TrimSpace(msg))
	if utf8.RuneCountInString(msg) < 1 || utf8.RuneCountInString(msg) > 4096 {
		return nil, core.ErrBad("Текст записи: от 1 до 4096 символов")
	}
	id := blogT.Insert(&GroupPost{IDGroup: g.ID, IDUser: me.ID, Title: title, Msg: msg, Time: core.Now()})
	p, _ := blogT.Get(id)
	return map[string]any{
		"id": p.ID, "title": p.Title, "text": p.Msg, "html": a.Text.Render(p.Msg), "author": a.Ref(p.IDUser), "time": p.Time,
	}, nil
}

// APIListSubforums возвращает подфорумы сообщества.
func APIListSubforums(viewer *model.User, gid int64) ([]map[string]any, error) {
	g, ok := groupsT.Get(gid)
	if !ok || !g.EnableForum {
		return nil, core.ErrNotFound("Форум сообщества недоступен")
	}
	if !canViewGroup(viewer, g) {
		return nil, core.ErrForbidden("Нет доступа к закрытому сообществу")
	}
	ensureSubforum(g.ID)
	list := subsT.By("group", i(g.ID))
	sort.Slice(list, func(x, y int) bool { return list[x].Pos < list[y].Pos || (list[x].Pos == list[y].Pos && list[x].ID < list[y].ID) })
	items := make([]map[string]any, 0, len(list))
	for _, s := range list {
		items = append(items, map[string]any{
			"id": s.ID, "name": s.Name, "opis": s.Opis, "pos": s.Pos, "topics": topicsT.CountBy("sub", i(s.ID)),
		})
	}
	return items, nil
}

// APIListTopics возвращает темы форума сообщества (опционально по подфоруму subID).
func APIListTopics(a *core.App, viewer *model.User, gid, subID int64, offset, per int) ([]map[string]any, int, error) {
	g, ok := groupsT.Get(gid)
	if !ok || !g.EnableForum {
		return nil, 0, core.ErrNotFound("Форум сообщества недоступен")
	}
	if !canViewGroup(viewer, g) {
		return nil, 0, core.ErrForbidden("Нет доступа к закрытому сообществу")
	}
	var all []GroupTopic
	if subID > 0 {
		all = topicsT.By("sub", i(subID))
	} else {
		all = topicsT.By("group", i(g.ID))
	}
	sort.Slice(all, func(x, y int) bool {
		if all[x].Pinned != all[y].Pinned {
			return all[x].Pinned
		}
		return all[x].TimeLast > all[y].TimeLast
	})
	sl, total := paginate(all, offset, per)
	items := make([]map[string]any, 0, len(sl))
	for _, t := range sl {
		items = append(items, map[string]any{
			"id": t.ID, "group_id": t.IDGroup, "sub_id": t.IDSub, "name": t.Name,
			"author": a.Ref(t.IDUser), "pinned": t.Pinned, "closed": t.Closed,
			"posts": tpostsT.CountBy("topic", i(t.ID)), "time": t.Time, "time_last": t.TimeLast,
		})
	}
	return items, total, nil
}

// APICreateTopic создаёт тему на форуме сообщества.
func APICreateTopic(a *core.App, me *model.User, gid, subID int64, name, msg string) (map[string]any, error) {
	g, ok := groupsT.Get(gid)
	if !ok || !g.EnableForum || g.Blocked {
		return nil, core.ErrNotFound("Форум сообщества недоступен")
	}
	if !permsOf(me, g).IsMember {
		return nil, core.ErrForbidden("Создавать темы могут только участники сообщества")
	}
	if subID == 0 {
		subID = ensureSubforum(g.ID).ID
	} else if s, ok := subsT.Get(subID); !ok || s.IDGroup != g.ID {
		return nil, core.ErrBad("Подфорум не найден")
	}
	name = a.Text.Clean(strings.TrimSpace(name))
	msg = a.Text.Clean(strings.TrimSpace(msg))
	if utf8.RuneCountInString(name) < 2 || utf8.RuneCountInString(name) > 120 {
		return nil, core.ErrBad("Название темы: от 2 до 120 символов")
	}
	if utf8.RuneCountInString(msg) < 1 || utf8.RuneCountInString(msg) > 4096 {
		return nil, core.ErrBad("Текст сообщения: от 1 до 4096 символов")
	}
	now := core.Now()
	id := topicsT.Insert(&GroupTopic{IDGroup: g.ID, IDSub: subID, IDUser: me.ID, Name: name, Msg: msg, Time: now, TimeLast: now})
	t, _ := topicsT.Get(id)
	return map[string]any{
		"id": t.ID, "group_id": t.IDGroup, "sub_id": t.IDSub, "name": t.Name, "text": t.Msg, "html": a.Text.Render(t.Msg), "author": a.Ref(t.IDUser), "time": t.Time,
	}, nil
}

// APIGetTopic возвращает тему форума и ответы в ней.
func APIGetTopic(a *core.App, viewer *model.User, tid int64, offset, per int) (map[string]any, []map[string]any, int, error) {
	t, ok := topicsT.Get(tid)
	if !ok {
		return nil, nil, 0, core.ErrNotFound("Тема не найдена")
	}
	g, ok := groupsT.Get(t.IDGroup)
	if !ok || !canViewGroup(viewer, g) {
		return nil, nil, 0, core.ErrForbidden("Нет доступа к закрытому сообществу")
	}
	all := tpostsT.By("topic", i(t.ID))
	sort.Slice(all, func(x, y int) bool { return all[x].ID < all[y].ID })
	sl, total := paginate(all, offset, per)
	posts := make([]map[string]any, 0, len(sl))
	for _, r := range sl {
		posts = append(posts, map[string]any{
			"id": r.ID, "author": a.Ref(r.IDUser), "text": r.Msg, "html": a.Text.Render(r.Msg), "time": r.Time,
		})
	}
	topic := map[string]any{
		"id": t.ID, "group_id": t.IDGroup, "sub_id": t.IDSub, "name": t.Name,
		"text": t.Msg, "html": a.Text.Render(t.Msg), "author": a.Ref(t.IDUser),
		"pinned": t.Pinned, "closed": t.Closed, "time": t.Time,
	}
	return topic, posts, total, nil
}

// APIReplyTopic добавляет ответ в тему форума сообщества.
func APIReplyTopic(a *core.App, me *model.User, tid int64, msg string) (map[string]any, error) {
	t, ok := topicsT.Get(tid)
	if !ok {
		return nil, core.ErrNotFound("Тема не найдена")
	}
	g, ok := groupsT.Get(t.IDGroup)
	if !ok || !g.EnableForum || g.Blocked {
		return nil, core.ErrNotFound("Форум сообщества недоступен")
	}
	pr := permsOf(me, g)
	if !pr.IsMember {
		return nil, core.ErrForbidden("Отвечать могут только участники сообщества")
	}
	if t.Closed && !pr.ModForum {
		return nil, core.ErrForbidden("Тема закрыта")
	}
	msg = a.Text.Clean(strings.TrimSpace(msg))
	if utf8.RuneCountInString(msg) < 1 || utf8.RuneCountInString(msg) > 4096 {
		return nil, core.ErrBad("Текст ответа: от 1 до 4096 символов")
	}
	now := core.Now()
	id := tpostsT.Insert(&GroupTopicPost{IDTopic: t.ID, IDGroup: g.ID, IDUser: me.ID, Time: now, Msg: msg})
	topicsT.Update(t.ID, func(x *GroupTopic) { x.TimeLast = now })
	return map[string]any{"id": id, "topic_id": t.ID, "author": a.Ref(me.ID), "text": msg, "html": a.Text.Render(msg), "time": now}, nil
}

// APIListChat возвращает последние сообщения чата сообщества.
func APIListChat(a *core.App, viewer *model.User, gid int64, limit int) ([]map[string]any, error) {
	g, ok := groupsT.Get(gid)
	if !ok || !g.EnableChat {
		return nil, core.ErrNotFound("Чат сообщества недоступен")
	}
	if !canViewGroup(viewer, g) {
		return nil, core.ErrForbidden("Нет доступа к закрытому сообществу")
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	all := chatT.By("group", i(g.ID))
	sort.Slice(all, func(x, y int) bool { return all[x].ID < all[y].ID })
	if len(all) > limit {
		all = all[len(all)-limit:]
	}
	items := make([]map[string]any, 0, len(all))
	for _, m := range all {
		items = append(items, map[string]any{
			"id": m.ID, "author": a.Ref(m.IDUser), "text": m.Msg, "html": a.Text.Render(m.Msg), "time": m.Time,
		})
	}
	return items, nil
}

// APISendChat отправляет сообщение в чат сообщества.
func APISendChat(a *core.App, me *model.User, gid int64, msg string) (map[string]any, error) {
	g, ok := groupsT.Get(gid)
	if !ok || !g.EnableChat || g.Blocked {
		return nil, core.ErrNotFound("Чат сообщества недоступен")
	}
	if !permsOf(me, g).IsMember {
		return nil, core.ErrForbidden("Писать в чат могут только участники сообщества")
	}
	msg = a.Text.Clean(strings.TrimSpace(msg))
	if utf8.RuneCountInString(msg) < 1 || utf8.RuneCountInString(msg) > 1024 {
		return nil, core.ErrBad("Сообщение: от 1 до 1024 символов")
	}
	now := core.Now()
	id := chatT.Insert(&GroupChat{IDGroup: g.ID, IDUser: me.ID, Time: now, Msg: msg})
	payload := map[string]any{"id": id, "group_id": g.ID, "user": a.Ref(me.ID), "msg": a.Text.Render(msg), "time": now}
	a.Hub.Publish("group:"+i(g.ID), "group_chat", payload)
	return map[string]any{"id": id, "group_id": g.ID, "author": a.Ref(me.ID), "text": msg, "html": a.Text.Render(msg), "time": now}, nil
}

// APIListFiles возвращает файлы сообщества.
func APIListFiles(a *core.App, viewer *model.User, gid int64, offset, per int) ([]map[string]any, int, error) {
	g, ok := groupsT.Get(gid)
	if !ok || !g.EnableFiles {
		return nil, 0, core.ErrNotFound("Файлы сообщества недоступны")
	}
	if !canViewGroup(viewer, g) {
		return nil, 0, core.ErrForbidden("Нет доступа к закрытому сообществу")
	}
	all := filesT.By("group", i(g.ID))
	sort.Slice(all, func(x, y int) bool { return all[x].ID > all[y].ID })
	sl, total := paginate(all, offset, per)
	items := make([]map[string]any, 0, len(sl))
	for _, f := range sl {
		items = append(items, map[string]any{
			"id": f.ID, "name": f.Name, "ext": f.Ras, "size": f.Size, "opis": f.Opis,
			"author": a.Ref(f.IDUser), "loads": f.Loads, "time": f.Time,
			"download_url": fmt.Sprintf("/api/group-files/%d/download", f.ID),
		})
	}
	return items, total, nil
}

// APISendUserDM отправляет личное сообщение от пользователя сообществу (начинает/продолжает диалог).
func APISendUserDM(a *core.App, me *model.User, gid int64, msg string) (map[string]any, error) {
	g, ok := groupsT.Get(gid)
	if !ok || g.Blocked || !g.EnableMessages {
		return nil, core.ErrForbidden("Личные сообщения в этом сообществе недоступны")
	}
	txt := a.Text.Clean(strings.TrimSpace(msg))
	if utf8.RuneCountInString(txt) < 1 || utf8.RuneCountInString(txt) > 2000 {
		return nil, core.ErrBad("Сообщение: от 1 до 2000 символов")
	}
	now := core.Now()
	id := dmT.Insert(&GroupMessage{IDGroup: g.ID, IDUser: me.ID, FromGroup: false, Msg: txt, Time: now, ReadByUsr: true})
	dto := map[string]any{"id": id, "group_id": g.ID, "user_id": me.ID, "from_group": false, "msg": a.Text.Render(txt), "raw": txt, "time": now}
	a.Hub.Publish("group_dm:"+i(g.ID), "group_dm", dto)
	return map[string]any{"id": id, "group_id": g.ID, "text": txt, "time": now}, nil
}

// APIListInbox возвращает входящие диалоги сообщества для создателя/админа/модератора сообщений.
func APIListInbox(a *core.App, me *model.User, gid int64) ([]map[string]any, error) {
	g, ok := groupsT.Get(gid)
	if !ok {
		return nil, core.ErrNotFound("Сообщество не найдено")
	}
	if !permsOf(me, g).ModMessages {
		return nil, core.ErrForbidden("Нет прав на просмотр сообщений сообщества")
	}
	type dlg struct {
		uid       int64
		initiated bool
		lastMsg   string
		lastTime  int64
		fromGroup bool
		unread    int
	}
	byU := map[int64]*dlg{}
	for _, m := range dmT.By("group", i(g.ID)) {
		d, ok := byU[m.IDUser]
		if !ok {
			d = &dlg{uid: m.IDUser}
			byU[m.IDUser] = d
		}
		if !m.FromGroup {
			d.initiated = true
			if !m.ReadByGrp {
				d.unread++
			}
		}
		if m.Time >= d.lastTime {
			d.lastTime = m.Time
			d.lastMsg = a.Text.Plain(m.Msg, 120)
			d.fromGroup = m.FromGroup
		}
	}
	var list []*dlg
	for _, d := range byU {
		if d.initiated {
			list = append(list, d)
		}
	}
	sort.Slice(list, func(x, y int) bool { return list[x].lastTime > list[y].lastTime })
	items := make([]map[string]any, 0, len(list))
	for _, d := range list {
		items = append(items, map[string]any{
			"user": a.Ref(d.uid), "last_msg": d.lastMsg, "last_time": d.lastTime, "from_group": d.fromGroup, "unread": d.unread,
		})
	}
	return items, nil
}

// APIReplyInbox отвечает пользователю от имени сообщества (строго только если пользователь писал первым).
func APIReplyInbox(a *core.App, me *model.User, gid, targetUID int64, msg string) (map[string]any, error) {
	g, ok := groupsT.Get(gid)
	if !ok || g.Blocked {
		return nil, core.ErrNotFound("Сообщество недоступно")
	}
	if !permsOf(me, g).ModMessages {
		return nil, core.ErrForbidden("Нет прав на ответы от имени сообщества")
	}
	if !hasUserInitiated(g.ID, targetUID) {
		return nil, core.ErrForbidden("Сообщество не может писать первым — пользователь должен начать диалог сам")
	}
	txt := a.Text.Clean(strings.TrimSpace(msg))
	if utf8.RuneCountInString(txt) < 1 || utf8.RuneCountInString(txt) > 2000 {
		return nil, core.ErrBad("Сообщение: от 1 до 2000 символов")
	}
	now := core.Now()
	id := dmT.Insert(&GroupMessage{IDGroup: g.ID, IDUser: targetUID, IDStaff: me.ID, FromGroup: true, Msg: txt, Time: now, ReadByGrp: true})
	dto := map[string]any{"id": id, "group_id": g.ID, "user_id": targetUID, "from_group": true, "msg": a.Text.Render(txt), "raw": txt, "time": now}
	a.Hub.Publish("group_dm:"+i(g.ID), "group_dm", dto)
	a.Hub.ToUser(targetUID, "group_dm", dto)
	a.Hub.ToUser(targetUID, "counters", nil)
	a.Notify(me.ID, targetUID, "group_dm", g.ID, false)
	return map[string]any{"id": id, "group_id": g.ID, "user_id": targetUID, "text": txt, "time": now}, nil
}
