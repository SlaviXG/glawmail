// Package pairing implements the Glawmail bot pairing protocol.
//
// The pairing handshake locks each bot to its expected counterpart
// before any operational messages are processed.
//
// How it works:
//
//	Machine A (ai_app / @openclawbot):
//	  1. Sends GLAWMAIL_PAIR_REQUEST:<code> to Machine B's bot
//	  2. Waits for GLAWMAIL_PAIR_CONFIRM:<code> back
//	  3. Locks the peer bot ID and writes it to .pairing
//
//	Machine B (approval_bot / @approvalbot):
//	  1. Waits for GLAWMAIL_PAIR_REQUEST:<code> from Machine A
//	  2. Displays code to owner for manual confirmation
//	  3. Owner types /confirm
//	  4. Sends GLAWMAIL_PAIR_CONFIRM:<code> to Machine A
//	  5. Locks the peer bot ID and writes it to .pairing
//
// After pairing, any message from a sender other than the locked peer is dropped.
package pairing

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// PairCodeTTL is how long a pairing code is valid.
	PairCodeTTL = 5 * time.Minute

	// PairingFile is the default path to the pairing state file.
	PairingFile = ".pairing"

	// PairRequestPrefix is the prefix for pairing request messages.
	PairRequestPrefix = "GLAWMAIL_PAIR_REQUEST"

	// PairConfirmPrefix is the prefix for pairing confirmation messages.
	PairConfirmPrefix = "GLAWMAIL_PAIR_CONFIRM"
)

// PairingData holds the persisted pairing state.
type PairingData struct {
	PeerBotID string  `json:"peer_bot_id"`
	PairedAt  float64 `json:"paired_at"`
}

// Load reads the pairing state from disk.
func Load(path string) (*PairingData, error) {
	if path == "" {
		path = PairingFile
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var p PairingData
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// Save writes the pairing state to disk with mode 0600.
func Save(path string, data *PairingData) error {
	if path == "" {
		path = PairingFile
	}
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0600)
}

// IsPaired returns true if pairing has been completed.
func IsPaired(path string) bool {
	p, err := Load(path)
	if err != nil || p == nil {
		return false
	}
	return p.PeerBotID != "" && p.PairedAt > 0
}

// GetPeerBotID returns the locked peer bot ID, or empty string if not paired.
func GetPeerBotID(path string) string {
	p, err := Load(path)
	if err != nil || p == nil {
		return ""
	}
	return p.PeerBotID
}

// RecordPairing saves the peer bot ID to the pairing file.
func RecordPairing(path, peerBotID string) error {
	return Save(path, &PairingData{
		PeerBotID: peerBotID,
		PairedAt:  float64(time.Now().Unix()),
	})
}

// GeneratePairCode generates a 6-digit zero-padded numeric code.
func GeneratePairCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// MakePairRequest creates a GLAWMAIL_PAIR_REQUEST message.
func MakePairRequest(code string) string {
	return PairRequestPrefix + ":" + code
}

// MakePairConfirm creates a GLAWMAIL_PAIR_CONFIRM message.
func MakePairConfirm(code string) string {
	return PairConfirmPrefix + ":" + code
}

// ParsePairMessage parses a pairing message and returns (prefix, code, ok).
func ParsePairMessage(text string) (prefix, code string, ok bool) {
	for _, p := range []string{PairRequestPrefix, PairConfirmPrefix} {
		if strings.HasPrefix(text, p+":") {
			code = strings.TrimPrefix(text, p+":")
			if len(code) == 6 && isDigits(code) {
				return p, code, true
			}
		}
	}
	return "", "", false
}

// IsTrustedSender returns true if the sender matches the locked peer bot ID.
func IsTrustedSender(senderID, path string) bool {
	peer := GetPeerBotID(path)
	if peer == "" {
		return false
	}
	return senderID == peer
}

func isDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// Int64ToStr converts an int64 to string.
func Int64ToStr(n int64) string {
	return strconv.FormatInt(n, 10)
}
