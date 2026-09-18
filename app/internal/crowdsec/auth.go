package crowdsec

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"sync"
	"time"
)

const (
	// tokenRefreshSkew refreshes the JWT this long before its expiry
	// to absorb clock drift between Servicarr and LAPI.
	tokenRefreshSkew = time.Minute

	// loginErrBackoff negative-caches failed logins so a persistently
	// failing LAPI isn't hammered with auth attempts every poll.
	loginErrBackoff = 30 * time.Second
)

// tokenCache stores the machine JWT. Thread-safe; a failed login is
// retried at most once per backoff window. Concurrent callers coalesce
// onto a single in-flight login (single-flight via sync.Cond).
type tokenCache struct {
	mu        sync.Mutex
	cond      *sync.Cond
	jwt       string
	expires   time.Time
	loginErr  error
	loginErrA time.Time
	loggingIn bool
}

func newTokenCache() *tokenCache {
	t := &tokenCache{}
	t.cond = sync.NewCond(&t.mu)
	return t
}

// get returns a cached token if still valid (with skew margin).
// Caller must hold c.mu.
func (c *tokenCache) get() (string, bool) {
	if c.jwt != "" && time.Now().Add(tokenRefreshSkew).Before(c.expires) {
		return c.jwt, true
	}
	return "", false
}

// token returns a valid JWT, logging in via the provided login function if
// necessary. The login func runs OUTSIDE the lock so concurrent callers
// coalesce onto a single in-flight request; waiters re-check after wake-up.
// After a 401 the caller invokes invalidate and calls token again — the
// client budgets exactly one re-login attempt.
func (c *tokenCache) token(ctx context.Context, login func(context.Context) (string, time.Time, error)) (string, error) {
	c.mu.Lock()
	for {
		if tok, ok := c.get(); ok {
			c.mu.Unlock()
			return tok, nil
		}
		if c.loginErr != nil && time.Since(c.loginErrA) < loginErrBackoff {
			err := c.loginErr
			c.mu.Unlock()
			return "", err
		}
		if c.loggingIn {
			c.cond.Wait()
			continue
		}
		c.loggingIn = true
		c.mu.Unlock()

		tok, exp, err := login(ctx)

		c.mu.Lock()
		c.loggingIn = false
		if err != nil {
			c.jwt = ""
			c.expires = time.Time{}
			c.loginErr = err
			c.loginErrA = time.Now()
		} else {
			c.jwt = tok
			c.expires = exp
			c.loginErr = nil
		}
		c.cond.Broadcast()
		c.mu.Unlock()
		return tok, err
	}
}

// invalidate drops the cached token so the next call re-logins.
func (c *tokenCache) invalidate() {
	c.mu.Lock()
	c.jwt = ""
	c.expires = time.Time{}
	c.mu.Unlock()
}

// parseJWTExpiry extracts the "exp" claim without verifying the signature.
// Signature verification is pointless client-side: we do not hold LAPI's
// signing key, and a MITM able to forge a token already owns the connection.
func parseJWTExpiry(token string) time.Time {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(payload, &claims) != nil || claims.Exp == 0 {
		return time.Time{}
	}
	return time.Unix(claims.Exp, 0)
}
