// dcms-bot — готовый тестовый бот сообщества DCMS Social на технологии Long Polling (getUpdates).
//
// Запуск:
//
//	dcms-bot -url https://your-site.ru -token dcms_grp_...
//	dcms-bot -url https://your-site.ru -token dcms_grp_... -auto-approve
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type botClient struct {
	baseURL string
	token   string
	http    *http.Client
}

func (b *botClient) call(method, path string, body any, out any) error {
	var r io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, b.baseURL+path, r)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+b.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := b.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

type meResp struct {
	OK        bool     `json:"ok"`
	TokenName string   `json:"token_name"`
	Scopes    []string `json:"scopes"`
	Group     struct {
		ID      int64  `json:"id"`
		Name    string `json:"name"`
		Members int    `json:"members"`
	} `json:"group"`
}

type updateItem struct {
	UpdateID int64           `json:"update_id"`
	Type     string          `json:"type"`
	Time     int64           `json:"time"`
	Data     json.RawMessage `json:"data"`
}

type updatesResp struct {
	OK     bool         `json:"ok"`
	Result []updateItem `json:"result"`
}

func main() {
	siteURL := flag.String("url", os.Getenv("DCMS_BOT_URL"), "URL сайта (например: https://example.com)")
	token := flag.String("token", os.Getenv("DCMS_BOT_TOKEN"), "Токен сообщества (dcms_grp_...)")
	timeout := flag.Int("timeout", 25, "Таймаут Long Poll (сек, 1–30)")
	autoApprove := flag.Bool("auto-approve", false, "Автоматически одобрять заявки на вступление в приватное сообщество")
	flag.Parse()

	base := strings.TrimRight(strings.TrimSpace(*siteURL), "/")
	tok := strings.TrimSpace(*token)
	if base == "" || tok == "" {
		fmt.Println("Использование: dcms-bot -url https://your-site.ru -token dcms_grp_... [-auto-approve]")
		os.Exit(2)
	}

	c := &botClient{
		baseURL: base,
		token:   tok,
		http:    &http.Client{Timeout: time.Duration(*timeout+15) * time.Second},
	}

	var info meResp
	if err := c.call("GET", "/api/v1/bot/me", nil, &info); err != nil {
		log.Fatalf("Не удалось подключиться к %s: %v", base, err)
	}
	log.Printf("✓ Бот «%s» запущен для сообщества «%s» (#%d, участников: %d)", info.TokenName, info.Group.Name, info.Group.ID, info.Group.Members)
	log.Printf("✓ Разрешения токена: %s", strings.Join(info.Scopes, ", "))

	// Устанавливаем меню команд «/» для пользователей в диалоге с сообществом
	_ = c.call("POST", "/api/v1/bot/setMyCommands", map[string]any{
		"commands": []map[string]string{
			{"command": "start", "description": "Открыть главное меню и кнопки бота"},
			{"command": "ping", "description": "Проверить скорость ответа бота"},
			{"command": "info", "description": "Информация о сообществе"},
			{"command": "time", "description": "Текущее время сервера"},
			{"command": "hide", "description": "Скрыть нижнюю клавиатуру"},
		},
	}, nil)
	log.Printf("✓ Команды меню «/» зарегистрированы")
	log.Printf("✓ Ожидаю события через Long Polling (getUpdates, timeout=%ds)…", *timeout)

	// Пропускаем старые события при первом запуске, чтобы отвечать только на новые
	var offset int64
	var initUpd updatesResp
	if err := c.call("GET", "/api/v1/bot/getUpdates?offset=0&limit=100&timeout=0", nil, &initUpd); err == nil {
		for _, u := range initUpd.Result {
			if u.UpdateID >= offset {
				offset = u.UpdateID + 1
			}
		}
	}

	for {
		var upd updatesResp
		err := c.call("GET", fmt.Sprintf("/api/v1/bot/getUpdates?offset=%d&limit=50&timeout=%d", offset, *timeout), nil, &upd)
		if err != nil {
			log.Printf("Ошибка getUpdates: %v (повтор через 3с)", err)
			time.Sleep(3 * time.Second)
			continue
		}
		for _, u := range upd.Result {
			if u.UpdateID >= offset {
				offset = u.UpdateID + 1
			}
			handleUpdate(c, info, u, *autoApprove)
		}
	}
}

func handleUpdate(c *botClient, info meResp, u updateItem, autoApprove bool) {
	switch u.Type {
	case "message_new":
		var d struct {
			ID   int64  `json:"id"`
			Text string `json:"text"`
			From struct {
				ID   int64  `json:"id"`
				Nick string `json:"nick"`
			} `json:"from"`
		}
		if err := json.Unmarshal(u.Data, &d); err != nil {
			return
		}
		log.Printf("[ЛС] %s (#%d): %s", d.From.Nick, d.From.ID, d.Text)
		sendSmartReply(c, info, d.From.ID, d.From.Nick, d.Text)

	case "inline_button_click":
		var d struct {
			MessageID    int64  `json:"message_id"`
			CallbackData string `json:"callback_data"`
			Text         string `json:"text"`
			From         struct {
				ID   int64  `json:"id"`
				Nick string `json:"nick"`
			} `json:"from"`
		}
		if err := json.Unmarshal(u.Data, &d); err != nil {
			return
		}
		log.Printf("[Inline-кнопка] %s нажал «%s» (data=%s, msg=#%d)", d.From.Nick, d.Text, d.CallbackData, d.MessageID)
		reply := fmt.Sprintf("Вы нажали [b]inline-кнопку[/b] «%s» (callback_data: [code]%s[/code])!", d.Text, d.CallbackData)
		if d.CallbackData == "inline_time" {
			reply = "⏰ Текущее время сервера: [b]" + time.Now().Format("02.01.2006 15:04:05") + "[/b]"
		} else if d.CallbackData == "inline_info" {
			reply = fmt.Sprintf("📊 Сообщество «[b]%s[/b]» (#%d) · участников: %d", info.Group.Name, info.Group.ID, info.Group.Members)
		}
		_ = c.call("POST", "/api/v1/bot/sendMessage", map[string]any{"user_id": d.From.ID, "text": reply}, nil)

	case "keyboard_button_click":
		var d struct {
			CallbackData string `json:"callback_data"`
			Text         string `json:"text"`
			From         struct {
				ID   int64  `json:"id"`
				Nick string `json:"nick"`
			} `json:"from"`
		}
		if err := json.Unmarshal(u.Data, &d); err != nil {
			return
		}
		log.Printf("[Нижняя кнопка] %s нажал «%s» (data=%s)", d.From.Nick, d.Text, d.CallbackData)
		switch d.CallbackData {
		case "kb_hide":
			_ = c.call("POST", "/api/v1/bot/sendMessage", map[string]any{
				"user_id": d.From.ID, "text": "Нижняя клавиатура скрыта. Чтобы вернуть её, отправьте [b]/start[/b].", "remove_keyboard": true,
			}, nil)
		case "kb_ping":
			_ = c.call("POST", "/api/v1/bot/sendMessage", map[string]any{
				"user_id": d.From.ID, "text": "⚡ Pong! Нажатие нижней кнопки обработано мгновенно.",
			}, nil)
		case "kb_info":
			_ = c.call("POST", "/api/v1/bot/sendMessage", map[string]any{
				"user_id": d.From.ID, "text": fmt.Sprintf("📋 Сообщество «[b]%s[/b]» · участников: %d", info.Group.Name, info.Group.Members),
			}, nil)
		default:
			_ = c.call("POST", "/api/v1/bot/sendMessage", map[string]any{
				"user_id": d.From.ID, "text": fmt.Sprintf("Вы нажали кнопку меню «[b]%s[/b]»!", d.Text),
			}, nil)
		}

	case "join_request":
		var d struct {
			User struct {
				ID   int64  `json:"id"`
				Nick string `json:"nick"`
			} `json:"user"`
		}
		if err := json.Unmarshal(u.Data, &d); err != nil {
			return
		}
		log.Printf("[Заявка] %s (#%d) хочет вступить в сообщество", d.User.Nick, d.User.ID)
		if autoApprove {
			if err := c.call("POST", "/api/v1/bot/approveRequest", map[string]any{"user_id": d.User.ID}, nil); err != nil {
				log.Printf("  ✗ Ошибка одобрения: %v", err)
			} else {
				log.Printf("  ✓ Заявка %s автоматически одобрена", d.User.Nick)
			}
		}

	case "chat_message_new":
		var d struct {
			Text   string `json:"text"`
			Author struct {
				ID   int64  `json:"id"`
				Nick string `json:"nick"`
			} `json:"author"`
		}
		_ = json.Unmarshal(u.Data, &d)
		log.Printf("[Чат] %s: %s", d.Author.Nick, d.Text)
		if strings.EqualFold(strings.TrimSpace(d.Text), "!ping") {
			_ = c.call("POST", "/api/v1/bot/sendChat", map[string]any{
				"text": fmt.Sprintf("[b]%s[/b], Pong! Бот сообщества на связи ⚡", d.Author.Nick),
			}, nil)
		}

	case "forum_topic_new":
		var d struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		}
		_ = json.Unmarshal(u.Data, &d)
		log.Printf("[Форум] Новая тема #%d: %s", d.ID, d.Name)

	case "forum_post_new":
		var d struct {
			TopicID int64  `json:"topic_id"`
			Text    string `json:"text"`
		}
		_ = json.Unmarshal(u.Data, &d)
		log.Printf("[Форум] Новый ответ в теме #%d: %s", d.TopicID, d.Text)

	case "blog_post_new":
		var d struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
		}
		_ = json.Unmarshal(u.Data, &d)
		log.Printf("[Блог] Новая запись #%d: %s", d.ID, d.Title)

	default:
		log.Printf("[%s] update #%d: %s", u.Type, u.UpdateID, string(u.Data))
	}
}

func sendSmartReply(c *botClient, info meResp, uid int64, nick, text string) {
	cmd := strings.ToLower(strings.TrimSpace(text))
	payload := map[string]any{"user_id": uid}
	switch cmd {
	case "/start", "/help", "/menu", "привет", "помощь":
		payload["text"] = fmt.Sprintf("Привет, [b]%s[/b]! Я бот сообщества «[b]%s[/b]».\nПопробуйте нажать inline-кнопки прямо под этим сообщением, нижнюю клавиатуру или меню команд «/»!", nick, info.Group.Name)
		payload["inline_keyboard"] = [][]map[string]string{
			{
				{"text": "📊 Статистика", "callback_data": "inline_info", "color": "primary"},
				{"text": "⏰ Время", "callback_data": "inline_time", "color": "positive"},
			},
			{
				{"text": "👍 Класс!", "callback_data": "inline_like", "color": "default"},
			},
		}
		payload["keyboard"] = [][]map[string]string{
			{
				{"text": "📋 О сообществе", "callback_data": "kb_info", "color": "primary"},
				{"text": "⚡ Пинг", "callback_data": "kb_ping", "color": "positive"},
			},
			{
				{"text": "❌ Скрыть клавиатуру", "callback_data": "kb_hide", "color": "negative"},
			},
		}
	case "/hide":
		payload["text"] = "Клавиатура скрыта. Введите [b]/start[/b], чтобы открыть её снова."
		payload["remove_keyboard"] = true
	case "/ping":
		payload["text"] = "Pong! Бот сообщества работает в режиме Long Polling ⚡"
	case "/info":
		payload["text"] = fmt.Sprintf("Сообщество: [b]%s[/b] (#%d)\nУчастников: %d", info.Group.Name, info.Group.ID, info.Group.Members)
	case "/time":
		payload["text"] = "Текущее время сервера: [b]" + time.Now().Format("02.01.2006 15:04:05") + "[/b]"
	default:
		payload["text"] = fmt.Sprintf("Получил ваше сообщение: «%s».\nНапишите [b]/start[/b] или откройте меню команд «/».", text)
	}
	if err := c.call("POST", "/api/v1/bot/sendMessage", payload, nil); err != nil {
		log.Printf("  ✗ Не удалось ответить: %v", err)
	} else {
		log.Printf("  → Ответ отправлен %s", nick)
	}
}
