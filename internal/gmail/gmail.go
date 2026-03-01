// Package gmail provides Gmail OAuth2 and email sending functionality.
// Uses raw HTTP requests instead of the Google API client library for compatibility.
package gmail

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const gmailSendURL = "https://gmail.googleapis.com/gmail/v1/users/me/messages/send"

var scopes = []string{"https://www.googleapis.com/auth/gmail.send"}

// Service wraps the Gmail API service.
type Service struct {
	client   *http.Client
	fromAddr string
}

// NewService creates a Gmail service from credentials and token files.
func NewService(credentialsFile, tokenFile, fromAddr string) (*Service, error) {
	// Load OAuth config from credentials file (needed for token refresh)
	credData, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("reading credentials file: %w", err)
	}
	config, err := google.ConfigFromJSON(credData, scopes...)
	if err != nil {
		return nil, fmt.Errorf("parsing credentials: %w", err)
	}

	// Load existing token
	tokenData, err := os.ReadFile(tokenFile)
	if err != nil {
		return nil, fmt.Errorf("reading token file: %w", err)
	}

	var token oauth2.Token
	if err := json.Unmarshal(tokenData, &token); err != nil {
		return nil, fmt.Errorf("parsing token: %w", err)
	}

	// Create a token source that can refresh
	tokenSource := config.TokenSource(context.Background(), &token)

	// Get a fresh token (this will refresh if needed)
	newToken, err := tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("refreshing token: %w", err)
	}

	// Save refreshed token if it changed
	if newToken.AccessToken != token.AccessToken {
		tokenJSON, _ := json.Marshal(newToken)
		if err := os.WriteFile(tokenFile, tokenJSON, 0600); err != nil {
			fmt.Printf("Warning: could not save refreshed token: %v\n", err)
		}
	}

	client := oauth2.NewClient(context.Background(), tokenSource)

	return &Service{client: client, fromAddr: fromAddr}, nil
}

// SendEmail sends an email and returns the Gmail message ID.
func (s *Service) SendEmail(to, subject, body string, html bool) (string, error) {
	contentType := "text/plain"
	if html {
		contentType = "text/html"
	}

	// Build RFC 2822 message
	var msg strings.Builder
	msg.WriteString("From: " + s.fromAddr + "\r\n")
	msg.WriteString("To: " + to + "\r\n")
	msg.WriteString("Subject: " + subject + "\r\n")
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: " + contentType + "; charset=utf-8\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body)

	raw := base64.URLEncoding.EncodeToString([]byte(msg.String()))

	reqBody := map[string]string{"raw": raw}
	reqJSON, _ := json.Marshal(reqBody)

	req, err := http.NewRequest("POST", gmailSendURL, bytes.NewReader(reqJSON))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("gmail API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}

	return result.ID, nil
}

// RunOAuthFlow performs the OAuth2 authorization flow and saves the token.
func RunOAuthFlow(credentialsFile, tokenFile string) error {
	credData, err := os.ReadFile(credentialsFile)
	if err != nil {
		return fmt.Errorf("reading credentials: %w", err)
	}

	config, err := google.ConfigFromJSON(credData, scopes...)
	if err != nil {
		return fmt.Errorf("parsing credentials (make sure you created a 'Desktop app' OAuth client, not 'Web application'): %w", err)
	}

	// Find an available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("finding port: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	config.RedirectURL = fmt.Sprintf("http://127.0.0.1:%d", port)

	// Channel to receive the auth code
	codeChan := make(chan string, 1)
	errChan := make(chan error, 1)

	// Start local server to receive callback
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			errChan <- fmt.Errorf("no code in callback")
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, "<h1>Authorization successful!</h1><p>You can close this window.</p>")
		codeChan <- code
	})

	server := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", port),
		Handler: mux,
	}

	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// Generate auth URL
	authURL := config.AuthCodeURL("state", oauth2.AccessTypeOffline)
	fmt.Printf("\nOpen this URL in your browser:\n\n%s\n\n", authURL)

	// Wait for code with timeout
	var code string
	select {
	case code = <-codeChan:
	case err := <-errChan:
		server.Close()
		return err
	case <-time.After(5 * time.Minute):
		server.Close()
		return fmt.Errorf("timeout waiting for authorization")
	}

	server.Close()

	// Exchange code for token
	token, err := config.Exchange(context.Background(), code)
	if err != nil {
		return fmt.Errorf("exchanging code: %w", err)
	}

	// Save token
	tokenJSON, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling token: %w", err)
	}
	if err := os.WriteFile(tokenFile, tokenJSON, 0600); err != nil {
		return fmt.Errorf("saving token: %w", err)
	}

	fmt.Printf("Token saved to %s\n", tokenFile)
	return nil
}
