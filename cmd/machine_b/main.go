// Machine B - Approval Bot + Gmail Sender
//
// Polls its own bot for GLAWMAIL_APPROVAL_REQUEST messages from Machine A.
// Shows email previews with Approve/Decline buttons to the owner.
//   - Approve -> sends email via Gmail API, notifies Machine A
//   - Decline -> notifies Machine A
//   - Error   -> notifies Machine A
//
// First run:
//
//	go run ./setup --machine b
//	go run ./cmd/machine_b
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SlaviXG/glawmail/internal/config"
	"github.com/SlaviXG/glawmail/internal/gmail"
	glawhmac "github.com/SlaviXG/glawmail/internal/hmac"
	"github.com/SlaviXG/glawmail/internal/pairing"
	"github.com/SlaviXG/glawmail/internal/telegram"
)

var (
	cfg         *config.MachineBConfig
	bot         *telegram.Bot
	gmailSvc    *gmail.Service
	logger      = log.New(os.Stdout, "", log.LstdFlags)
	pendingMu   sync.Mutex
	pending     = make(map[string]EmailData)
	ownerChatID int64
	peerChatID  int64
)

// EmailData represents an email pending approval.
type EmailData struct {
	CallbackID string            `json:"callback_id"`
	To         string            `json:"to"`
	Subject    string            `json:"subject"`
	Body       string            `json:"body"`
	HTML       bool              `json:"html"`
	Metadata   map[string]string `json:"metadata"`
}

func main() {
	var err error
	cfg, err = config.LoadMachineBConfig(".env")
	if err != nil {
		fmt.Println("ERROR:", err)
		fmt.Println("Run: go run ./setup --machine b")
		os.Exit(1)
	}

	ownerChatID, _ = strconv.ParseInt(cfg.OwnerChatID, 10, 64)
	peerChatID, _ = strconv.ParseInt(cfg.OpenclawBotChatID, 10, 64)

	bot = telegram.NewBot(cfg.OwnBotToken)

	me, err := bot.GetMe()
	if err != nil {
		log.Fatalf("Failed to verify bot token: %v", err)
	}
	logger.Printf("Bot verified: @%s (ID: %d)", me.Username, me.ID)

	gmailSvc, err = gmail.NewService(cfg.GmailTokenFile, cfg.GmailFrom)
	if err != nil {
		log.Fatalf("Failed to initialize Gmail: %v", err)
	}
	logger.Println("Gmail service initialized")

	var offset int64

	if !pairing.IsPaired("") {
		logger.Println("Not yet paired - waiting for pairing handshake from Machine A...")
		var success bool
		success, offset = runPairing(offset)
		if !success {
			logger.Println("Pairing failed. Exiting.")
			os.Exit(1)
		}
	}

	logger.Println("Approval bot running - polling Telegram...")
	pollUpdates(offset)
}

func runPairing(offset int64) (bool, int64) {
	deadline := time.Now().Add(pairing.PairCodeTTL)
	var pendingCode, pendingSender string

	logger.Println("Waiting for GLAWMAIL_PAIR_REQUEST from @openclawbot...")
	bot.SendMessageWithMarkup(ownerChatID,
		"Glawmail approval bot started.\n\n"+
			"Waiting for Machine A to initiate pairing...\n"+
			"Make sure machine_a is running on Machine A.",
		"", nil)

	for time.Now().Before(deadline) {
		updates, err := bot.GetUpdates(offset, 10, []string{"message"})
		if err != nil {
			logger.Printf("Pairing poll error: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		for _, update := range updates {
			offset = update.UpdateID + 1
			if update.Message == nil || update.Message.From == nil {
				continue
			}

			senderID := strconv.FormatInt(update.Message.From.ID, 10)
			text := strings.TrimSpace(update.Message.Text)

			// Step 1: receive PAIR_REQUEST from Machine A's bot
			if pendingCode == "" {
				prefix, code, ok := pairing.ParsePairMessage(text)
				if !ok || prefix != pairing.PairRequestPrefix {
					continue
				}
				if senderID != cfg.OpenclawBotChatID {
					logger.Printf("Pairing request from unexpected sender %s - ignoring", senderID)
					continue
				}
				pendingCode = code
				pendingSender = senderID
				logger.Printf("Received GLAWMAIL_PAIR_REQUEST - code=%s", code)
				bot.SendMessageWithMarkup(ownerChatID,
					fmt.Sprintf("Pairing request received from Machine A.\n\n"+
						"Code: <b>%s</b>\n\n"+
						"Type /confirm to approve this pairing, "+
						"or /cancel to reject it.", code),
					"HTML", nil)
				continue
			}

			// Step 2: wait for owner to confirm
			if text == "/confirm" {
				if strconv.FormatInt(update.Message.From.ID, 10) != cfg.OwnerChatID {
					continue
				}
				bot.SendMessage(peerChatID, pairing.MakePairConfirm(pendingCode))
				if err := pairing.RecordPairing("", pendingSender); err != nil {
					logger.Printf("Failed to record pairing: %v", err)
					return false, offset
				}
				bot.SendMessageWithMarkup(ownerChatID,
					"Pairing complete. Machine B is now linked to Machine A.",
					"", nil)
				logger.Printf("Pairing complete - peer locked to bot ID %s", pendingSender)
				return true, offset
			}

			if text == "/cancel" {
				if strconv.FormatInt(update.Message.From.ID, 10) != cfg.OwnerChatID {
					continue
				}
				pendingCode = ""
				pendingSender = ""
				bot.SendMessageWithMarkup(ownerChatID,
					"Pairing cancelled. Waiting for a new request...",
					"", nil)
				logger.Println("Owner cancelled pairing")
			}
		}
	}

	logger.Println("Pairing timed out.")
	bot.SendMessageWithMarkup(ownerChatID,
		"Pairing timed out. Restart both machines to try again.",
		"", nil)
	return false, offset
}

func pollUpdates(offset int64) {
	for {
		updates, err := bot.GetUpdates(offset, 30, []string{"message", "callback_query"})
		if err != nil {
			if !strings.Contains(err.Error(), "timeout") {
				logger.Printf("Polling error: %v - retrying in 5s", err)
				time.Sleep(5 * time.Second)
			}
			continue
		}

		for _, update := range updates {
			offset = update.UpdateID + 1

			if update.Message != nil {
				handleMessage(update.Message)
			}
			if update.CallbackQuery != nil {
				handleCallbackQuery(update.CallbackQuery)
			}
		}
	}
}

func handleMessage(msg *telegram.Message) {
	if msg.From == nil {
		return
	}

	senderID := strconv.FormatInt(msg.From.ID, 10)
	text := msg.Text

	if !pairing.IsTrustedSender(senderID, "") {
		if strings.HasPrefix(text, "GLAWMAIL_") {
			logger.Printf("Dropped GLAWMAIL message from untrusted sender %s", senderID)
		}
		return
	}

	if strings.HasPrefix(text, "GLAWMAIL_APPROVAL_REQUEST:") {
		processApprovalRequest(text)
	}
}

func processApprovalRequest(text string) {
	parts := strings.SplitN(text, ":", 3)
	if len(parts) != 3 {
		return
	}
	sig, payloadStr := parts[1], parts[2]

	if !glawhmac.Verify(cfg.WebhookSecret, payloadStr, sig) {
		logger.Println("GLAWMAIL_APPROVAL_REQUEST failed HMAC verification - ignoring")
		return
	}

	var data EmailData
	if err := json.Unmarshal([]byte(payloadStr), &data); err != nil {
		logger.Printf("Failed to parse approval request: %v", err)
		return
	}

	if data.CallbackID == "" || data.To == "" {
		logger.Println("GLAWMAIL_APPROVAL_REQUEST missing callback_id or to - ignoring")
		return
	}

	pendingMu.Lock()
	pending[data.CallbackID] = data
	pendingMu.Unlock()

	preview := fmt.Sprintf(
		"Email Approval Request\n\n"+
			"<b>To:</b> %s\n"+
			"<b>Subject:</b> %s\n\n"+
			"<b>Body:</b>\n%s\n\n"+
			"<i>Tap Send to approve, Decline to reject and notify the AI.</i>",
		data.To, data.Subject, data.Body)

	keyboard := &telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{{
			{Text: "Send Email", CallbackData: "approve:" + data.CallbackID},
			{Text: "Decline", CallbackData: "decline:" + data.CallbackID},
		}},
	}

	_, err := bot.SendMessageWithMarkup(ownerChatID, preview, "HTML", keyboard)
	if err != nil {
		pendingMu.Lock()
		delete(pending, data.CallbackID)
		pendingMu.Unlock()
		sendToOpenclaw("GLAWMAIL_ERROR", data, map[string]string{
			"stage":  "telegram_preview",
			"reason": err.Error(),
		})
		logger.Printf("Failed to send preview to owner for %s: %v", data.CallbackID, err)
		return
	}

	logger.Printf("Approval request queued: %s to %s", data.CallbackID, data.To)
}

func handleCallbackQuery(cq *telegram.CallbackQuery) {
	fromID := strconv.FormatInt(cq.From.ID, 10)
	if fromID != cfg.OwnerChatID {
		logger.Printf("Ignoring callback from non-owner %s", fromID)
		return
	}

	bot.AnswerCallbackQuery(cq.ID)

	parts := strings.SplitN(cq.Data, ":", 2)
	if len(parts) != 2 {
		return
	}
	action, callbackID := parts[0], parts[1]
	messageID := cq.Message.MessageID

	pendingMu.Lock()
	emailData, exists := pending[callbackID]
	if exists {
		delete(pending, callbackID)
	}
	pendingMu.Unlock()

	if !exists {
		bot.EditMessageText(ownerChatID, messageID, "Email not found - already processed?", "")
		return
	}

	switch action {
	case "approve":
		gmailID, err := gmailSvc.SendEmail(emailData.To, emailData.Subject, emailData.Body, emailData.HTML)
		if err != nil {
			pendingMu.Lock()
			pending[callbackID] = emailData
			pendingMu.Unlock()

			bot.EditMessageText(ownerChatID, messageID,
				fmt.Sprintf("Send failed: %s\n\n<i>Email restored - tap Send to retry.</i>", err.Error()),
				"HTML")

			keyboard := &telegram.InlineKeyboardMarkup{
				InlineKeyboard: [][]telegram.InlineKeyboardButton{{
					{Text: "Send Email", CallbackData: "approve:" + callbackID},
					{Text: "Decline", CallbackData: "decline:" + callbackID},
				}},
			}
			bot.EditMessageReplyMarkup(ownerChatID, messageID, keyboard)

			sendToOpenclaw("GLAWMAIL_ERROR", emailData, map[string]string{
				"stage":  "gmail_send",
				"reason": err.Error(),
			})
			logger.Printf("Gmail error for %s: %v", callbackID, err)
			return
		}

		bot.EditMessageText(ownerChatID, messageID,
			fmt.Sprintf("Sent to <b>%s</b>\n<i>Gmail ID: %s</i>", emailData.To, gmailID),
			"HTML")
		sendToOpenclaw("GLAWMAIL_APPROVED", emailData, map[string]string{"gmail_id": gmailID})
		logger.Printf("Email %s sent to %s", callbackID, emailData.To)

	case "decline":
		bot.EditMessageText(ownerChatID, messageID,
			fmt.Sprintf("Declined - email to <b>%s</b> discarded.\n<i>Notifying AI...</i>", emailData.To),
			"HTML")
		sendToOpenclaw("GLAWMAIL_DECLINE", emailData, map[string]string{"reason": "declined_by_owner"})
		logger.Printf("Email %s declined", callbackID)
	}
}

func sendToOpenclaw(prefix string, data EmailData, extra map[string]string) {
	payload := map[string]interface{}{
		"callback_id": data.CallbackID,
		"to":          data.To,
		"subject":     data.Subject,
		"metadata":    data.Metadata,
	}
	for k, v := range extra {
		payload[k] = v
	}

	payloadBytes, _ := json.Marshal(payload)
	payloadStr := string(payloadBytes)
	sig := glawhmac.Sign(cfg.WebhookSecret, payloadStr)

	message := fmt.Sprintf("%s:%s:%s", prefix, sig, payloadStr)
	_, err := bot.SendMessage(peerChatID, message)
	if err != nil {
		logger.Printf("Failed to send %s to @openclawbot: %v", prefix, err)
	} else {
		logger.Printf("%s sent to @openclawbot for callback_id=%s", prefix, data.CallbackID)
	}
}
