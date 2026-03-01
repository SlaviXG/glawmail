// cmd/setup - GlawMail first-run setup wizard
//
// Run this once on each machine before starting the main bot.
//
//	glawmail-setup --machine a    - configure Machine A (OpenClaw AI bot)
//	glawmail-setup --machine b    - configure Machine B (Approval bot + Gmail)
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strings"

	"github.com/SlaviXG/glawmail/internal/color"
	"github.com/SlaviXG/glawmail/internal/config"
	"github.com/SlaviXG/glawmail/internal/gmail"
)

func pathExample(filename string) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("e.g. %s or C:\\Users\\You\\%s", filename, filename)
	}
	return fmt.Sprintf("e.g. %s or /home/you/%s", filename, filename)
}

// -- Input helpers ------------------------------------------------------------

var reader = bufio.NewReader(os.Stdin)

func prompt(label, defaultVal string, secret bool) string {
	for {
		if defaultVal != "" {
			fmt.Printf("%s [%s]: ", color.Bold(label), defaultVal)
		} else {
			fmt.Printf("%s: ", color.Bold(label))
		}
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			line = defaultVal
		}
		if line != "" {
			return line
		}
		color.Warn("This field is required.")
	}
}

func promptYN(label string, defaultYes bool) bool {
	hint := "[Y/n]"
	if !defaultYes {
		hint = "[y/N]"
	}
	fmt.Printf("%s %s: ", color.Bold(label), hint)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	if line == "" {
		return defaultYes
	}
	return line == "y" || line == "yes"
}

// -- Validators ---------------------------------------------------------------

var (
	botTokenRe = regexp.MustCompile(`^\d+:[A-Za-z0-9_-]{35,}$`)
	chatIDRe   = regexp.MustCompile(`^-?\d+$`)
	emailRe    = regexp.MustCompile(`^[^@]+@[^@]+\.[^@]+$`)
)

// -- Telegram API helpers -----------------------------------------------------

func tgGetMe(token string) (username string, id int64, err error) {
	resp, err := http.Get(fmt.Sprintf("https://api.telegram.org/bot%s/getMe", token))
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			Username string `json:"username"`
			ID       int64  `json:"id"`
		} `json:"result"`
		Description string `json:"description"`
	}
	json.Unmarshal(body, &result)
	if !result.OK {
		return "", 0, fmt.Errorf("%s", result.Description)
	}
	return result.Result.Username, result.Result.ID, nil
}

func tgSendMessage(token, chatID, text string) error {
	body := fmt.Sprintf(`{"chat_id":%s,"text":%q}`, chatID, text)
	resp, err := http.Post(
		fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token),
		"application/json",
		strings.NewReader(body),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	raw, _ := io.ReadAll(resp.Body)
	json.Unmarshal(raw, &result)
	if !result.OK {
		return fmt.Errorf("%s", result.Description)
	}
	return nil
}

// -- Machine A setup ----------------------------------------------------------

func setupMachineA() {
	color.Heading("Machine A - Email Preview Bot Setup (Optional)")
	fmt.Println("Machine A generates email previews in GLAWMAIL format.")
	fmt.Println("You can skip this if your AI generates GLAWMAIL messages directly.")
	fmt.Println()

	existing := loadExisting()

	// Own bot token
	color.Heading("Step 1 / 2 - Bot Token")
	color.Info("Create this bot via @BotFather (/newbot) if you haven't already.")
	var ownToken, botUsername string
	for {
		ownToken = prompt("Bot token", existing["OWN_BOT_TOKEN"], true)
		if !botTokenRe.MatchString(ownToken) {
			color.Warn("Expected format: 123456789:ABCdef...")
			continue
		}
		color.Info("Verifying with Telegram API...")
		username, _, err := tgGetMe(ownToken)
		if err != nil {
			color.Err(fmt.Sprintf("Token rejected: %v", err))
			continue
		}
		botUsername = username
		color.Ok(fmt.Sprintf("Token valid - bot: @%s", botUsername))
		break
	}

	// Owner chat ID
	color.Heading("Step 2 / 2 - Your Telegram Chat ID")
	color.Info("Get your chat ID from @userinfobot on Telegram.")
	var ownerChatID string
	for {
		ownerChatID = prompt("Your Telegram chat ID", existing["OWNER_CHAT_ID"], false)
		if !chatIDRe.MatchString(ownerChatID) {
			color.Warn("Must be a number, e.g. 987654321")
			continue
		}
		color.Info("Sending test message...")
		if err := tgSendMessage(ownToken, ownerChatID, "GlawMail setup: Preview bot connected!"); err != nil {
			color.Err(fmt.Sprintf("Could not send: %v", err))
			color.Warn("Make sure you have sent /start to @" + botUsername + " first.")
			continue
		}
		color.Ok("Test message sent - check your Telegram!")
		break
	}

	values := map[string]string{
		"OWN_BOT_TOKEN": ownToken,
		"OWNER_CHAT_ID": ownerChatID,
	}
	if err := config.Write(".env", values); err != nil {
		color.Err(fmt.Sprintf("Could not write .env: %v", err))
		os.Exit(1)
	}
	fmt.Println()
	color.Ok("Setup complete. Start with:")
	fmt.Printf("  %s\n", color.Bold("go run ./cmd/machine_a"))
}

// -- Machine B setup ----------------------------------------------------------

func setupMachineB() {
	color.Heading("Machine B - Gmail Sender Bot Setup")
	fmt.Println("Machine B sends emails via Gmail when you forward GLAWMAIL_SEND messages.")
	fmt.Println()

	existing := loadExisting()

	// Own bot token
	color.Heading("Step 1 / 3 - Gmail Sender Bot Token")
	color.Info("Create a bot via @BotFather (/newbot).")
	var ownToken string
	for {
		ownToken = prompt("Approval bot token", existing["OWN_BOT_TOKEN"], true)
		if !botTokenRe.MatchString(ownToken) {
			color.Warn("Expected format: 123456789:ABCdef...")
			continue
		}
		color.Info("Verifying...")
		username, _, err := tgGetMe(ownToken)
		if err != nil {
			color.Err(fmt.Sprintf("Token rejected: %v", err))
			continue
		}
		color.Ok(fmt.Sprintf("Token valid - bot: @%s", username))
		break
	}

	// Owner chat ID
	color.Heading("Step 2 / 3 - Your Telegram Chat ID")
	color.Info("Get your chat ID from @userinfobot.")
	var ownerChatID string
	for {
		ownerChatID = prompt("Your Telegram chat ID", existing["OWNER_CHAT_ID"], false)
		if !chatIDRe.MatchString(ownerChatID) {
			color.Warn("Must be a number.")
			continue
		}
		color.Info("Sending test message...")
		if err := tgSendMessage(ownToken, ownerChatID, "GlawMail setup: Gmail sender bot connected!"); err != nil {
			color.Err(fmt.Sprintf("Could not send: %v", err))
			color.Warn("Make sure you have sent /start to the bot first.")
			continue
		}
		color.Ok("Test message sent - check your Telegram!")
		break
	}

	// Gmail
	color.Heading("Step 3 / 3 - Gmail Account + OAuth")
	var gmailFrom string
	for {
		gmailFrom = prompt("Gmail address to send from", existing["GMAIL_FROM"], false)
		if !emailRe.MatchString(gmailFrom) {
			color.Warn("Does not look like a valid email.")
			continue
		}
		break
	}
	color.Info(pathExample("credentials.json"))
	credPath := prompt("Path to credentials.json", func() string {
		if v := existing["GMAIL_CREDENTIALS_FILE"]; v != "" {
			return v
		}
		return "credentials.json"
	}(), false)
	color.Info(pathExample("token.json"))
	tokenPath := prompt("Path to store Gmail token", func() string {
		if v := existing["GMAIL_TOKEN_FILE"]; v != "" {
			return v
		}
		return "token.json"
	}(), false)

	_, tokenExists := os.Stat(tokenPath)
	if os.IsNotExist(tokenExists) || promptYN("Re-run Gmail OAuth flow?", false) {
		if _, err := os.Stat(credPath); os.IsNotExist(err) {
			color.Err(fmt.Sprintf("%s not found. Download it from Google Cloud Console first.", credPath))
			os.Exit(1)
		}
		color.Info("If you haven't set up Google Cloud yet:")
		fmt.Println("  1. https://console.cloud.google.com/ - your project")
		fmt.Println("  2. APIs and Services - Enable Gmail API")
		fmt.Println("  3. OAuth consent screen - add your Gmail as a test user")
		fmt.Println("  4. Credentials - Create OAuth 2.0 Client ID")
		fmt.Println("     IMPORTANT: Select 'Desktop app' (NOT 'Web application')")
		fmt.Println("  5. Download JSON - that's your credentials.json")
		fmt.Println()
		if err := gmail.RunOAuthFlow(credPath, tokenPath); err != nil {
			color.Err(fmt.Sprintf("OAuth failed: %v", err))
			os.Exit(1)
		}
		color.Ok(fmt.Sprintf("Gmail token saved to %s", tokenPath))
	}

	values := map[string]string{
		"OWN_BOT_TOKEN":          ownToken,
		"OWNER_CHAT_ID":          ownerChatID,
		"GMAIL_FROM":             gmailFrom,
		"GMAIL_CREDENTIALS_FILE": credPath,
		"GMAIL_TOKEN_FILE":       tokenPath,
	}
	if err := config.Write(".env", values); err != nil {
		color.Err(fmt.Sprintf("Could not write .env: %v", err))
		os.Exit(1)
	}
	fmt.Println()
	color.Ok("Setup complete. Start with:")
	fmt.Printf("  %s\n", color.Bold("go run ./cmd/machine_b"))
}

// -- Helpers ------------------------------------------------------------------

func loadExisting() map[string]string {
	existing := map[string]string{}
	f, err := os.Open(".env")
	if err != nil {
		return existing
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if ok {
			existing[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return existing
}

// -- Entry point --------------------------------------------------------------

func main() {
	machine := flag.String("machine", "", "Which machine to configure: 'a' or 'b'")
	flag.Parse()

	fmt.Printf("\n%s\n", color.Bold("================================================"))
	fmt.Printf("%s\n", color.Bold("  GlawMail - First-Run Setup Wizard"))
	fmt.Printf("%s\n", color.Bold("================================================"))
	fmt.Println("Secrets are written to .env (mode 0600) and never")
	fmt.Println("stored in source code or shown after confirmation.")
	fmt.Println()

	switch strings.ToLower(*machine) {
	case "a":
		setupMachineA()
	case "b":
		setupMachineB()
	default:
		fmt.Fprintln(os.Stderr, "Usage: glawmail-setup --machine <a|b>")
		os.Exit(1)
	}
}
