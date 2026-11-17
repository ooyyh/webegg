package auth

import (
	"strings"
	"testing"
)

func TestGenerateAndVerifyToken(t *testing.T) {
	token, hash, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(token, TokenPrefix) {
		t.Fatalf("token does not have expected prefix: %q", token)
	}
	if !VerifyToken(token, hash) {
		t.Fatal("generated token should verify")
	}
	if VerifyToken(token+"x", hash) {
		t.Fatal("modified token must not verify")
	}
	if VerifyToken("invalid", hash) {
		t.Fatal("token with wrong prefix must not verify")
	}
	if VerifyToken(token, "not-a-hash") {
		t.Fatal("invalid stored hash must not verify")
	}
}

func TestGeneratedTokensAreUnique(t *testing.T) {
	first, _, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("two generated tokens should not match")
	}
}
