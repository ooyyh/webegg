package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"strings"
)

const TokenPrefix = "we_"

func GenerateToken() (plainText string, hash string, err error) {
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		return "", "", err
	}

	plainText = TokenPrefix + base64.RawURLEncoding.EncodeToString(secret)
	return plainText, HashToken(plainText), nil
}

func HashToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func VerifyToken(token, expectedHash string) bool {
	if !strings.HasPrefix(token, TokenPrefix) || len(expectedHash) != sha256.Size*2 {
		return false
	}

	expected, err := hex.DecodeString(expectedHash)
	if err != nil || len(expected) != sha256.Size {
		return false
	}

	actual := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(actual[:], expected) == 1
}
