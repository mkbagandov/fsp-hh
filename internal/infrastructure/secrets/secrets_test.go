package secrets

import (
	"strings"
	"testing"
)

func TestPasswordHashRoundTrip(t *testing.T) {
	h, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=65536,t=3,p=2$") {
		t.Fatalf("unexpected format: %s", h)
	}
	ok, err := VerifyPassword("correct horse battery staple", h)
	if err != nil || !ok {
		t.Fatalf("verify valid: ok=%v err=%v", ok, err)
	}
	ok, err = VerifyPassword("wrong", h)
	if err != nil || ok {
		t.Fatalf("verify wrong: ok=%v err=%v", ok, err)
	}
	if _, err := VerifyPassword("x", "$bcrypt$..."); err == nil {
		t.Fatal("foreign format must be rejected")
	}
	h2, _ := HashPassword("correct horse battery staple")
	if h2 == h {
		t.Fatal("salt must differ between hashes")
	}
}

func TestTokensAndCodes(t *testing.T) {
	tok, err := RandomToken(32)
	if err != nil || len(tok) < 40 {
		t.Fatalf("token: %q %v", tok, err)
	}
	code, err := NumericCode(6)
	if err != nil || len(code) != 6 {
		t.Fatalf("code: %q %v", code, err)
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			t.Fatalf("non-digit in code %q", code)
		}
	}
	a := HashToken("user-1", "123456")
	b := HashToken("user-2", "123456")
	if a == b || len(a) != 64 {
		t.Fatal("hash must depend on salt and be sha256 hex")
	}
	if !EqualHash(a, HashToken("user-1", "123456")) || EqualHash(a, b) {
		t.Fatal("equal hash")
	}
}
