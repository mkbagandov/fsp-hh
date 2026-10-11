package secrets

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
)

// RandomToken возвращает криптослучайный токен из n байт в base64url (без паддинга).
func RandomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("secrets: random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// NumericCode возвращает случайный код из digits цифр (ведущие нули сохраняются).
func NumericCode(digits int) (string, error) {
	maxVal := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(digits)), nil)
	n, err := rand.Int(rand.Reader, maxVal)
	if err != nil {
		return "", fmt.Errorf("secrets: code: %w", err)
	}
	return fmt.Sprintf("%0*d", digits, n), nil
}

// HashToken — SHA-256 в hex: так хранятся refresh-токены и коды подтверждения.
// Соль (идентификатор пользователя) мешает подбору кода по таблице хэшей.
func HashToken(salt, token string) string {
	sum := sha256.Sum256([]byte(salt + ":" + token))
	return hex.EncodeToString(sum[:])
}

// EqualHash сравнивает два хэша за постоянное время.
func EqualHash(a, b string) bool {
	return hmac.Equal([]byte(a), []byte(b))
}
