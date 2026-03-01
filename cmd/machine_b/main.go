// Machine B - Gmail Sender Bot
//
// Receives GLAWMAIL_SEND messages from the owner (forwarded from Machine A or AI).
// If the message format is valid and HMAC verifies, sends via Gmail automatically.
// Responds with success or error.
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
	"time"

	"github.com/SlaviXG/glawmail/internal/config"
	"github.com/SlaviXG/glawmail/internal/gmail"
	glawhmac "github.com/SlaviXG/glawmail/internal/hmac"
	"github.com/SlaviXG/glawmail/internal/telegram"
)

var (
	cfg         *config.MachineBConfig
	bot         *telegram.Bot
	gmailSvc    *gmail.Service
	logger      = log.New(os.Stdout, "", log.LstdFlags)
	ownerChatID int64
)

// EmailRequest represents the email send request format.
// This is the interface shared between Machine A and Machine B.
type EmailRequest struct {
	To       string            `json:"to"`
	Subject  string            `json:"subject"`
	Body     string            `json:"body"`
	HTML     bool              `json:"html,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
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

	bot = telegram.NewBot(cfg.OwnBotToken)

	me, err := bot.GetMe()
	if err != nil {
		log.Fatalf("Failed to verify bot token: %v", err)
	}
	logger.Printf("Bot verified: @%s (ID: %d)", me.Username, me.ID)

	gmailSvc, err = gmail.NewService(cfg.GmailCredentialsFile, cfg.GmailTokenFile, cfg.GmailFrom)
	if err != nil {
		log.Fatalf("Failed to initialize Gmail: %v", err)
	}
	logger.Println("Gmail service initialized")

	logger.Println("GlawMail sender ready.")
	bot.SendMessage(ownerChatID,
		"GlawMail sender started.\n\n"+
			"Forward GLAWMAIL_SEND messages to send emails.")

	logger.Println("Polling Telegram...")
	pollUpdates()
}

func pollUpdates() {
	var offset int64
	for {
		updates, err := bot.GetUpdates(offset, 30, []string{"message"})
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
		}
	}
}

func handleMessage(msg *telegram.Message) {
	text := msg.Text

	// Process GLAWMAIL_SEND messages
	if strings.HasPrefix(text, "GLAWMAIL_SEND:") {
		processSendRequest(text)
	}
}

func processSendRequest(text string) {
	// Format: GLAWMAIL_SEND:<HMAC>:<JSON>
	parts := strings.SplitN(text, ":", 3)
	if len(parts) != 3 {
		bot.SendMessage(ownerChatID, "Invalid format. Expected: GLAWMAIL_SEND:<hmac>:<json>")
		logger.Println("Invalid GLAWMAIL_SEND format - wrong number of parts")
		return
	}

	sig, payloadStr := parts[1], parts[2]

	// Verify HMAC
	if !glawhmac.Verify(cfg.WebhookSecret, payloadStr, sig) {
		bot.SendMessage(ownerChatID, "HMAC verification failed. Message rejected.")
		logger.Println("GLAWMAIL_SEND failed HMAC verification")
		return
	}

	// Parse JSON
	var req EmailRequest
	if err := json.Unmarshal([]byte(payloadStr), &req); err != nil {
		bot.SendMessage(ownerChatID, fmt.Sprintf("Invalid JSON: %v", err))
		logger.Printf("GLAWMAIL_SEND JSON parse error: %v", err)
		return
	}

	// Validate required fields
	if req.To == "" {
		bot.SendMessage(ownerChatID, "Missing required field: to")
		logger.Println("GLAWMAIL_SEND missing 'to' field")
		return
	}
	if req.Subject == "" {
		bot.SendMessage(ownerChatID, "Missing required field: subject")
		logger.Println("GLAWMAIL_SEND missing 'subject' field")
		return
	}
	if req.Body == "" {
		bot.SendMessage(ownerChatID, "Missing required field: body")
		logger.Println("GLAWMAIL_SEND missing 'body' field")
		return
	}

	// Send email
	gmailID, err := gmailSvc.SendEmail(req.To, req.Subject, req.Body, req.HTML)
	if err != nil {
		bot.SendMessage(ownerChatID, fmt.Sprintf("Gmail error: %v", err))
		logger.Printf("Gmail send error: %v", err)
		return
	}

	// Success
	bot.SendMessage(ownerChatID, fmt.Sprintf("Sent to %s", req.To))
	logger.Printf("Email sent to %s (gmail_id=%s)", req.To, gmailID)
}
