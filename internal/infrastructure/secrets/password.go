// Package secrets — хэширование паролей (Argon2id), генерация и хэширование
// одноразовых кодов и токенов.
package secrets

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"runtime"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Параметры Argon2id (docs/02-data-model.md, раздел 5): 64 МиБ, 3 итерации, 2 потока.
const (
	argonMemory  = 64 * 1024
	argonTime    = 3
	argonThreads = 2
	argonKeyLen  = 32
	argonSaltLen = 16
)

// argonSlots ограничивает одновременные вычисления Argon2id: каждое занимает 64 МиБ, и поток входов
// (в том числе на несуществующие e-mail, где хэш считается ради выравнивания времени) без лимита
// исчерпал бы память процесса. Остальные запросы ждут своей очереди.
var argonSlots = make(chan struct{}, max(2, runtime.NumCPU()))

func argonKey(password, salt []byte, t, m uint32, p uint8, keyLen uint32) []byte {
	argonSlots <- struct{}{}
	defer func() { <-argonSlots }()
	return argon2.IDKey(password, salt, t, m, p, keyLen)
}

// ErrHashFormat — строка хэша повреждена или не того формата.
var ErrHashFormat = errors.New("secrets: unsupported hash format")

// HashPassword возвращает хэш в формате PHC: $argon2id$v=19$m=65536,t=3,p=2$<salt>$<hash>.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("secrets: salt: %w", err)
	}
	key := argonKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword сравнивает пароль с хэшем за постоянное время.
func VerifyPassword(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, ErrHashFormat
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, ErrHashFormat
	}
	var memory, iterations uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &threads); err != nil {
		return false, ErrHashFormat
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, ErrHashFormat
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) == 0 || len(want) > 1024 {
		return false, ErrHashFormat
	}
	keyLen := uint32(len(want)) //nolint:gosec // длина ограничена проверкой выше
	if memory > argonMemory || iterations > 10 || threads > 8 {
		return false, ErrHashFormat // хэши создаём только мы: завышенные параметры — признак подмены
	}
	got := argonKey([]byte(password), salt, iterations, memory, threads, keyLen)
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
