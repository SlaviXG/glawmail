// Machine A - Email Preview Bot
//
// Generates emails and shows previews to the owner with a GLAWMAIL_SEND message.
// The owner forwards the message to Machine B's bot to send the email.
//
// This component is optional - any AI with the correct skill/prompt can
// generate GLAWMAIL_SEND messages directly.
//
// First run:
//
//	go run ./setup --machine a
//	go run ./cmd/machine_a
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/SlaviXG/glawmail/internal/config"
	glawhmac "github.com/SlaviXG/glawmail/internal/hmac"
	"github.com/SlaviXG/glawmail/internal/telegram"
)

var (
	cfg         *config.MachineAConfig
	bot         *telegram.Bot
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
	cfg, err = config.LoadMachineAConfig(".env")
	if err != nil {
		fmt.Println("ERROR:", err)
		fmt.Println("Run: go run ./setup --machine a")
		os.Exit(1)
	}

	ownerChatID, _ = strconv.ParseInt(cfg.OwnerChatID, 10, 64)
	bot = telegram.NewBot(cfg.OwnBotToken)

	me, err := bot.GetMe()
	if err != nil {
		log.Fatalf("Failed to verify bot token: %v", err)
	}
	logger.Printf("Bot verified: @%s (ID: %d)", me.Username, me.ID)

	logger.Println("GlawMail preview bot ready.")

	if len(os.Args) > 1 && os.Args[1] == "test" {
		runTest()
		return
	}

	// In a real setup, this would integrate with an AI system
	// For now, just show that it's running
	bot.SendMessage(ownerChatID, "GlawMail preview bot started.\n\nUse 'glawmail-a test' to send a test email.")
	logger.Println("Waiting for AI to generate emails...")

	// Keep running (in real use, would receive emails from AI system)
	select {}
}

// generateSendMessage creates a GLAWMAIL_SEND message for the owner to forward.
func generateSendMessage(to, subject, body string, html bool, metadata map[string]string) string {
	req := EmailRequest{
		To:       to,
		Subject:  subject,
		Body:     body,
		HTML:     html,
		Metadata: metadata,
	}

	payloadBytes, _ := json.Marshal(req)
	payloadStr := string(payloadBytes)
	sig := glawhmac.Sign(cfg.WebhookSecret, payloadStr)

	return fmt.Sprintf("GLAWMAIL_SEND:%s:%s", sig, payloadStr)
}

// showEmailPreview shows the email preview to the owner with the GLAWMAIL_SEND message.
func showEmailPreview(to, subject, body string, html bool, metadata map[string]string) {
	// Show human-readable preview
	preview := fmt.Sprintf(
		"Email Preview\n\n"+
			"To: %s\n"+
			"Subject: %s\n\n"+
			"%s\n\n"+
			"Forward the next message to the Gmail bot to send.",
		to, subject, body)

	bot.SendMessage(ownerChatID, preview)

	// Send the GLAWMAIL_SEND message that can be forwarded
	sendMsg := generateSendMessage(to, subject, body, html, metadata)
	bot.SendMessage(ownerChatID, sendMsg)

	logger.Printf("Email preview shown: to=%s subject=%s", to, subject)
}

func runTest() {
	logger.Println("Sending test email preview...")

	showEmailPreview(
		"james@spotship.com",
		"Quick question about Spot Ship",
		"Hi James,\n\nI saw you're building Spot Ship - congrats on the recent raise!\n\nCurious: how much time do you spend manually reconciling Stripe payouts?\n\nHappy to chat for 20 min.\n\nSlava",
		false,
		map[string]string{"lead_id": "lead_001", "campaign": "outreach_jan_2026"},
	)

	logger.Println("Test preview sent. Forward the GLAWMAIL_SEND message to the Gmail bot.")
}
