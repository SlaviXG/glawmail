// GlawMail Setup Wizard
//
// Run this once before starting the bot:
//
//	go run ./setup
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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

var reader = bufio.NewReader(os.Stdin)

func prompt(label, defaultVal string) string {
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

var (
	botTokenRe = regexp.MustCompile(`^\d+:[A-Za-z0-9_-]{35,}$`)
	chatIDRe   = regexp.MustCompile(`^-?\d+$`)
	emailRe    = regexp.MustCompile(`^[^@]+@[^@]+\.[^@]+$`)
)

func tgGetMe(token string) (username string, id int64, err error) {
	resp, err := http.Get(fmt.Sprintf("https://api.telegram.org/bot%s/getMe", token))
	if err != nil {
		return "", 0, err
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			Username string `json:"username"`
			ID       int64  `json:"id"`
		} `json:"result"`
		Description string `json:"description"`
	}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return "", 0, fmt.Errorf("failed to unmarshal body into result,%v", err)
	}

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
	defer func() {
		_ = resp.Body.Close()
	}()

	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	raw, _ := io.ReadAll(resp.Body)
	err = json.Unmarshal(raw, &result)
	if err != nil {
		return fmt.Errorf("failed to unmarshal raw into result, %v", err)
	}

	if !result.OK {
		return fmt.Errorf("%s", result.Description)
	}
	return nil
}

func loadExisting() map[string]string {
	existing := map[string]string{}
	f, err := os.Open(".env")
	if err != nil {
		return existing
	}
	defer func() {
		_ = f.Close()
	}()
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

func getWorkingDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return dir
}

func setupGlobalAlias() {
	fmt.Println()
	if !promptYN("Add global 'glawmail' command?", true) {
		return
	}

	workDir := getWorkingDir()

	if runtime.GOOS == "windows" {
		setupWindowsAlias(workDir)
	} else {
		setupUnixAlias(workDir)
	}
}

func getUserPath() string {
	// Query user PATH from registry (not the combined system+user PATH)
	cmd := exec.Command("reg", "query", "HKCU\\Environment", "/v", "Path")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	// Parse output: "    Path    REG_SZ    value"
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "REG_SZ") || strings.Contains(line, "REG_EXPAND_SZ") {
			parts := strings.SplitN(line, "REG_", 2)
			if len(parts) == 2 {
				// Skip "SZ    " or "EXPAND_SZ    "
				val := parts[1]
				idx := strings.Index(val, "    ")
				if idx != -1 {
					return strings.TrimSpace(val[idx+4:])
				}
			}
		}
	}
	return ""
}

func setupWindowsAlias(workDir string) {
	// Create glawmail.cmd in a common location
	userProfile := os.Getenv("USERPROFILE")
	binDir := filepath.Join(userProfile, "bin")

	// Create bin directory if it doesn't exist
	if err := os.MkdirAll(binDir, 0755); err != nil {
		color.Err(fmt.Sprintf("Could not create %s: %v", binDir, err))
		return
	}

	// Create wrapper script
	cmdPath := filepath.Join(binDir, "glawmail.cmd")
	scriptContent := fmt.Sprintf("@echo off\r\n\"%s\\glawmail.bat\" %%*\r\n", workDir)
	if err := os.WriteFile(cmdPath, []byte(scriptContent), 0755); err != nil {
		color.Err(fmt.Sprintf("Could not create %s: %v", cmdPath, err))
		return
	}

	// Check if bin dir is already in PATH (check full PATH for existing installs)
	fullPath := os.Getenv("PATH")
	if strings.Contains(strings.ToLower(fullPath), strings.ToLower(binDir)) {
		color.Ok("Added 'glawmail' command.")
		return
	}

	// Get user PATH only (not system PATH) to avoid setx 1024 char limit
	userPath := getUserPath()
	var newPath string
	if userPath == "" {
		newPath = binDir
	} else {
		newPath = userPath + ";" + binDir
	}

	// Add to user PATH
	cmd := exec.Command("setx", "PATH", newPath)
	if err := cmd.Run(); err != nil {
		color.Warn(fmt.Sprintf("Created %s but could not add to PATH.", cmdPath))
		color.Info(fmt.Sprintf("Add %s to your PATH manually.", binDir))
		return
	}
	color.Ok("Added 'glawmail' command. Restart your terminal to use it.")
}

func setupUnixAlias(workDir string) {
	home := os.Getenv("HOME")
	scriptPath := filepath.Join(workDir, "glawmail.sh")
	aliasLine := fmt.Sprintf("\nalias glawmail='%s'\n", scriptPath)

	// Detect shell config file
	shell := os.Getenv("SHELL")
	var rcFile string
	if strings.Contains(shell, "zsh") {
		rcFile = filepath.Join(home, ".zshrc")
	} else {
		rcFile = filepath.Join(home, ".bashrc")
	}

	// Check if alias already exists
	content, err := os.ReadFile(rcFile)
	if err != nil {
		color.Err(fmt.Sprintf("Could not read %s: %v", rcFile, err))
		return
	}

	if strings.Contains(string(content), "alias glawmail=") {
		color.Info("Alias already exists in " + rcFile)
		return
	}

	// Append alias
	f, err := os.OpenFile(rcFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		color.Err(fmt.Sprintf("Could not open %s: %v", rcFile, err))
		return
	}
	defer func() {
		_ = f.Close()
	}()

	if _, err := f.WriteString(aliasLine); err != nil {
		color.Err(fmt.Sprintf("Could not write to %s: %v", rcFile, err))
		return
	}

	color.Ok(fmt.Sprintf("Added alias to %s", rcFile))
	color.Info("Run: source " + rcFile)
}

func main() {
	flag.Parse()

	fmt.Printf("\n%s\n", color.Bold("================================"))
	fmt.Printf("%s\n", color.Bold("  GlawMail Setup Wizard"))
	fmt.Printf("%s\n", color.Bold("================================"))
	fmt.Println("Gmail sender bot for Telegram.")
	fmt.Println()

	existing := loadExisting()

	// Bot token
	color.Heading("Step 1 / 3 - Telegram Bot")
	color.Info("Create a bot via @BotFather (/newbot) if you haven't already.")
	var botToken, botUsername string
	for {
		botToken = prompt("Bot token", existing["BOT_TOKEN"])
		if !botTokenRe.MatchString(botToken) {
			color.Warn("Expected format: 123456789:ABCdef...")
			continue
		}
		color.Info("Verifying...")
		username, _, err := tgGetMe(botToken)
		if err != nil {
			color.Err(fmt.Sprintf("Token rejected: %v", err))
			continue
		}
		botUsername = username
		color.Ok(fmt.Sprintf("Token valid - bot: @%s", botUsername))
		break
	}

	// Owner chat ID
	color.Heading("Step 2 / 3 - Your Telegram Chat ID")
	color.Info("Get your chat ID from @userinfobot on Telegram.")
	var ownerChatID string
	for {
		ownerChatID = prompt("Your Telegram chat ID", existing["OWNER_CHAT_ID"])
		if !chatIDRe.MatchString(ownerChatID) {
			color.Warn("Must be a number, e.g. 987654321")
			continue
		}
		color.Info("Sending test message...")
		if err := tgSendMessage(botToken, ownerChatID, "GlawMail setup: Bot connected!"); err != nil {
			color.Err(fmt.Sprintf("Could not send: %v", err))
			color.Warn("Make sure you have sent /start to @" + botUsername + " first.")
			continue
		}
		color.Ok("Test message sent - check your Telegram!")
		break
	}

	// Gmail
	color.Heading("Step 3 / 3 - Gmail")
	var gmailFrom string
	for {
		gmailFrom = prompt("Gmail address to send from", existing["GMAIL_FROM"])
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
	}())
	color.Info(pathExample("token.json"))
	tokenPath := prompt("Path to store Gmail token", func() string {
		if v := existing["GMAIL_TOKEN_FILE"]; v != "" {
			return v
		}
		return "token.json"
	}())

	_, tokenExists := os.Stat(tokenPath)
	if os.IsNotExist(tokenExists) || promptYN("Re-run Gmail OAuth flow?", false) {
		if _, err := os.Stat(credPath); os.IsNotExist(err) {
			color.Err(
				fmt.Sprintf("%s not found. Download it from Google Cloud Console first.", credPath),
			)
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
		"BOT_TOKEN":              botToken,
		"OWNER_CHAT_ID":          ownerChatID,
		"GMAIL_FROM":             gmailFrom,
		"GMAIL_CREDENTIALS_FILE": credPath,
		"GMAIL_TOKEN_FILE":       tokenPath,
	}
	if err := config.Write(".env", values); err != nil {
		color.Err(fmt.Sprintf("Could not write .env: %v", err))
		os.Exit(1)
	}

	// Ask about global alias
	setupGlobalAlias()

	fmt.Println()
	color.Ok("Setup complete!")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  %s\n", color.Bold("glawmail install"))
	fmt.Printf("  %s\n", color.Bold("glawmail up"))
}
