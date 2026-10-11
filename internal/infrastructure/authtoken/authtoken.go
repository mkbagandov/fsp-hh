// Package authtoken выпускает и проверяет короткоживущие access-токены (JWT, HS256).
package authtoken

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"fsp/internal/domain"
)

// Claims — содержимое access-токена.
type Claims struct {
	Role  string `json:"role"`
	Email string `json:"email"`
	jwt.RegisteredClaims
}

// Issuer выпускает и проверяет токены.
type Issuer struct {
	secret []byte
	ttl    time.Duration
	issuer string
	now    func() time.Time
}

// ErrInvalid — токен не прошёл проверку.
var ErrInvalid = errors.New("authtoken: invalid token")

// New создаёт эмитент. Секрет должен быть не короче 32 байт.
func New(secret string, ttl time.Duration, issuer string) (*Issuer, error) {
	if len(secret) < 32 {
		return nil, errors.New("authtoken: JWT_SECRET must be at least 32 characters")
	}
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	return &Issuer{secret: []byte(secret), ttl: ttl, issuer: issuer, now: time.Now}, nil
}

// TTL — срок жизни access-токена.
func (i *Issuer) TTL() time.Duration { return i.ttl }

// IssuerName возвращает имя эмитента токенов.
func (i *Issuer) IssuerName() string { return i.issuer }

// stateClaims — подписанное состояние для многошаговых потоков (OIDC): хранится в cookie.
type stateClaims struct {
	Data map[string]string `json:"d"`
	jwt.RegisteredClaims
}

const stateAudience = "fsp-state"

// IssueState подписывает произвольные строковые данные с ограниченным сроком.
func (i *Issuer) IssueState(data map[string]string, ttl time.Duration) (string, error) {
	now := i.now()
	claims := stateClaims{
		Data: data,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    i.issuer,
			Audience:  jwt.ClaimStrings{stateAudience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			ID:        uuid.NewString(),
		},
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(i.secret)
	if err != nil {
		return "", fmt.Errorf("authtoken: sign state: %w", err)
	}
	return s, nil
}

// ParseState проверяет подпись и срок и возвращает данные.
func (i *Issuer) ParseState(token string) (map[string]string, error) {
	var claims stateClaims
	parsed, err := jwt.ParseWithClaims(token, &claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalid
		}
		return i.secret, nil
	}, jwt.WithIssuer(i.issuer), jwt.WithAudience(stateAudience), jwt.WithTimeFunc(i.now))
	if err != nil || !parsed.Valid {
		return nil, ErrInvalid
	}
	return claims.Data, nil
}

// Issue выпускает токен для субъекта.
func (i *Issuer) Issue(p domain.Principal) (string, time.Time, error) {
	now := i.now()
	exp := now.Add(i.ttl)
	claims := Claims{
		Role:  string(p.Role),
		Email: p.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   p.UserID.String(),
			Issuer:    i.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
			ID:        uuid.NewString(),
		},
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(i.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("authtoken: sign: %w", err)
	}
	return s, exp, nil
}

// Parse проверяет подпись и срок и возвращает субъекта.
func (i *Issuer) Parse(token string) (domain.Principal, error) {
	var claims Claims
	parsed, err := jwt.ParseWithClaims(token, &claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalid
		}
		return i.secret, nil
	}, jwt.WithIssuer(i.issuer), jwt.WithLeeway(30*time.Second), jwt.WithTimeFunc(i.now))
	if err != nil || !parsed.Valid {
		return domain.Principal{}, ErrInvalid
	}
	id, err := uuid.Parse(claims.Subject)
	if err != nil {
		return domain.Principal{}, ErrInvalid
	}
	role := domain.Role(claims.Role)
	if !role.Valid() {
		return domain.Principal{}, ErrInvalid
	}
	return domain.Principal{UserID: id, Role: role, Email: claims.Email}, nil
}
