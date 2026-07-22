package config

import "testing"

func base() Config {
	return Config{
		DatabaseURL: "postgres://x",
		JWTSecret:   "0123456789abcdef0123456789abcdef", // 32 chars
		Env:         "development",
	}
}

func TestValidate_RejectsWeakJWTSecret(t *testing.T) {
	for _, s := range []string{"", "cambia-esto-por-un-secreto", "short", "0123456789abcdef"} { // last is 16 chars
		c := base()
		c.JWTSecret = s
		if err := Validate(c); err == nil {
			t.Errorf("JWT_SECRET %q should be rejected", s)
		}
	}
}

func TestValidate_AcceptsStrongSecret(t *testing.T) {
	if err := Validate(base()); err != nil {
		t.Fatalf("strong config should pass, got %v", err)
	}
}

func TestValidate_RejectsWildcardCORSInProd(t *testing.T) {
	c := base()
	c.Env = "production"
	c.CORSOrigin = "*"
	if err := Validate(c); err == nil {
		t.Fatal("CORS_ORIGIN=* must be rejected in production")
	}
	c.CORSOrigin = "https://app.elgatobobah.com"
	if err := Validate(c); err != nil {
		t.Fatalf("exact origin should pass in prod, got %v", err)
	}
}

func TestIsPlaceholder(t *testing.T) {
	for _, s := range []string{"", "cambia-esto", "cambia-esto-por-un-secreto", "your_secret_here"} {
		if !IsPlaceholder(s) {
			t.Errorf("%q should be a placeholder", s)
		}
	}
	if IsPlaceholder("un-secreto-real-aleatorio") {
		t.Error("real secret must not be a placeholder")
	}
}
