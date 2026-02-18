package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const schemaVersion = 1

type Store struct {
	secretsPath string
	keyPath     string
	mu          sync.Mutex
}

type OpenAICodexOAuthCredentials struct {
	AccessToken  string    `json:"access_token,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	IDToken      string    `json:"id_token,omitempty"`
	AccountLabel string    `json:"account_label,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
}

type Secrets struct {
	SchemaVersion    int                          `json:"schema_version"`
	OpenAIKey        string                       `json:"openai_api_key,omitempty"`
	AnthropicKey     string                       `json:"anthropic_api_key,omitempty"`
	GoogleKey        string                       `json:"google_api_key,omitempty"`
	OpenAICodexOAuth *OpenAICodexOAuthCredentials `json:"openai_codex_oauth,omitempty"`
}

type encryptedPayload struct {
	SchemaVersion int    `json:"schema_version"`
	Nonce         string `json:"nonce"`
	Ciphertext    string `json:"ciphertext"`
}

func NewStore(secretsPath, keyPath string) *Store {
	return &Store{secretsPath: secretsPath, keyPath: keyPath}
}

func (s *Store) GetOpenAIKey() (string, error) {
	secrets, err := s.load()
	if err != nil {
		return "", err
	}
	return secrets.OpenAIKey, nil
}

func (s *Store) SetOpenAIKey(key string) error {
	secrets, err := s.load()
	if err != nil {
		return err
	}
	secrets.OpenAIKey = key
	return s.save(secrets)
}

func (s *Store) GetAnthropicKey() (string, error) {
	secrets, err := s.load()
	if err != nil {
		return "", err
	}
	return secrets.AnthropicKey, nil
}

func (s *Store) SetAnthropicKey(key string) error {
	secrets, err := s.load()
	if err != nil {
		return err
	}
	secrets.AnthropicKey = key
	return s.save(secrets)
}

func (s *Store) GetGoogleKey() (string, error) {
	secrets, err := s.load()
	if err != nil {
		return "", err
	}
	return secrets.GoogleKey, nil
}

func (s *Store) SetGoogleKey(key string) error {
	secrets, err := s.load()
	if err != nil {
		return err
	}
	secrets.GoogleKey = key
	return s.save(secrets)
}

func (s *Store) GetOpenAICodexOAuthCredentials() (*OpenAICodexOAuthCredentials, error) {
	secrets, err := s.load()
	if err != nil {
		return nil, err
	}
	if secrets.OpenAICodexOAuth == nil {
		return nil, nil
	}
	copy := *secrets.OpenAICodexOAuth
	return &copy, nil
}

func (s *Store) SetOpenAICodexOAuthCredentials(creds *OpenAICodexOAuthCredentials) error {
	secrets, err := s.load()
	if err != nil {
		return err
	}
	if creds == nil {
		secrets.OpenAICodexOAuth = nil
		return s.save(secrets)
	}
	copy := *creds
	secrets.OpenAICodexOAuth = &copy
	return s.save(secrets)
}

func (s *Store) ClearProviderKey(providerID string) error {
	secrets, err := s.load()
	if err != nil {
		return err
	}
	switch providerID {
	case "openai":
		secrets.OpenAIKey = ""
	case "anthropic":
		secrets.AnthropicKey = ""
	case "google":
		secrets.GoogleKey = ""
	case "openai-codex":
		secrets.OpenAICodexOAuth = nil
	default:
		return errors.New("unsupported provider")
	}
	return s.save(secrets)
}

func (s *Store) load() (*Secrets, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.secretsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &Secrets{SchemaVersion: schemaVersion}, nil
		}
		return nil, err
	}
	var payload encryptedPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	key, err := s.loadOrCreateKey()
	if err != nil {
		return nil, err
	}
	nonce, err := base64.StdEncoding.DecodeString(payload.Nonce)
	if err != nil {
		return nil, err
	}
	ciphertext, err := base64.StdEncoding.DecodeString(payload.Ciphertext)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	var secrets Secrets
	if err := json.Unmarshal(plain, &secrets); err != nil {
		return nil, err
	}
	if secrets.SchemaVersion == 0 {
		secrets.SchemaVersion = schemaVersion
	}
	return &secrets, nil
}

func (s *Store) save(secrets *Secrets) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, err := s.loadOrCreateKey()
	if err != nil {
		return err
	}
	plain, err := json.Marshal(secrets)
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}
	ciphertext := gcm.Seal(nil, nonce, plain, nil)
	payload := encryptedPayload{
		SchemaVersion: schemaVersion,
		Nonce:         base64.StdEncoding.EncodeToString(nonce),
		Ciphertext:    base64.StdEncoding.EncodeToString(ciphertext),
	}
	if err := os.MkdirAll(filepath.Dir(s.secretsPath), 0o755); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.secretsPath, encoded, 0o600)
}

func (s *Store) loadOrCreateKey() ([]byte, error) {
	key, err := os.ReadFile(s.keyPath)
	if err == nil {
		if len(key) != 32 {
			return nil, errors.New("invalid master key length")
		}
		return key, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(s.keyPath), 0o755); err != nil {
		return nil, err
	}
	key = make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(s.keyPath, key, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}
