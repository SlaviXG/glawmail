// Package telegram provides helpers for the Telegram Bot API.
package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const apiBase = "https://api.telegram.org/bot"

// Bot represents a Telegram bot with its API token.
type Bot struct {
	Token  string
	client *http.Client
}

// NewBot creates a new Bot instance.
func NewBot(token string) *Bot {
	return &Bot{
		Token: token,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// APIResponse represents a generic Telegram API response.
type APIResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result,omitempty"`
	Description string          `json:"description,omitempty"`
	ErrorCode   int             `json:"error_code,omitempty"`
}

// User represents a Telegram user.
type User struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	Username  string `json:"username,omitempty"`
}

// Message represents a Telegram message.
type Message struct {
	MessageID int64  `json:"message_id"`
	From      *User  `json:"from,omitempty"`
	Chat      *Chat  `json:"chat"`
	Text      string `json:"text,omitempty"`
}

// Chat represents a Telegram chat.
type Chat struct {
	ID int64 `json:"id"`
}

// Update represents a Telegram update.
type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message,omitempty"`
	CallbackQuery *CallbackQuery `json:"callback_query,omitempty"`
}

// CallbackQuery represents a Telegram callback query (button press).
type CallbackQuery struct {
	ID      string   `json:"id"`
	From    *User    `json:"from"`
	Message *Message `json:"message,omitempty"`
	Data    string   `json:"data,omitempty"`
}

// InlineKeyboardButton represents an inline keyboard button.
type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
}

// InlineKeyboardMarkup represents an inline keyboard.
type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

// call makes a POST request to the Telegram API.
func (b *Bot) call(method string, params map[string]interface{}) (*APIResponse, error) {
	url := apiBase + b.Token + "/" + method
	body, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var apiResp APIResponse
	if err := json.Unmarshal(data, &apiResp); err != nil {
		return nil, err
	}
	return &apiResp, nil
}

// GetMe returns information about the bot.
func (b *Bot) GetMe() (*User, error) {
	resp, err := b.call("getMe", nil)
	if err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("getMe failed: %s", resp.Description)
	}
	var user User
	if err := json.Unmarshal(resp.Result, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

// SendMessage sends a text message to a chat.
func (b *Bot) SendMessage(chatID int64, text string) (*Message, error) {
	return b.SendMessageWithMarkup(chatID, text, "", nil)
}

// SendMessageWithMarkup sends a message with optional parse mode and reply markup.
func (b *Bot) SendMessageWithMarkup(chatID int64, text, parseMode string, replyMarkup *InlineKeyboardMarkup) (*Message, error) {
	params := map[string]interface{}{
		"chat_id": chatID,
		"text":    text,
	}
	if parseMode != "" {
		params["parse_mode"] = parseMode
	}
	if replyMarkup != nil {
		params["reply_markup"] = replyMarkup
	}

	resp, err := b.call("sendMessage", params)
	if err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("sendMessage failed: %s", resp.Description)
	}
	var msg Message
	if err := json.Unmarshal(resp.Result, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// EditMessageText edits the text of a message.
func (b *Bot) EditMessageText(chatID, messageID int64, text, parseMode string) error {
	params := map[string]interface{}{
		"chat_id":      chatID,
		"message_id":   messageID,
		"text":         text,
		"reply_markup": InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{}},
	}
	if parseMode != "" {
		params["parse_mode"] = parseMode
	}
	resp, err := b.call("editMessageText", params)
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("editMessageText failed: %s", resp.Description)
	}
	return nil
}

// EditMessageReplyMarkup edits the reply markup of a message.
func (b *Bot) EditMessageReplyMarkup(chatID, messageID int64, markup *InlineKeyboardMarkup) error {
	params := map[string]interface{}{
		"chat_id":      chatID,
		"message_id":   messageID,
		"reply_markup": markup,
	}
	resp, err := b.call("editMessageReplyMarkup", params)
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("editMessageReplyMarkup failed: %s", resp.Description)
	}
	return nil
}

// AnswerCallbackQuery acknowledges a callback query.
func (b *Bot) AnswerCallbackQuery(queryID string) error {
	resp, err := b.call("answerCallbackQuery", map[string]interface{}{
		"callback_query_id": queryID,
	})
	if err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("answerCallbackQuery failed: %s", resp.Description)
	}
	return nil
}

// GetUpdates performs long polling for updates.
func (b *Bot) GetUpdates(offset int64, timeout int, allowedUpdates []string) ([]Update, error) {
	// Use longer timeout for long polling
	client := &http.Client{
		Timeout: time.Duration(timeout+10) * time.Second,
	}

	params := url.Values{}
	params.Set("offset", strconv.FormatInt(offset, 10))
	params.Set("timeout", strconv.Itoa(timeout))
	if len(allowedUpdates) > 0 {
		b, _ := json.Marshal(allowedUpdates)
		params.Set("allowed_updates", string(b))
	}

	url := apiBase + b.Token + "/getUpdates?" + params.Encode()
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var apiResp APIResponse
	if err := json.Unmarshal(data, &apiResp); err != nil {
		return nil, err
	}
	if !apiResp.OK {
		return nil, fmt.Errorf("getUpdates failed: %s", apiResp.Description)
	}

	var updates []Update
	if err := json.Unmarshal(apiResp.Result, &updates); err != nil {
		return nil, err
	}
	return updates, nil
}
