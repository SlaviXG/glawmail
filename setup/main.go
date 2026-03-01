// Interactive setup wizard for Glawmail.
//
// Usage:
//
//	go run ./setup --machine a    # Configure Machine A (OpenClaw AI Bot)
//	go run ./setup --machine b    # Configure Machine B (Approval Bot + Gmail)
package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/SlaviXG/glawmail/internal/config"
	"github.com/SlaviXG/glawmail/internal/gmail"
	"github.com/SlaviXG/glawmail/internal/telegram"
)

// ANSI color codes
const (
	Reset  = "\033[0m"
	Bold   = "\033[1m"
	Green  = "\033[32m"
	Yellow = "\033[33m"
	Red    = "\033[31m"
	Cyan   = "\033[36m"
)

var reader = bufio.NewReader(os.Stdin)

func info(msg string)    { fmt.Printf("%si  %s%s\n", Cyan, msg, Reset) }
func ok(msg string)      { fmt.Printf("%s✔  %s%s\n", Green, msg, Reset) }
func warn(msg string)    { fmt.Printf("%s⚠  %s%s\n", Yellow, msg, Reset) }
func errMsg(msg string)  { fmt.Printf("%s✖  %s%s\n", Red, msg, Reset) }
func heading(msg string) { fmt.Printf("\n%s%s%s\n%s\n", Bold, msg, Reset, strings.Repeat("─", len(msg))) }

func prompt(label, defaultVal string, validator func(string) string) string {
	for {
		display := Bold + label + Reset
		if defaultVal != "" {
			display += fmt.Sprintf(" [%s]", defaultVal)
		}
		display += ": "
		fmt.Print(display)

		input, _ := reader.ReadString('\n')
		value := strings.TrimSpace(input)
		if value == "" {
			value = defaultVal
		}
		if value == "" {
			warn("This field is required.")
			continue
		}
		if validator != nil {
			if err := validator(value); err != "" {
				warn(err)
				continue
			}
		}
		return value
	}
}

func promptYesNo(label string, defaultYes bool) bool {
	hint := "[Y/n]"
	if !defaultYes {
		hint = "[y/N]"
	}
	fmt.Printf("%s%s%s %s: ", Bold, label, Reset, hint)
	input, _ := reader.ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(input))
	if answer == "" {
		return defaultYes
	}
	return answer == "y" || answer == "yes"
}

func validateBotToken(t string) string {
	match, _ := regexp.MatchString(`^\d+:[A-Za-z0-9_-]{35,}$`, t)
	if !match {
		return "Expected format: 123456789:ABCdef..."
	}
	return ""
}

func validateChatID(c string) string {
	match, _ := regexp.MatchString(`^-?\d+$`, c)
	if !match {
		return "Chat ID must be numeric (e.g. 987654321)"
	}
	return ""
}

func validateEmail(e string) string {
	if !strings.Contains(e, "@") || !strings.Contains(strings.Split(e, "@")[1], ".") {
		return "Doesn't look like a valid email address"
	}
	return ""
}

func generateSecret() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func setupMachineA() {
	heading("Machine A - OpenClaw AI Bot Setup")

	existing, _ := config.LoadEnv(".env")

	fmt.Println("Machine A runs your OpenClaw AI bot. It generates emails and sends")
	fmt.Println("approval requests to Machine B via Telegram. It listens for decline")
	fmt.Println("callbacks on its own bot - no public endpoint needed.")
	fmt.Println()

	// Step 1: Own bot token
	heading("Step 1 / 4 - OpenClaw Bot Token")
	info("This is the token for YOUR AI bot (@openclawbot or similar).")
	info("Create it via @BotFather if you haven't already.")

	var ownToken string
	for {
		ownToken = prompt("OpenClaw bot token", existing["OWN_BOT_TOKEN"], validateBotToken)
		info("Verifying...")
		bot := telegram.NewBot(ownToken)
		me, err := bot.GetMe()
		if err != nil {
			errMsg(fmt.Sprintf("Rejected: %v", err))
			if !promptYesNo("Try again?", true) {
				os.Exit(1)
			}
			continue
		}
		ok(fmt.Sprintf("Token valid - bot: @%s", me.Username))
		break
	}

	// Step 2: Owner chat ID
	heading("Step 2 / 4 - Your Telegram Chat ID")
	info("Machine A needs your chat ID to forward any status messages to you.")
	info("Get it from @userinfobot.")

	var ownerChatID string
	bot := telegram.NewBot(ownToken)
	for {
		ownerChatID = prompt("Your Telegram chat ID", existing["OWNER_CHAT_ID"], validateChatID)
		info("Sending test message via OpenClaw bot...")
		chatID := parseInt64(ownerChatID)
		_, err := bot.SendMessage(chatID, "✅ OpenClaw bot connected successfully!")
		if err != nil {
			errMsg(fmt.Sprintf("Failed: %v", err))
			warn("Make sure you've sent /start to your OpenClaw bot first.")
			if !promptYesNo("Try again?", true) {
				os.Exit(1)
			}
			continue
		}
		ok("Test message sent!")
		break
	}

	// Step 3: Approval bot chat ID
	heading("Step 3 / 4 - Approval Bot Chat ID")
	info("Machine A sends approval requests TO Machine B's bot.")
	info("You need the numeric user ID of Machine B's bot (@approvalbot).")
	fmt.Println("Two ways to get it:")
	fmt.Println("  a) Run setup --machine b first - it will print the bot ID")
	fmt.Println("  b) Forward a message from @approvalbot to @userinfobot")
	fmt.Println()

	approvalBotChatID := prompt("Approval bot's Telegram user ID", existing["APPROVAL_BOT_CHAT_ID"], validateChatID)

	// Step 4: Shared secret
	heading("Step 4 / 4 - Shared HMAC Secret")
	existingWS := existing["WEBHOOK_SECRET"]
	var webhookSecret string
	if existingWS != "" {
		info("Existing WEBHOOK_SECRET found.")
		if promptYesNo("Keep it?", true) {
			webhookSecret = existingWS
		} else {
			webhookSecret = generateSecret()
		}
	} else {
		webhookSecret = generateSecret()
	}

	ok("WEBHOOK_SECRET:")
	fmt.Printf("\n  %s%s%s\n\n", Bold, webhookSecret, Reset)
	warn("Copy this exact value into Machine B's .env WEBHOOK_SECRET.")
	fmt.Printf("%sPress Enter once you've noted it...%s ", Bold, Reset)
	reader.ReadString('\n')

	err := config.WriteEnv(".env", map[string]string{
		"OWN_BOT_TOKEN":        ownToken,
		"OWNER_CHAT_ID":        ownerChatID,
		"APPROVAL_BOT_CHAT_ID": approvalBotChatID,
		"WEBHOOK_SECRET":       webhookSecret,
	})
	if err != nil {
		errMsg(fmt.Sprintf("Failed to write .env: %v", err))
		os.Exit(1)
	}
	ok("Written .env (mode 600)")

	fmt.Println()
	ok("Machine A setup complete! Start with:")
	fmt.Printf("  %sgo run ./cmd/machine_a%s\n", Bold, Reset)
}

func setupMachineB() {
	heading("Machine B - Approval Bot + Gmail Sender Setup")

	existing, _ := config.LoadEnv(".env")

	fmt.Println("Machine B runs the Telegram approval bot and Gmail sender.")
	fmt.Println("It never shares credentials or network access with Machine A.")
	fmt.Println("Telegram is the only relay between the two machines.")
	fmt.Println()

	// Step 1: Approval bot token
	heading("Step 1 / 5 - Approval Bot Token")
	info("This is a SEPARATE bot from the OpenClaw bot on Machine A.")
	info("Create a new bot via @BotFather (/newbot).")

	var ownToken string
	var botID string
	for {
		ownToken = prompt("Approval bot token", existing["OWN_BOT_TOKEN"], validateBotToken)
		info("Verifying...")
		bot := telegram.NewBot(ownToken)
		me, err := bot.GetMe()
		if err != nil {
			errMsg(fmt.Sprintf("Rejected: %v", err))
			if !promptYesNo("Try again?", true) {
				os.Exit(1)
			}
			continue
		}
		ok(fmt.Sprintf("Token valid - bot: @%s", me.Username))
		botID = fmt.Sprintf("%d", me.ID)
		fmt.Println()
		info(fmt.Sprintf("This bot's numeric ID is: %s%s%s", Bold, botID, Reset))
		warn("You'll need this value for Machine A's APPROVAL_BOT_CHAT_ID.")
		fmt.Printf("%sPress Enter once you've noted the bot ID...%s ", Bold, Reset)
		reader.ReadString('\n')
		break
	}

	// Step 2: Owner chat ID
	heading("Step 2 / 5 - Your Telegram Chat ID")
	info("Machine B sends approval previews to your personal Telegram chat.")
	info("Get your chat ID from @userinfobot.")

	var ownerChatID string
	bot := telegram.NewBot(ownToken)
	for {
		ownerChatID = prompt("Your Telegram chat ID", existing["OWNER_CHAT_ID"], validateChatID)
		info("Sending test message via approval bot...")
		chatID := parseInt64(ownerChatID)
		_, err := bot.SendMessage(chatID, "✅ Approval bot connected successfully!")
		if err != nil {
			errMsg(fmt.Sprintf("Failed: %v", err))
			warn("Make sure you've sent /start to the approval bot first.")
			if !promptYesNo("Try again?", true) {
				os.Exit(1)
			}
			continue
		}
		ok("Test message sent!")
		break
	}

	// Step 3: OpenClaw bot chat ID
	heading("Step 3 / 5 - OpenClaw Bot Chat ID")
	info("When you decline an email, Machine B sends a message TO Machine A's bot.")
	info("You need the numeric user ID of @openclawbot.")
	info("Get it by forwarding a message from @openclawbot to @userinfobot.")

	openclawBotChatID := prompt("OpenClaw bot's Telegram user ID", existing["OPENCLAW_BOT_CHAT_ID"], validateChatID)

	// Step 4: Shared webhook secret
	heading("Step 4 / 5 - Shared HMAC Secret")
	info("Paste the WEBHOOK_SECRET generated on Machine A.")
	webhookSecret := prompt("WEBHOOK_SECRET", existing["WEBHOOK_SECRET"], nil)

	// Step 5: Gmail OAuth
	heading("Step 5 / 5 - Gmail Account + OAuth")
	gmailFrom := prompt("Gmail address to send from", existing["GMAIL_FROM"], validateEmail)
	tokenPath := prompt("Path to store Gmail OAuth token", orDefault(existing["GMAIL_TOKEN_FILE"], "token.json"), nil)

	if fileExists(tokenPath) {
		info(fmt.Sprintf("%s already exists.", tokenPath))
		if promptYesNo("Re-run Gmail OAuth flow?", false) {
			runGmailOAuth(tokenPath)
		}
	} else {
		runGmailOAuth(tokenPath)
	}

	err := config.WriteEnv(".env", map[string]string{
		"OWN_BOT_TOKEN":        ownToken,
		"OWNER_CHAT_ID":        ownerChatID,
		"OPENCLAW_BOT_CHAT_ID": openclawBotChatID,
		"WEBHOOK_SECRET":       webhookSecret,
		"GMAIL_FROM":           gmailFrom,
		"GMAIL_TOKEN_FILE":     tokenPath,
	})
	if err != nil {
		errMsg(fmt.Sprintf("Failed to write .env: %v", err))
		os.Exit(1)
	}
	ok("Written .env (mode 600)")

	fmt.Println()
	ok("Machine B setup complete! Start with:")
	fmt.Printf("  %sgo run ./cmd/machine_b%s\n", Bold, Reset)
}

func runGmailOAuth(tokenPath string) {
	fmt.Println()
	info("You need credentials.json from Google Cloud Console:")
	fmt.Println("  1. https://console.cloud.google.com/ → your project")
	fmt.Println("  2. APIs & Services → Enable Gmail API")
	fmt.Println("  3. OAuth consent screen → add your Gmail as a test user")
	fmt.Println("  4. Credentials → Create OAuth 2.0 Client ID (Desktop app)")
	fmt.Println("  5. Download JSON → save as credentials.json here")
	fmt.Println()

	credsPath := prompt("Path to credentials.json", "credentials.json", nil)
	if !fileExists(credsPath) {
		errMsg(fmt.Sprintf("%s not found. Download it first.", credsPath))
		os.Exit(1)
	}

	info("Opening browser for Gmail authorization...")
	if err := gmail.RunOAuthFlow(credsPath, tokenPath); err != nil {
		errMsg(fmt.Sprintf("OAuth failed: %v", err))
		os.Exit(1)
	}
	ok(fmt.Sprintf("Gmail token saved to %s (mode 600)", tokenPath))
}

func parseInt64(s string) int64 {
	var n int64
	fmt.Sscanf(s, "%d", &n)
	return n
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func orDefault(val, def string) string {
	if val != "" {
		return val
	}
	return def
}

func main() {
	machine := flag.String("machine", "", "'a' = OpenClaw AI Bot  |  'b' = Approval Bot + Gmail")
	flag.Parse()

	if *machine != "a" && *machine != "b" {
		fmt.Println("Usage: go run ./setup --machine a|b")
		fmt.Println("  --machine a    Configure Machine A (OpenClaw AI Bot)")
		fmt.Println("  --machine b    Configure Machine B (Approval Bot + Gmail)")
		os.Exit(1)
	}

	fmt.Printf("\n%s%s\n", Bold, strings.Repeat("=", 55))
	fmt.Println("  Glawmail - First-Run Setup Wizard")
	fmt.Printf("%s%s\n", strings.Repeat("=", 55), Reset)
	fmt.Println("Secrets are written to .env (mode 600) and never")
	fmt.Println("stored in source code or shown after confirmation.")
	fmt.Println()

	if *machine == "a" {
		setupMachineA()
	} else {
		setupMachineB()
	}
}
