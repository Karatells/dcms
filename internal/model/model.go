// Package model — центральные (сквозные) таблицы. Модули добавляют собственные таблицы в своих пакетах.
//
// Соглашения: время — unix-секунды (int64); имена полей повторяют колонки старого движка
// (id_user -> IDUser); булевы флаги enum('0','1') — bool.
package model

import "dcms/internal/store"

// User — пользователь (таблица `user`). Пароль хранится как PBKDF2-хеш (PassHash), старые поля ip_cl/ip_xff удалены.
type User struct {
	store.Base
	Nick        string `json:"nick"`
	NickLower   string `json:"nick_lower"`
	PassHash    string `json:"-"`
	Activation  string `json:"-"` // непусто — аккаунт ещё не активирован
	IP          string `json:"ip"`
	UA          string `json:"ua"`
	Browser     string `json:"browser"` // wap|web
	URL         string `json:"url"`     // последняя страница
	ShowURL     bool   `json:"show_url"`
	DateReg     int64  `json:"date_reg"`
	DateAut     int64  `json:"date_aut"`
	DateLast    int64  `json:"date_last"`
	Time        int64  `json:"time"` // суммарное время на сайте (сек)
	Balls       int64  `json:"balls"`
	Rating      int64  `json:"rating"`
	RatingTmp   int64  `json:"rating_tmp"`
	Money       int64  `json:"money"`
	GroupAccess int64  `json:"group_access"` // id группы
	Level       int64  `json:"level"`        // уровень группы (кеш)
	Pol         int64  `json:"pol"`          // 1 — муж, 0 — жен
	Email       string `json:"-"`            // ank_mail
	ShowMail    bool   `json:"show_mail"`
	// анкета
	AnkGR            int64  `json:"ank_g_r"`
	AnkMR            int64  `json:"ank_m_r"`
	AnkDR            int64  `json:"ank_d_r"`
	AnkCity          string `json:"ank_city"`
	AnkOSebe         string `json:"ank_o_sebe"`
	AnkSkype         string `json:"ank_skype"`
	AnkTelegram      string `json:"ank_telegram"`
	AnkVk            string `json:"ank_vk"`
	AnkNTel          string `json:"ank_n_tel"`
	AnkName          string `json:"ank_name"`
	AnkMaritalStatus int64  `json:"ank_marital_status"`
	AnkLov1          int64  `json:"ank_lov_1"`
	AnkLov2          int64  `json:"ank_lov_2"`
	AnkLov3          int64  `json:"ank_lov_3"`
	AnkLov4          int64  `json:"ank_lov_4"`
	AnkLov5          int64  `json:"ank_lov_5"`
	AnkLov6          int64  `json:"ank_lov_6"`
	AnkLov7          int64  `json:"ank_lov_7"`
	AnkLov8          int64  `json:"ank_lov_8"`
	AnkLov9          int64  `json:"ank_lov_9"`
	AnkLov10         int64  `json:"ank_lov_10"`
	AnkLov11         int64  `json:"ank_lov_11"`
	AnkLov12         int64  `json:"ank_lov_12"`
	AnkLov13         int64  `json:"ank_lov_13"`
	AnkLov14         int64  `json:"ank_lov_14"`
	AnkLov15         int64  `json:"ank_lov_15"`
	AnkZan           string `json:"ank_zan"`
	AnkSmok          int64  `json:"ank_smok"`
	AnkBaby          int64  `json:"ank_baby"`
	AnkAlkoN         int64  `json:"ank_alko_n"`
	// настройки
	SetTimeChat   int64  `json:"set_time_chat"`
	SetPStr       int64  `json:"set_p_str"`
	SetShowIcon   int64  `json:"set_show_icon"`
	SetTranslit   bool   `json:"set_translit"`
	SetFiles      bool   `json:"set_files"`
	SetTimesdvig  int64  `json:"set_timesdvig"`
	SetNewsToMail bool   `json:"set_news_to_mail"`
	SetNick       int64  `json:"set_nick"`
	Theme         string `json:"theme"` // light|dark|auto
	Accent        string `json:"accent"`
	AddKonts      int64  `json:"add_konts"` // кто может добавлять в контакты: 0 все,1 друзья,2 никто
	Wall          int64  `json:"wall"`      // кто может писать на стене
	Sort          int64  `json:"sort"`
	NewsRead      int64  `json:"news_read"`
	Abuld         bool   `json:"abuld"`
	GuestMess     int64  `json:"guest_mess"`
	ForumThemComm int64  `json:"forum_them_comm"`
	MeteoCountry  int64  `json:"meteo_country"`
	AvatarID      int64  `json:"avatar_id"` // id фото-аватара в галерее (0 — нет)
	AvatarExt     string `json:"avatar_ext"`
	Verified      bool   `json:"verified"`       // синяя галочка верификации
	TOTPSecret    string `json:"totp_secret"`    // секрет 2FA (Base32)
	TOTPEnabled   bool   `json:"totp_enabled"`   // включена ли 2FA
	VIPUntil      int64  `json:"vip_until"`      // VIP-подписка активна до (unix)
	VIPBadge      string `json:"vip_badge"`      // иконка возле ника (эмодзи)
	VIPNickStyle  string `json:"vip_nick_style"` // стиль градиента ника
	VIPCover      string `json:"vip_cover"`      // стиль обложки профиля
	VIPStealth    bool   `json:"vip_stealth"`    // невидимка в гостях
	VIPTop        bool   `json:"vip_top"`        // показывать анкету в Топе / Знакомствах
	VIPTopMsg     string `json:"vip_top_msg"`    // подпись анкеты в Топе / Знакомствах
}

// Session — сессия входа.
type Session struct {
	store.Base
	Token   string `json:"token"`
	UserID  int64  `json:"user_id"`
	Created int64  `json:"created"`
	Last    int64  `json:"last"`
	IP      string `json:"ip"`
	UA      string `json:"ua"`
}

// KV — настройка системы (ключ-значение), редактируется в админке.
type KV struct {
	store.Base
	Key   string `json:"key"`
	Value string `json:"value"`
}

// UserRef — краткая карточка пользователя в ответах API.
type UserRef struct {
	ID      int64  `json:"id"`
	Nick    string `json:"nick"`
	Pol     int64  `json:"pol"`
	Online  bool   `json:"online"`
	Mobile  bool   `json:"mobile,omitempty"`
	Avatar  string `json:"avatar"` // url или ""
	Group   string `json:"group"`
	Level   int64  `json:"level"`
	Medal   int    `json:"medal"` // 0..7 по рейтингу
	Banned       bool   `json:"banned,omitempty"`
	Deleted      bool   `json:"deleted,omitempty"`
	Verified     bool   `json:"verified,omitempty"`
	Accent       string `json:"accent,omitempty"`
	VIP          bool   `json:"vip,omitempty"`
	VIPBadge     string `json:"vip_badge,omitempty"`
	VIPNickStyle string `json:"vip_nick_style,omitempty"`
}

// UserGroup — таблица `user_group` старого движка.
type UserGroup struct {
	store.Base
	Name  string `json:"name"`
	Level int64  `json:"level"`
}

// UserGroupAccess — таблица `user_group_access` старого движка.
type UserGroupAccess struct {
	store.Base
	IDGroup  int64  `json:"id_group"`
	IDAccess string `json:"id_access"`
}

// AllAccesses — таблица `all_accesses` старого движка.
type AllAccesses struct {
	store.Base
	Type string `json:"type"`
	Name string `json:"name"`
}

// Ban — таблица `ban` старого движка.
type Ban struct {
	store.Base
	Time     int64  `json:"time"`
	IDUser   int64  `json:"id_user"`
	IDBan    int64  `json:"id_ban"`
	Prich    string `json:"prich"`
	View     bool   `json:"view"`
	Razdel   string `json:"razdel"`
	Post     int64  `json:"post"`
	Pochemu  int64  `json:"pochemu"`
	Navsegda int64  `json:"navsegda"`
}

// Frends — таблица `frends` старого движка.
type Frends struct {
	store.Base
	User            int64 `json:"user"`
	Frend           int64 `json:"frend"`
	Time            int64 `json:"time"`
	I               int64 `json:"i"`
	LentaForum      int64 `json:"lenta_forum"`
	LentaObmen      int64 `json:"lenta_obmen"`
	LentaFoto       int64 `json:"lenta_foto"`
	LentaNotes      int64 `json:"lenta_notes"`
	LentaAvatar     int64 `json:"lenta_avatar"`
	LentaFrends     int64 `json:"lenta_frends"`
	LentaStatus     int64 `json:"lenta_status"`
	LentaStatusLike int64 `json:"lenta_status_like"`
	DiscForum       int64 `json:"disc_forum"`
	DiscObmen       int64 `json:"disc_obmen"`
	DiscFoto        int64 `json:"disc_foto"`
	DiscNotes       int64 `json:"disc_notes"`
	DiscFrends      int64 `json:"disc_frends"`
	DiscStatus      int64 `json:"disc_status"`
}

// FrendsNew — таблица `frends_new` старого движка.
type FrendsNew struct {
	store.Base
	User int64 `json:"user"`
	To   int64 `json:"to"`
	Time int64 `json:"time"`
}

// Notification — таблица `notification` старого движка.
type Notification struct {
	store.Base
	IDUser   int64  `json:"id_user"`
	Time     int64  `json:"time"`
	IDObject int64  `json:"id_object"`
	Type     string `json:"type"`
	Avtor    int64  `json:"avtor"`
	AllType  string `json:"all_type"`
	Read     int64  `json:"read"`
}

// NotificationSet — таблица `notification_set` старого движка.
type NotificationSet struct {
	store.Base
	IDUser int64 `json:"id_user"`
	Komm   int64 `json:"komm"`
}

// Tape — таблица `tape` старого движка.
type Tape struct {
	store.Base
	IDUser int64  `json:"id_user"`
	Count  int64  `json:"count"`
	IDFile int64  `json:"id_file"`
	Avtor  int64  `json:"avtor"`
	Time   int64  `json:"time"`
	Type   string `json:"type"`
	Read   bool   `json:"read"`
	Avatar int64  `json:"avatar"`
	OtKogo int64  `json:"ot_kogo"`
}

// TapeSet — таблица `tape_set` старого движка.
type TapeSet struct {
	store.Base
	IDUser          int64 `json:"id_user"`
	LentaStatusLike int64 `json:"lenta_status_like"`
	LentaStatus     int64 `json:"lenta_status"`
	LentaFoto       int64 `json:"lenta_foto"`
	LentaFiles      int64 `json:"lenta_files"`
	LentaForum      int64 `json:"lenta_forum"`
	LentaNotes      int64 `json:"lenta_notes"`
	LentaAvatar     int64 `json:"lenta_avatar"`
	LentaFrends     int64 `json:"lenta_frends"`
}

// Discussions — таблица `discussions` старого движка.
type Discussions struct {
	store.Base
	Avtor  int64  `json:"avtor"`
	IDUser int64  `json:"id_user"`
	Count  int64  `json:"count"`
	Msg    string `json:"msg"`
	Time   int64  `json:"time"`
	Type   string `json:"type"`
	IDSim  int64  `json:"id_sim"`
}

// DiscussionsSet — таблица `discussions_set` старого движка.
type DiscussionsSet struct {
	store.Base
	IDUser     int64 `json:"id_user"`
	DiscStatus int64 `json:"disc_status"`
	DiscFoto   int64 `json:"disc_foto"`
	DiscFiles  int64 `json:"disc_files"`
	DiscForum  int64 `json:"disc_forum"`
	DiscNotes  int64 `json:"disc_notes"`
}

// Guests — таблица `guests` старого движка.
type Guests struct {
	store.Base
	IP       int64  `json:"ip"`
	Ua       string `json:"ua"`
	DateAut  int64  `json:"date_aut"`
	DateLast int64  `json:"date_last"`
	Url      string `json:"url"`
	Pereh    int64  `json:"pereh"`
}

// UserCollision — таблица `user_collision` старого движка.
type UserCollision struct {
	store.Base
	IDUser  int64  `json:"id_user"`
	IDUser2 int64  `json:"id_user2"`
	Type    string `json:"type"`
}

// UserLog — таблица `user_log` старого движка.
type UserLog struct {
	store.Base
	IDUser int64  `json:"id_user"`
	Method bool   `json:"method"`
	Time   int64  `json:"time"`
	IP     int64  `json:"ip"`
	Ua     string `json:"ua"`
}

// UserSet — таблица `user_set` старого движка.
type UserSet struct {
	store.Base
	IDUser            int64 `json:"id_user"`
	PrivatStr         int64 `json:"privat_str"`
	PrivatMail        int64 `json:"privat_mail"`
	Ocenka            int64 `json:"ocenka"`
	BlockGroupInvites bool  `json:"block_group_invites"`
}

// Spamus — таблица `spamus` старого движка.
type Spamus struct {
	store.Base
	IDUser   int64  `json:"id_user"`
	Msg      string `json:"msg"`
	Razdel   string `json:"razdel"`
	IDSpam   int64  `json:"id_spam"`
	Types    int64  `json:"types"`
	Time     int64  `json:"time"`
	IDPost   int64  `json:"id_post"`
	Spam     string `json:"spam"`
	IDObject int64  `json:"id_object"`
}

// LikeObject — таблица `like_object` старого движка.
type LikeObject struct {
	store.Base
	IDUser   int64  `json:"id_user"`
	Type     string `json:"type"`
	Like     bool   `json:"like"`
	IDObject int64  `json:"id_object"`
}

// Bookmarks — таблица `bookmarks` старого движка.
type Bookmarks struct {
	store.Base
	IDUser   int64  `json:"id_user"`
	IDObject int64  `json:"id_object"`
	Time     int64  `json:"time"`
	Type     string `json:"type"`
	TimeObn  int64  `json:"time_obn"` // время последнего просмотра/обновления (для «новых» в закладках форума)
}

// AdminLog — таблица `admin_log` старого движка.
type AdminLog struct {
	store.Base
	IDUser int64  `json:"id_user"`
	Time   int64  `json:"time"`
	Mod    int64  `json:"mod"`
	Act    int64  `json:"act"`
	Opis   string `json:"opis"`
}

// AdminLogMod — таблица `admin_log_mod` старого движка.
type AdminLogMod struct {
	store.Base
	Name string `json:"name"`
}

// AdminLogAct — таблица `admin_log_act` старого движка.
type AdminLogAct struct {
	store.Base
	IDMod int64  `json:"id_mod"`
	Name  string `json:"name"`
}

// VisitEveryday — таблица `visit_everyday` старого движка.
type VisitEveryday struct {
	store.Base
	Time     int64 `json:"time"`
	Host     int64 `json:"host"`
	Hit      int64 `json:"hit"`
	HostIpUa int64 `json:"host_ip_ua"`
}

// VisitToday — таблица `visit_today` старого движка.
type VisitToday struct {
	store.Base
	IP   int64  `json:"ip"`
	Ua   string `json:"ua"`
	Time int64  `json:"time"`
}

// BanIp — таблица `ban_ip` старого движка.
type BanIp struct {
	store.Base
	Min int64 `json:"min"`
	Max int64 `json:"max"`
}

// Opsos — таблица `opsos` старого движка.
type Opsos struct {
	store.Base
	Min   int64  `json:"min"`
	Max   int64  `json:"max"`
	Opsos string `json:"opsos"`
}

// Menu — таблица `menu` старого движка.
type Menu struct {
	store.Base
	Type    string `json:"type"`
	Name    string `json:"name"`
	Url     string `json:"url"`
	Counter string `json:"counter"`
	Pos     int64  `json:"pos"`
	Icon    string `json:"icon"`
}

// UserSettings — таблица `user_settings` старого движка.
type UserSettings struct {
	store.Base
	Type     string `json:"type"`
	Name     string `json:"name"`
	Url      string `json:"url"`
	Counter  string `json:"counter"`
	Pos      int64  `json:"pos"`
	Img      string `json:"img"`
	Accesses string `json:"accesses"`
	Time     int64  `json:"time"`
}
