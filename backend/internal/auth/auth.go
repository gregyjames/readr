package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const SessionMaxAge = 30 * 24 * time.Hour

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

func VerifyPassword(hash, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func GenerateRandomSecret() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func SignSession(secret string, now time.Time) string {
	tsMillis := strconv.FormatInt(now.UnixMilli(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(tsMillis))
	signature := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("%s.%s", tsMillis, signature)
}

func VerifySession(secret, token string, now time.Time) (bool, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return false, errors.New("invalid token format")
	}

	tsMillisStr, signature := parts[0], parts[1]
	tsMillis, err := strconv.ParseInt(tsMillisStr, 10, 64)
	if err != nil {
		return false, errors.New("invalid timestamp in token")
	}

	tokenTime := time.UnixMilli(tsMillis)
	if now.Sub(tokenTime) > SessionMaxAge || now.Before(tokenTime.Add(-5*time.Minute)) {
		return false, errors.New("session expired")
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(tsMillisStr))
	expectedSignature := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expectedSignature)) {
		return false, errors.New("invalid token signature")
	}

	return true, nil
}

// GenerateAPIKey generates a new secure API key with 'rdr_live_' prefix and returns (fullKey, keyPrefix, keyHash, error).
func GenerateAPIKey() (string, string, string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", "", err
	}
	randomHex := hex.EncodeToString(bytes)
	fullKey := "rdr_live_" + randomHex
	prefix := fullKey[:13] + "..." // e.g. "rdr_live_9a8f..."
	hash := HashAPIKey(fullKey)
	return fullKey, prefix, hash, nil
}

// HashAPIKey computes the SHA-256 hash of an API key for safe database storage.
func HashAPIKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}
