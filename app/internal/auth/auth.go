package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Auth holds authentication configuration
type Auth struct {
	User           string
	Hash           []byte
	HmacSecret     []byte
	InsecureDev    bool
	SessionMaxAgeS int
}

// Session represents a user session
type Session struct {
	U   string `json:"u"`
	Exp int64  `json:"exp"`
}

// NewAuth creates a new Auth instance
func NewAuth(user string, hash []byte, secret []byte, insecure bool, maxAge int) *Auth {
	return &Auth{
		User:           user,
		Hash:           hash,
		HmacSecret:     secret,
		InsecureDev:    insecure,
		SessionMaxAgeS: maxAge,
	}
}

// SetCSRFCookie sets a CSRF token cookie
func (a *Auth) SetCSRFCookie(w http.ResponseWriter) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	val := base64.RawURLEncoding.EncodeToString(b)
	c := &http.Cookie{
		Name:     "csrf",
		Value:    val,
		Path:     "/",
		MaxAge:   a.SessionMaxAgeS,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
		Secure:   !a.InsecureDev,
	}
	http.SetCookie(w, c)
	return val, nil
}

// VerifyCSRF verifies CSRF token
func (a *Auth) VerifyCSRF(r *http.Request) bool {
	cookieVal := ""
	if c, err := r.Cookie("csrf"); err == nil {
		cookieVal = c.Value
	}
	headerVal := r.Header.Get("X-CSRF-Token")
	return cookieVal != "" && headerVal != "" && cookieVal == headerVal
}

// MakeSessionCookie creates a session cookie
func (a *Auth) MakeSessionCookie(w http.ResponseWriter, username string, maxAge time.Duration) error {
	exp := time.Now().Add(maxAge).Unix()
	payload := fmt.Sprintf(`{"u":"%s","exp":%d}`, username, exp)
	sig := a.sign([]byte(payload))
	val := base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + sig
	c := &http.Cookie{
		Name:     "sess",
		Value:    val,
		Path:     "/",
		MaxAge:   int(maxAge.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   !a.InsecureDev,
	}
	http.SetCookie(w, c)
	_, _ = a.SetCSRFCookie(w)
	return nil
}

// ClearSessionCookie removes session cookies
func (a *Auth) ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: "sess", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: !a.InsecureDev})
	http.SetCookie(w, &http.Cookie{Name: "csrf", Value: "", Path: "/", MaxAge: -1, HttpOnly: false, SameSite: http.SameSiteLaxMode, Secure: !a.InsecureDev})
}

// ParseSession parses and validates a session cookie
func (a *Auth) ParseSession(r *http.Request) (*Session, error) {
	c, err := r.Cookie("sess")
	if err != nil || c.Value == "" {
		return nil, errors.New("no session")
	}
	parts := strings.Split(c.Value, ".")
	if len(parts) != 2 {
		return nil, errors.New("bad cookie")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, errors.New("decode")
	}
	want := parts[1]
	if a.sign(raw) != want {
		return nil, errors.New("bad sig")
	}
	var s Session
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, errors.New("json")
	}
	if time.Now().Unix() > s.Exp {
		return nil, errors.New("expired")
	}
	return &s, nil
}

// RequireAuth is middleware that requires authentication
func (a *Auth) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := a.ParseSession(r); err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !a.VerifyCSRF(r) && r.Method != http.MethodGet {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func (a *Auth) sign(b []byte) string {
	m := hmac.New(sha256.New, a.HmacSecret)
	m.Write(b)
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}
