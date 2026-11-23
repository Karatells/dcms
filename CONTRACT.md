# DCMS Social → Go: контракт для разработчиков модулей

Задача проекта: **полностью** перенести функционал старого PHP-движка DCMS-Social 2.1.0 (исходники: `/home/claude/src_php`, только чтение)
на Go (бэкенд, только стандартная библиотека) + SPA-фронтенд на чистом JS (ES-модули, без сборки). Поведение, правила, права, лимиты,
начисление баллов/рейтинга, уведомления, уведомления друзьям, тексты ошибок — как в PHP. Внешний вид и серверный код — современные.

**Нельзя**: ставить пакеты (`go get`, npm, pip) — сети нет. Только stdlib. Не меняйте чужие модули.
Исходники PHP читайте ЦЕЛИКОМ для своих файлов (включая `include`-ы из `inc/`, а также `replace/` — это переопределения, которые побеждают
оригиналы: `replace/user/info/*` и т.д.). Ничего не пропускайте «по чуть-чуть»: каждая страница, форма, действие, проверка, админ-функция,
настройка раздела должны быть реализованы (кроме того, что физически не имеет смысла: Flash-плеер, WAP-темы, `mysql.php` — произвольный SQL, установщик).

## Структура
```
/home/claude/dcms
  main.go                      — регистрирует все модули (НЕ править)
  internal/store               — хранилище на MySQL / MariaDB (database/sql + go-sql-driver/mysql), generics
  internal/model/model.go      — ЦЕНТРАЛЬНЫЕ модели (User, Ban, Friends, Notification, Tape, Discussions, Likes, Bookmarks, Menu …)
  internal/core                — App, Ctx, auth, права, Config, Text(BBCode), feed-сервисы, Hub(SSE), image, registry
  internal/mods/<модуль>       — ВАШ модуль (пакет), Register(a *core.App)
  web/index.html, web/css/app.css, web/js/core.js, web/js/app.js   — оболочка и UI-кит (читать, не ломать; расширять — только добавлением)
  web/js/pages/<модуль>.js     — ВАШ файл фронтенда (+ при желании web/css/<модуль>.css, подключайте через <link> из js)
  tools/gen_structs.py         — генератор Go-структур по SQL старого движка (python3 tools/gen_structs.py таблица …)
  CONTRACT.md                  — этот файл
```
Читайте: `internal/core/*.go` (особенно app.go, users.go, feed.go, text.go, registry.go, image.go), `internal/store/store.go`, `web/js/core.js`.

## Бэкенд: основные правила
* Таблицы модуля объявляйте в своём пакете (`tables.go`) через `store.NewTable[T](a.DB, "имя")` + `.Index(...)` **внутри Register** (до `a.Start()`).
  Имя таблицы = имя таблицы старого движка (кроме центральных — они уже в `a.T`). Структуры: `store.Base` + поля, JSON-теги snake_case. Время — unix-секунды int64.
  Генератор структур: `python3 tools/gen_structs.py forum_t forum_p …` (читает /home/claude/tables.json; там схемы всех 90 таблиц старого движка).
  Дополнительные поля и индексы добавляйте свободно. Для поиска используйте индексы (`By`, `OneBy`, `CountBy`, `Select` с Index+Where+Less+Desc+Offset+Limit — возвращает страницу и total).
  Составные операции с несколькими таблицами, где важна атомарность (деньги, счётчики) — под `a.DB.Lock.Lock()`.
* Маршруты: `a.GET("/api/…/{id}", handler, core.Auth)`; middleware: `core.Auth`, `core.Level(n)`, `core.Access("имя_привилегии")` (привилегии — из seed `all_accesses`).
  Обработчик `func(c *core.Ctx) error`: `c.User()`, `c.Need()`, `c.UID()`, `c.Can("forum_post_ed")`, `c.NeedAccess`, `c.NeedLevel`, `c.PathInt("id")`, `c.Q`, `c.QInt`, `c.Bind(&v)`, `c.Page(def...)` → Pager(Page,Per,Offset),
  `c.List(items,total,pager)` (конверт `{items,total,page,per,pages}`), `c.JSON(v)`, `c.OK("k",v…)`, `c.FormFile(field,maxBytes)`, `c.FormValue`, `c.ServeFile`, `c.ServeBytes`.
  Ошибки: `return core.ErrBad("текст")`, `ErrForbidden()`, `ErrNotFound()`, `ErrAuth()`, `core.Err(code,msg)`. Тексты ошибок — по-русски, как в PHP.
  Все не-GET запросы требуют заголовок `X-DCMS: 1` (фронтенд `api.*` ставит сам). Ответы — JSON.
* Пользователи в ответах — `a.Ref(id)` (model.UserRef: id,nick,pol,online,avatar,group,level,medal,banned). Список — `a.Refs(ids...)`.
* Текст пользователей: храните сырой (после `a.Text.Clean(s)`), отдавайте `msg` (сырой, для редактирования) и `msg_html` = `a.Text.Render(raw)`. Для превью — `a.Text.Plain(raw, n)`.
* Баллы/рейтинг/деньги: `a.AddBalls(uid, balls, ratingTmp)`, `a.AddRatingTmp`, `a.AddMoney(uid, ±n) bool`. Начисления делайте ровно как в PHP (сколько баллов за пост/тему/файл и т.д.).
* Права: `a.Can(uid, "access")`; уровни групп (`user.Level`): 0 пользователь, 1 модератор раздела, 2 модератор, 3 админ, 9 главный админ, 10 создатель. Бан по разделу: `a.ActiveBan(uid, "forum")` — проверяйте перед
  записью в своём разделе (разделы бана: all, forum, chat, guest, obmen, foto, notes …) и возвращайте текст как в PHP.
* Приватность: `a.UserSettings(uid)` → PrivatStr (кто видит профиль), PrivatMail (кто пишет в почту), Ocenka (кто оценивает): 0 все / 1 друзья / 2 только я (проверьте в user/settings/settings.privacy.php). Друзья: `a.IsFriend(a,b)`.
* Ленты/обсуждения/уведомления — ТОЛЬКО через сервисы core (см. `feed.go`): `a.FanTape(...)`, `a.TapeTo`, `a.Discuss(...)`, `a.Notify(avtor,to,type,objectID,komm)`. Воспроизводите те же события, что и PHP (после новой темы форума — FanTape(KForum,"them"), и т.д.),
  с теми же значениями `type` (смотрите INSERT INTO `tape`/`discussions`/`notification` в вашем PHP).
* Описатели объектов: каждый модуль регистрирует `a.Describer("тип", fn)` для своих объектов (список типов — в registry.go): лента/обсуждения/уведомления/закладки/жалобы используют `a.Describe(type,id)` для отображения чужих объектов.
  Блоки профиля (счётчики «Форум: N тем», «Фото: N» …) — `a.ProfileStat(...)`.
* Счётчики меню/кабинета: `a.Counter("ключ", fn)`; ключ = последний сегмент URL пункта меню (news, chat, guest, obmen, forum, foto, notes, love, users) либо "mail", "myguest" и др.; формат значения как в PHP count.php (например "12" или "3/45" — на ваше усмотрение, главное строка; пусто — не показывать).
* Удаление пользователя: `a.OnUserDelete(func(uid int64){…})` — удалите всё его содержимое в вашем модуле (и файлы на диске).
* Файлы: `a.UploadDir("модуль", ...)` — каталог на диске (data/uploads/…). Раздавайте файлы своими маршрутами с проверкой прав. Изображения: `core.ImageInfo`, `core.Thumb(data,w,h,crop)`. Расширения/лимиты — как в PHP.
* Реальное время: `a.Hub.Publish("chat:3","chat",data)`, `a.Hub.ToUser(uid,"message",data)`; клиент: `events.join('chat:3', fn)`. Доступ к темам с префиксом ограничивайте `core.GuardTopic("chat:", fn)`.
* Периодические задачи: `a.Cron("имя", time.Hour, func(a *core.App){…})` (аналог «cron everyday» и чисток старого движка).
* Настройки раздела (из админки PHP: settings_forum, settings_chat, settings_foto, settings_loads …): храните в `a.Cfg` (`Str/Int/Bool/Set`), значения по умолчанию — в `core.Defaults` (добавьте ключи своими `init()`-ом: `func init(){ core.Defaults["forum_x"]="1" }`), админ-страницу и API делает владелец раздела.
* Тест: `DCMS_DEV=1` включает `POST /api/dev/login {"nick":"Admin","group":15,"pol":1}` (создаёт пользователя при отсутствии и ставит cookie). Первый созданный пользователь автоматически получает группу 15 (Создатель).
  Группы: 1 Пользователь, 2 Модератор чата, 3 форума, 4 обмена, 5 библиотеки, 6 фото, 7 Модератор, 8 Администратор, 9 Главный админ, 11 дневников, 12 гостевой, 15 Создатель.
* Эндпоинты: префикс по области (см. раздел «Модули»), REST-стиль, JSON. Не пересекайтесь с чужими префиксами.

## Фронтенд: правила
* Ваш файл `web/js/pages/<модуль>.js` — ES-модуль: `import { h, api, ui, router, state, can, icon, fmt, link, bus, events, pagedList } from '../core.js';` и вызовы `router.add('/путь/:param', async ctx => Node, {auth?:true, level?:n, access?:'имя'})`.
  `ctx`: `params`, `query`, `page` (из ?page=), `title('Заголовок')`, `onLeave(fn)`. Возвращайте DOM-элемент (собирайте через `h('div.card', …)`). Серверный HTML (`*_html`) вставляйте через `h('div.text',{html: x})`/`ui.html(x)` — он уже безопасный.
* Маршруты — «чистые» версии старых URL (например `/forum/3/5/17`, `/user/frends`, `/foto/user/5`), профиль `/id:ID` (pattern `'/id:id'`). Ссылки между модулями ведите на эти пути (договорённость ниже). Ссылки внутри SPA — обычные `<a href="/path">`.
* UI-кит: `ui.pageHead`, `ui.card/section`, `ui.form(fields,{onSubmit,submit,cancel})`, `ui.bbEditor`, `ui.userLink(ref,{avatar})`, `ui.avatar`, `ui.time(ts)`, `ui.pager(data)`, `ui.tabs`, `ui.modal/confirm/prompt`, `ui.toast/ok`, `ui.dropdown`, `ui.table`, `ui.reaction`, `ui.post`, `ui.empty/notFound/forbidden`, `ui.breadcrumbs`, `icon('имя')` (см. `iconNames` в core.js — если нужной иконки нет, добавьте в словарь `I` в core.js ТОЛЬКО новую строку).
  Классы CSS: card, pad, list/list-item, row, stack, grid, btn(.primary/.danger/.ghost/.sm), input, field, badge, muted, tabs, tiles, kv, thumb-grid, post, text… (см. `web/css/app.css`). Свои стили — в `web/css/<модуль>.css`, подключайте: `document.head.append(h('link',{rel:'stylesheet',href:'/css/<модуль>.css'}))`.
* Дизайн: аккуратно, современно, адаптивно (мобильный первым), тёмная тема через CSS-переменные (никаких жёстких цветов, кроме акцентных). Пустые состояния, загрузка, ошибки — обязательны. Подтверждения удалений — `ui.confirm`.
* Админ-страницы вашего раздела (то, что в PHP лежало в `adm_panel/` и относится к вашему модулю) делаете вы: маршруты `/admin/<что-то>`, и регистрируете ссылку: `state.adminLinks.push({group:'Форум', title:'Настройки форума', href:'/admin/forum', access:'adm_set_forum', icon:'settings', desc:'…'})`.
  Страницу-оболочку `/admin` рисует модуль admin: она показывает ссылки из `state.adminLinks`, отфильтрованные по `can(access)`.
* Все пользовательские строки — по-русски.

## Модули и владение
| модуль (Go-пакет / JS) | PHP-источники | префикс API |
|---|---|---|
| **account** | login.php, aut.php, my_aut.php, reg.php, exit.php, pass.php, secure.php, captcha.php, err.php, ban.php, ban_ip.php, go.php, rules.php(ссылка), user/settings/*, user/admin.user.php(если относится), sys/inc/captcha.php, sys/inc/sess.php, sys/inc/ipua.php, sys/fnc/aut.php, passgen/user_collision, reg_mail, активация, письма (см. mail_to_send — реализуйте очередь/лог писем), adm-часть: нет | /api/login, /api/logout, /api/register, /api/captcha, /api/pass/*, /api/settings/*, /api/secure/* |
| **profile** | info.php, avatar.php(вместе с foto), user/info/*, user/users/*, user/stena/*, user/status/*, user/komm.php, user/frends/*, user/myguest/*, ajax/php/anketa.php, replace/user/info/* | /api/user/*, /api/users/*, /api/stena/*, /api/status/*, /api/frends/*, /api/myguest/* |
| **feeds** | user/tape/*, user/discussions/*, user/notification/*, mail.php, mails.php, konts.php, new_mess.php, sys/inc/… почтовое | /api/tape/*, /api/discussions/*, /api/notification/*, /api/mail/*, /api/konts/* |
| **extras** | user/bookmark/*, user/liders/*, user/money/*, user/gift/*, user/music/*, user/love/*, adm_panel/… относящееся к подаркам | /api/bookmark/*, /api/liders/*, /api/money/*, /api/gift/*, /api/music/*, /api/love/* |
| **forum** | forum/*, adm_panel/settings_forum.php, adm_panel/forum_sinc.php, user/info/them_p.php(если связано), sys/add/… форумное | /api/forum/* |
| **chatnews** | chat/*, plugins/admin/chat/*(админ-чат), adm_panel/settings_chat.php, chat_shut.php, chat_vopr.php, guest/*, news/*, index.php(главная), sys/inc/news_main.php, main_menu.php, main_notes.php, thead/tfoot по смыслу, user/users/online(главная-блоки) | /api/chat/*, /api/adm-chat/*, /api/guest/*, /api/news/*, /api/home |
| **foto** | foto/*, avatar-логика (смена аватара, установка User.AvatarID/AvatarExt, раздача `/api/foto/img/{id}.{ext}?s=50|100|200|orig` — обязательно, на него ссылается core.AvatarURL), adm_panel/settings_foto.php | /api/foto/* |
| **files** | obmen/*, user/personalfiles/*, adm_panel/settings_loads.php, sys/inc/obmen.php, media_info, mp3-теги, sys/inc/downloadfile.php | /api/obmen/*, /api/personalfiles/* |
| **content** | plugins/notes/*, plugins/rules/*, plugins/smiles/*, ajax/smiles.php, adm_panel/smiles.php, rekl (показ рекламы: sys/fnc/rekl.php, adm_panel/rekl.php целиком), sys/inc/censure/antimat (если нужно расширить), sys/fnc/… текстовые | /api/notes/*, /api/rules/*, /api/smiles/* (в т.ч. GET /api/smiles/all → [{code,url}] для редактора), /api/rekl/* |
| **admin** | adm_panel/* (кроме разделов, отданных другим модулям), plugins/admin/index.php, plugins/admin/spam/*, adm_panel/user.php, ban.php, banlist.php, ban_ip.php, opsos.php, delete_user(s).php, rights.php, accesses.php, administration.php, menu.php, user_settings.php (меню настроек), settings_sys/user/bbcode, statistic.php, referals.php, adm_log.php, themes.php/style.php/them_installer.php (тема оформления → светлая/тёмная/акцент по умолчанию + кастом-CSS), tables.php/info.php/mysql.php (вместо SQL: информация о системе, статистика хранилища, бэкап/снимок БД, экспорт) | /api/admin/* |

Договорённые фронтенд-пути (используйте в ссылках): профиль `/id{ID}`; почта `/mail`, диалог `/mail/{id}`; друзья `/user/frends`; лента `/user/tape`; обсуждения `/user/discussions`; уведомления `/user/notification`;
закладки `/user/bookmark`; гости `/user/myguest`; подарки `/user/gift`; деньги `/user/money`; музыка `/user/music`; знакомства `/user/love`; настройки `/user/settings`; лидеры `/user/liders`; пользователи `/user/users`, онлайн `/user/users/online`;
форум `/forum`, `/forum/{forum}/{razdel}/{them}`; чат `/chat`, `/chat/{room}`; гостевая `/guest`; новости `/news`, `/news/{id}`; фото `/foto`, `/foto/user/{uid}`, `/foto/{uid}/{gallery}/{foto}`; обменник `/obmen`, `/obmen/...`; личные файлы `/user/personalfiles/{uid}`;
блоги `/notes`, `/notes/{id}`; правила `/rules`; админка `/admin`, `/admin/...`; вход `/login`, регистрация `/reg`; главная `/`.

## Перекрёстные соглашения
* **User** (core/model): аватар = `AvatarID`+`AvatarExt` (ставит foto при смене аватара вместе с `a.ForgetUser(uid)`).
* **Закладки** — центральная таблица `a.T.Bookmarks` (type: `forum`(id темы), `foto`, `files`(обменник), `notes`, `people`(id пользователя)); владелец UI/API списков — extras (`/api/bookmark/toggle {type,id}`, `/api/bookmark/list?type=`), остальные модули на своих страницах вызывают этот toggle и показывают состояние (`/api/bookmark/state?type=&id=`) — либо читают `a.T.Bookmarks` напрямую.
  В ответах объектов отдавайте `bookmarked bool` для авторизованного.
* **Лайки** — центральная `a.T.Likes` (type,id_object,like) где использует PHP like_object; специфичные рейтинги (фото, файлы форума, статусы) — как в PHP, в своих таблицах.
* **Жалобы (spam)**: `a.ReportSpam(...)`; список/обработка — admin (`plugins/admin/spam`). Каждый модуль, где в PHP есть кнопка «Пожаловаться» (`?spam=`), делает POST `/api/<модуль>/…/spam` и вызывает `ReportSpam` с теми же полями (razdel, id_post, id_object …). Владелец оригинала жалобы — по `razdel`.
* **Бан**: создание/снятие банов делает admin (`/api/admin/ban/*`) — запись в `a.T.Bans` (формат полей как в PHP: razdel, time, navsegda, view, prich, id_ban(админ), post, pochemu). Остальные модули только проверяют `a.ActiveBan`.
* **Рейтинг пользователя** (`rating` из `rating_tmp`, голосование «who_rating») — profile.
* **Админ-лог**: `a.AdminLog(uid, "Модуль", "Действие", "описание")` для всех админ-действий, как `admin_log()` в PHP.
* **Дневники в профиле / фото / файлы / форум-счётчики** — каждый модуль регистрирует `a.ProfileStat`.

## Что сдать (в финальном отчёте — коротко, ≤25 строк)
1. Какие файлы PHP перенесены целиком; 2. что сознательно изменено/опущено и почему; 3. публичные API/хуки, которые нужны другим модулям; 4. как проверяли. Код: `gofmt`, `go vet ./internal/mods/<модуль>/` без замечаний; свой dev-сервер (`cmd/dev_<модуль>/main.go` по шаблону `cmd/dev_template`) — запущен, API прогнан curl'ом (cookie jar + `X-DCMS: 1`),
страницы открыты Playwright'ом (chromium предустановлен: `require('playwright')` из node, либо python playwright) — без ошибок в консоли, скриншот(ы) просмотрены. Порты/каталоги данных: используйте свои (см. ваше задание).
