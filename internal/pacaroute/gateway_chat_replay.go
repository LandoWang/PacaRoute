package pacaroute

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/LandoWang/pacaroute/internal/pacaroute/protocol"
	"github.com/LandoWang/pacaroute/internal/pacaroute/routing"
)

const chatReplayPrefix = "pacaroute-chat-v1."
const maxChatReplay = 8 << 20

// The client stores the capsule alongside its history. Installation-key encryption
// permits process restarts without keeping an unbounded server-side signature cache.
func (s *Server) chatReplay(x *gatewayExchange, t routing.Target) protocol.ChatReplay {
	key := sha256.Sum256([]byte("pacaroute-chat-replay-v1\x00" + s.AdminKey))
	block, _ := aes.NewCipher(key[:])
	aead, _ := cipher.NewGCM(block)
	pool := ""
	if t.Pool != nil {
		pool = t.Pool.ID
	}
	scope, _ := json.Marshal([]string{x.affinityScope, string(x.agent), x.session, t.Source.ID, t.Source.Endpoint, t.Source.AccountID, t.Model, pool})
	return protocol.ChatReplay{
		Seal: func(plain []byte) (string, error) {
			if len(plain) > maxChatReplay {
				return "", errors.New("Chat replay exceeds 8 MiB")
			}
			nonce := make([]byte, aead.NonceSize())
			if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
				return "", err
			}
			sealed := aead.Seal(nonce, nonce, plain, scope)
			return chatReplayPrefix + base64.RawURLEncoding.EncodeToString(sealed), nil
		},
		Open: func(token string) ([]byte, error) {
			fail := errors.New("Chat reasoning belongs to another client/session/source or is invalid; start a new conversation")
			if !strings.HasPrefix(token, chatReplayPrefix) || len(token) > 2*maxChatReplay {
				return nil, fail
			}
			data, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(token, chatReplayPrefix))
			if err != nil || len(data) < aead.NonceSize() {
				return nil, fail
			}
			plain, err := aead.Open(nil, data[:aead.NonceSize()], data[aead.NonceSize():], scope)
			if err != nil {
				return nil, fail
			}
			return plain, nil
		},
	}
}
