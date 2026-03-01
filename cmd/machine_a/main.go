// Machine A - Email Preview Bot (Optional)
//
// Generates emails and shows previews to the owner in GLAWMAIL format.
// The owner forwards the message to Machine B's bot to send the email.
//
// This component is optional - any AI can generate GLAWMAIL messages directly.
//
// First run:
//
//	go run ./setup --machine a
//	go run ./cmd/machine_a
package main

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/SlaviXG/glawmail/internal/config"
	"github.com/SlaviXG/glawmail/internal/telegram"
)

var (
	cfg         *config.MachineAConfig
	bot         *telegram.Bot
	logger      = log.New(os.Stdout, "", log.LstdFlags)
	ownerChatID int64
)

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

	bot.SendMessage(ownerChatID, "GlawMail preview bot started.\n\nUse 'go run ./cmd/machine_a test' to send a test email.")
	logger.Println("Waiting for AI to generate emails...")

	select {}
}

// formatEmail creates a GLAWMAIL message in the human-readable format.
func formatEmail(to, subject, body string) string {
	return fmt.Sprintf("GLAWMAIL\nTo: %s\nSubject: %s\nBody:\n%s", to, subject, body)
}

// showEmailPreview shows the email in GLAWMAIL format for forwarding.
func showEmailPreview(to, subject, body string) {
	msg := formatEmail(to, subject, body)
	bot.SendMessage(ownerChatID, msg)
	logger.Printf("Email preview shown: to=%s subject=%s", to, subject)
}

func runTest() {
	logger.Println("Sending test email preview...")

	showEmailPreview(
		"james@spotship.com",
		"Quick question about Spot Ship",
		"Hi James,\n\nI saw you're building Spot Ship - congrats on the recent raise!\n\nCurious: how much time do you spend manually reconciling Stripe payouts?\n\nHappy to chat for 20 min.\n\nSlava",
	)

	logger.Println("Test preview sent. Forward the message to the Gmail bot.")
}
