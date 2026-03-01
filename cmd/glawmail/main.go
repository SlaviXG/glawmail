// GlawMail - Gmail Sender Bot
//
// Receives email messages in a simple human-readable format.
// If the format is valid, sends via Gmail automatically.
//
// Format:
//
//	GLAWMAIL
//	To: recipient@example.com
//	Subject: Email subject
//	Body:
//	Email body text here...
//
// First run:
//
//	go run ./setup
//	go run ./cmd/glawmail
package main

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/SlaviXG/glawmail/internal/config"
	"github.com/SlaviXG/glawmail/internal/gmail"
	"github.com/SlaviXG/glawmail/internal/telegram"
)

var (
	cfg         *config.Config
	bot         *telegram.Bot
	gmailSvc    *gmail.Service
	logger      = log.New(os.Stdout, "", log.LstdFlags)
	ownerChatID int64
)

func main() {
	var err error
	cfg, err = config.Load(".env")
	if err != nil {
		fmt.Println("ERROR:", err)
		fmt.Println("Run: go run ./setup")
		os.Exit(1)
	}

	ownerChatID, _ = strconv.ParseInt(cfg.OwnerChatID, 10, 64)

	bot = telegram.NewBot(cfg.BotToken)

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

	logger.Println("GlawMail ready.")
	bot.SendMessage(ownerChatID, "GlawMail started.\n\nForward emails in this format:\n\nGLAWMAIL\nTo: email@example.com\nSubject: Subject here\nBody:\nYour message...")

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
	text := strings.TrimSpace(msg.Text)

	// Check for GLAWMAIL prefix (case insensitive)
	if !strings.HasPrefix(strings.ToUpper(text), "GLAWMAIL") {
		return
	}

	email, err := parseEmail(text)
	if err != nil {
		bot.SendMessage(ownerChatID, fmt.Sprintf("❌ %v", err))
		logger.Printf("Parse error: %v", err)
		return
	}

	// Send email
	gmailID, err := gmailSvc.SendEmail(email.To, email.Subject, email.Body, false)
	if err != nil {
		bot.SendMessage(ownerChatID, fmt.Sprintf("❌ Gmail: %v", err))
		logger.Printf("Gmail send error: %v", err)
		return
	}

	// Success
	bot.SendMessage(ownerChatID, fmt.Sprintf("✅ Sent to %s", email.To))
	logger.Printf("Email sent to %s (gmail_id=%s)", email.To, gmailID)
}

type Email struct {
	To      string
	Subject string
	Body    string
}

var emailRegex = regexp.MustCompile(`(?i)^[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}$`)

func parseEmail(text string) (*Email, error) {
	lines := strings.Split(text, "\n")

	var to, subject, body string
	var inBody bool

	for i, line := range lines {
		// Skip the GLAWMAIL header line
		if i == 0 && strings.HasPrefix(strings.ToUpper(strings.TrimSpace(line)), "GLAWMAIL") {
			continue
		}

		trimmed := strings.TrimSpace(line)

		if inBody {
			if body != "" {
				body += "\n"
			}
			body += line
			continue
		}

		// Parse To:
		if strings.HasPrefix(strings.ToLower(trimmed), "to:") {
			to = strings.TrimSpace(strings.TrimPrefix(trimmed, trimmed[:3]))
			continue
		}

		// Parse Subject:
		if strings.HasPrefix(strings.ToLower(trimmed), "subject:") {
			subject = strings.TrimSpace(strings.TrimPrefix(trimmed, trimmed[:8]))
			continue
		}

		// Parse Body:
		if strings.HasPrefix(strings.ToLower(trimmed), "body:") {
			inBody = true
			// Check if body starts on same line
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, trimmed[:5]))
			if rest != "" {
				body = rest
			}
			continue
		}
	}

	// Validate
	to = strings.TrimSpace(to)
	if to == "" {
		return nil, fmt.Errorf("missing To: field")
	}
	if !emailRegex.MatchString(to) {
		return nil, fmt.Errorf("invalid email: %s", to)
	}

	subject = strings.TrimSpace(subject)
	if subject == "" {
		return nil, fmt.Errorf("missing Subject: field")
	}

	body = strings.TrimSpace(body)
	if body == "" {
		return nil, fmt.Errorf("missing Body: field")
	}

	return &Email{To: to, Subject: subject, Body: body}, nil
}
