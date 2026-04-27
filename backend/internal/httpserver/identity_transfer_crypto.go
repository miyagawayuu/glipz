package httpserver

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	identityBundleV2           = 2
	identityTransferTokenBytes = 32
	identityBundleSaltBytes    = 16
	identityBundleNonceBytes   = 12
)

type encryptedIdentitySecret struct {
	KDF        string `json:"kdf"`
	Salt       string `json:"salt"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

type identityBundleV2Payload struct {
	AccountPrivateKey string `json:"account_private_key"`
}

func randomBase64URL(n int) (string, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashTransferToken(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func hmacIP(secret []byte, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	m := hmac.New(sha256.New, secret)
	_, _ = m.Write([]byte("identity-transfer-ip\000"))
	_, _ = m.Write([]byte(raw))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func constantEqualBase64Hash(a, b string) bool {
	aa, errA := base64.RawURLEncoding.DecodeString(strings.TrimSpace(a))
	bb, errB := base64.RawURLEncoding.DecodeString(strings.TrimSpace(b))
	if errA != nil || errB != nil || len(aa) == 0 || len(aa) != len(bb) {
		return false
	}
	return hmac.Equal(aa, bb)
}

func deriveIdentityBundleKey(passphrase, salt []byte) []byte {
	return argon2.IDKey(passphrase, salt, 3, 64*1024, 2, 32)
}

func encryptIdentityPrivateKey(passphrase, privateKey string) (encryptedIdentitySecret, error) {
	passphrase = strings.TrimSpace(passphrase)
	if len(passphrase) < 12 {
		return encryptedIdentitySecret{}, fmt.Errorf("weak passphrase")
	}
	salt := make([]byte, identityBundleSaltBytes)
	nonce := make([]byte, identityBundleNonceBytes)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return encryptedIdentitySecret{}, err
	}
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return encryptedIdentitySecret{}, err
	}
	key := deriveIdentityBundleKey([]byte(passphrase), salt)
	block, err := aes.NewCipher(key)
	if err != nil {
		return encryptedIdentitySecret{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return encryptedIdentitySecret{}, err
	}
	plain, err := json.Marshal(identityBundleV2Payload{AccountPrivateKey: strings.TrimSpace(privateKey)})
	if err != nil {
		return encryptedIdentitySecret{}, err
	}
	return encryptedIdentitySecret{
		KDF:        "argon2id:v=19,m=65536,t=3,p=2",
		Salt:       base64.RawURLEncoding.EncodeToString(salt),
		Nonce:      base64.RawURLEncoding.EncodeToString(nonce),
		Ciphertext: base64.RawURLEncoding.EncodeToString(gcm.Seal(nil, nonce, plain, nil)),
	}, nil
}

func decryptIdentityPrivateKey(passphrase string, enc encryptedIdentitySecret) (string, error) {
	if !strings.HasPrefix(strings.TrimSpace(enc.KDF), "argon2id:") {
		return "", fmt.Errorf("unsupported kdf")
	}
	salt, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(enc.Salt))
	if err != nil {
		return "", err
	}
	nonce, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(enc.Nonce))
	if err != nil {
		return "", err
	}
	ct, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(enc.Ciphertext))
	if err != nil {
		return "", err
	}
	key := deriveIdentityBundleKey([]byte(strings.TrimSpace(passphrase)), salt)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("invalid identity bundle")
	}
	var payload identityBundleV2Payload
	if err := json.Unmarshal(plain, &payload); err != nil {
		return "", err
	}
	if strings.TrimSpace(payload.AccountPrivateKey) == "" {
		return "", fmt.Errorf("invalid identity bundle")
	}
	return strings.TrimSpace(payload.AccountPrivateKey), nil
}

func serverAEAD(secret []byte) (cipher.AEAD, error) {
	sum := sha256.Sum256(append([]byte("identity-transfer-token\000"), secret...))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func (s *Server) encryptServerSecret(value string) (string, error) {
	aead, err := serverAEAD(s.secret)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := aead.Seal(nil, nonce, []byte(strings.TrimSpace(value)), nil)
	return base64.RawURLEncoding.EncodeToString(append(nonce, ct...)), nil
}

func (s *Server) decryptServerSecret(value string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return "", err
	}
	aead, err := serverAEAD(s.secret)
	if err != nil {
		return "", err
	}
	if len(raw) <= aead.NonceSize() {
		return "", fmt.Errorf("invalid encrypted secret")
	}
	nonce, ct := raw[:aead.NonceSize()], raw[aead.NonceSize():]
	plain, err := aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
