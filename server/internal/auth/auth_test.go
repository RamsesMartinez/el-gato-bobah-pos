package auth

import (
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

func TestIssueParseRoundTrip(t *testing.T) {
	m := NewManager("secret", nil)
	tok, err := m.Issue(domain.User{ID: 42, Name: "Kate", Role: domain.RoleCajero})
	if err != nil {
		t.Fatal(err)
	}
	claims, err := m.Parse(tok)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != "42" || claims.Name != "Kate" || claims.Role != domain.RoleCajero {
		t.Fatalf("claims mismatch: %+v", claims)
	}
}

func TestExpiredTokenRejected(t *testing.T) {
	past := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	issuer := NewManager("secret", func() time.Time { return past })
	tok, _ := issuer.Issue(domain.User{ID: 1, Role: domain.RoleAdmin})
	// verifier with real clock sees an expired token
	if _, err := NewManager("secret", nil).Parse(tok); err == nil {
		t.Fatal("expected expired token to be rejected")
	}
}

func TestWrongSecretRejected(t *testing.T) {
	tok, _ := NewManager("secret-a", nil).Issue(domain.User{ID: 1, Role: domain.RoleAdmin})
	if _, err := NewManager("secret-b", nil).Parse(tok); err == nil {
		t.Fatal("expected wrong-secret token to be rejected")
	}
}

func TestSecretHashing(t *testing.T) {
	h, err := HashSecret("1234")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckSecret(h, "1234") {
		t.Fatal("correct secret should match")
	}
	if CheckSecret(h, "9999") {
		t.Fatal("wrong secret must not match")
	}
}

func TestRefreshTokenHashStable(t *testing.T) {
	tok, hash, err := NewRefreshToken()
	if err != nil {
		t.Fatal(err)
	}
	if HashToken(tok) != hash {
		t.Fatal("hash of token must equal returned hash")
	}
}
