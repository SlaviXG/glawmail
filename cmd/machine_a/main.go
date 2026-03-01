// Machine A - OpenClaw AI Bot Bridge
//
// Generates emails, sends approval requests to Machine B's bot via Telegram,
// and listens for status callbacks on its own bot's incoming messages.
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
	"strings"
	"time"

	"github.com/SlaviXG/glawmail/internal/config"
	glawhmac "github.com/SlaviXG/glawmail/internal/hmac"
	"github.com/SlaviXG/glawmail/internal/pairing"
	"github.com/SlaviXG/glawmail/internal/telegram"
	"github.com/google/uuid"
)

var (
	cfg    *config.MachineAConfig
	bot    *telegram.Bot
	logger = log.New(os.Stdout, "", log.LstdFlags)
)

// ApprovalRequest represents an email approval request payload.
type ApprovalRequest struct {
	CallbackID string            `json:"callback_id"`
	To         string            `json:"to"`
	Subject    string            `json:"subject"`
	Body       string            `json:"body"`
	HTML       bool              `json:"html"`
	Metadata   map[string]string `json:"metadata"`
}

// StatusCallback represents a callback from Machine B.
type StatusCallback struct {
	CallbackID string            `json:"callback_id"`
	To         string            `json:"to"`
	Subject    string            `json:"subject"`
	GmailID    string            `json:"gmail_id,omitempty"`
	Stage      string            `json:"stage,omitempty"`
	Reason     string            `json:"reason,omitempty"`
	Metadata   map[string]string `json:"metadata"`
}

func main() {
	var err error
	cfg, err = config.LoadMachineAConfig(".env")
	if err != nil {
		fmt.Println("ERROR:", err)
		fmt.Println("Run: go run ./setup --machine a")
		os.Exit(1)
	}

	bot = telegram.NewBot(cfg.OwnBotToken)

	me, err := bot.GetMe()
	if err != nil {
		log.Fatalf("Failed to verify bot token: %v", err)
	}
	logger.Printf("Bot verified: @%s (ID: %d)", me.Username, me.ID)

	if !pairing.IsPaired("") {
		logger.Println("Not yet paired - starting pairing handshake...")
		if !runPairing() {
			logger.Println("Pairing failed. Exiting.")
			os.Exit(1)
		}
	}

	logger.Println("Glawmail Machine A ready.")

	if len(os.Args) > 1 && os.Args[1] == "test" {
		runTest()
		return
	}

	pollUpdates()
}

func runPairing() bool {
	code, err := pairing.GeneratePairCode()
	if err != nil {
		logger.Printf("Failed to generate pair code: %v", err)
		return false
	}

	deadline := time.Now().Add(pairing.PairCodeTTL)
	peerChatID, _ := strconv.ParseInt(cfg.ApprovalBotChatID, 10, 64)
	ownerChatID, _ := strconv.ParseInt(cfg.OwnerChatID, 10, 64)

	logger.Println("Sending GLAWMAIL_PAIR_REQUEST to approval bot...")
	_, err = bot.SendMessage(peerChatID, pairing.MakePairRequest(code))
	if err != nil {
		logger.Printf("Failed to send pair request: %v", err)
		return false
	}

	bot.SendMessage(ownerChatID, fmt.Sprintf(
		"Glawmail pairing started.\n\n"+
			"The approval bot will show you a code and ask you to "+
			"type /confirm to complete pairing.\n\n"+
			"This request expires in %d minutes.",
		int(pairing.PairCodeTTL.Minutes()),
	))

	var offset int64
	logger.Println("Waiting for GLAWMAIL_PAIR_CONFIRM...")

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
			text := update.Message.Text

			prefix, recvCode, ok := pairing.ParsePairMessage(text)
			if !ok || prefix != pairing.PairConfirmPrefix {
				continue
			}
			if senderID != cfg.ApprovalBotChatID {
				logger.Printf("Pairing confirm from unexpected sender %s - ignoring", senderID)
				continue
			}
			if recvCode != code {
				logger.Println("Pairing confirm code mismatch - ignoring")
				continue
			}

			if err := pairing.RecordPairing("", senderID); err != nil {
				logger.Printf("Failed to record pairing: %v", err)
				return false
			}

			bot.SendMessage(ownerChatID, "Glawmail pairing complete. Machine A and Machine B are now linked.")
			logger.Printf("Pairing complete - peer locked to bot ID %s", senderID)
			return true
		}
	}

	logger.Println("Pairing timed out.")
	bot.SendMessage(ownerChatID, "Glawmail pairing timed out. Restart both machines to try again.")
	return false
}

func pollUpdates() {
	var offset int64
	logger.Println("Polling for Telegram updates...")

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
			if update.Message == nil || update.Message.From == nil {
				continue
			}

			senderID := strconv.FormatInt(update.Message.From.ID, 10)
			text := update.Message.Text

			if !pairing.IsTrustedSender(senderID, "") {
				if strings.HasPrefix(text, "GLAWMAIL_") {
					logger.Printf("Dropped GLAWMAIL message from untrusted sender %s", senderID)
				}
				continue
			}

			if strings.HasPrefix(text, "GLAWMAIL_APPROVED:") ||
				strings.HasPrefix(text, "GLAWMAIL_DECLINE:") ||
				strings.HasPrefix(text, "GLAWMAIL_ERROR:") {
				processStatusMessage(text)
			}
		}
	}
}

func processStatusMessage(text string) {
	parts := strings.SplitN(text, ":", 3)
	if len(parts) != 3 {
		return
	}
	prefix, sig, payloadStr := parts[0], parts[1], parts[2]

	if !glawhmac.Verify(cfg.WebhookSecret, payloadStr, sig) {
		logger.Printf("%s message failed HMAC verification - ignoring", prefix)
		return
	}

	var data StatusCallback
	if err := json.Unmarshal([]byte(payloadStr), &data); err != nil {
		logger.Printf("Failed to parse status message: %v", err)
		return
	}

	switch prefix {
	case "GLAWMAIL_APPROVED":
		logger.Printf("APPROVED - id=%s | to=%s | subject=%s | gmail_id=%s",
			data.CallbackID, data.To, data.Subject, data.GmailID)
	case "GLAWMAIL_DECLINE":
		logger.Printf("DECLINED - id=%s | to=%s | subject=%s",
			data.CallbackID, data.To, data.Subject)
	case "GLAWMAIL_ERROR":
		logger.Printf("ERROR - id=%s | to=%s | stage=%s | reason=%s",
			data.CallbackID, data.To, data.Stage, data.Reason)
	}
}

func requestEmailApproval(to, subject, body string, html bool, metadata map[string]string) (string, error) {
	if !pairing.IsPaired("") {
		return "", fmt.Errorf("glawmail is not paired")
	}

	callbackID := uuid.New().String()
	if metadata == nil {
		metadata = make(map[string]string)
	}

	payload := ApprovalRequest{
		CallbackID: callbackID,
		To:         to,
		Subject:    subject,
		Body:       body,
		HTML:       html,
		Metadata:   metadata,
	}
	payloadBytes, _ := json.Marshal(payload)
	payloadStr := string(payloadBytes)

	sig := glawhmac.Sign(cfg.WebhookSecret, payloadStr)
	message := fmt.Sprintf("GLAWMAIL_APPROVAL_REQUEST:%s:%s", sig, payloadStr)

	peerChatID, _ := strconv.ParseInt(cfg.ApprovalBotChatID, 10, 64)
	_, err := bot.SendMessage(peerChatID, message)
	if err != nil {
		return "", fmt.Errorf("failed to send approval request: %w", err)
	}

	logger.Printf("Approval request sent for %s (callback_id=%s)", to, callbackID)
	return callbackID, nil
}

func runTest() {
	logger.Println("Sending test approval request to Machine B...")

	callbackID, err := requestEmailApproval(
		"james@spotship.com",
		"Quick question about Spot Ship",
		"Hi James,\n\nI saw you're building Spot Ship - congrats on the recent raise!\n\nCurious: how much time do you spend manually reconciling Stripe payouts?\n\nHappy to chat for 20 min.\n\nSlava",
		false,
		map[string]string{"lead_id": "lead_001", "campaign": "outreach_jan_2026"},
	)
	if err != nil {
		logger.Fatalf("Failed to send test request: %v", err)
	}

	logger.Printf("Sent. callback_id=%s", callbackID)
	logger.Println("Polling for callbacks - press Ctrl+C to stop.")
	pollUpdates()
}
