package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"arca/internal/store"
	"arca/internal/store/models"
)

const (
	SessionCookie = "cn_session"
	SessionTTL    = 30 * 24 * time.Hour
)

func CreateSession(database *store.Store, userID string) (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(buffer)

	now := store.NowMillis()
	session := &models.Session{
		ID:        SHA256Hex(token),
		UserID:    userID,
		CreatedAt: now,
		ExpiresAt: now + SessionTTL.Milliseconds(),
	}
	if err := database.CreateSession(session); err != nil {
		return "", err
	}
	return token, nil
}

func DestroySession(database *store.Store, token string) error {
	return database.DeleteSession(SHA256Hex(token))
}

func AttachSessionCookie(writer http.ResponseWriter, request *http.Request, token string) {
	http.SetCookie(writer, &http.Cookie{
		Name:     SessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   IsSecure(request),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(SessionTTL.Seconds()),
	})
}

func ClearSessionCookie(writer http.ResponseWriter, request *http.Request) {
	http.SetCookie(writer, &http.Cookie{
		Name:     SessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   IsSecure(request),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func SessionToken(request *http.Request) string {
	cookie, err := request.Cookie(SessionCookie)
	if err != nil {
		return ""
	}
	return cookie.Value
}

// IsSecure decides whether a cookie set on this request gets the Secure flag.
func IsSecure(request *http.Request) bool {
	return request.TLS != nil || request.Header.Get("X-Forwarded-Proto") == "https"
}
