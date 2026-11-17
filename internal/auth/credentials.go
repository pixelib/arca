package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"arca/internal/store/models"
)

var (
	ErrInvalidCredentials = errors.New("auth: invalid email or password")
	ErrThrottled          = errors.New("auth: too many failed attempts")
)

// ThrottledError carries how long the caller should wait.
type ThrottledError struct{ RetryAfter time.Duration }

func (e ThrottledError) Error() string { return ErrThrottled.Error() }
func (e ThrottledError) Is(target error) bool {
	return target == ErrThrottled
}

// verifiedTTL is how long a login and password pair that has been checked once
// is trusted without deriving the key again. The entry is bound to the stored
// hash, so a password change ends it at once.
const verifiedTTL = 5 * time.Minute

type credentialCache struct {
	mutex   sync.Mutex
	entries map[string]time.Time
	now     func() time.Time
}

func newCredentialCache() *credentialCache {
	return &credentialCache{entries: map[string]time.Time{}, now: time.Now}
}

func credentialKey(login, password, storedHash string) string {
	return SHA256Hex(login + "\x00" + password + "\x00" + storedHash)
}

func (c *credentialCache) has(key string) bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	expires, ok := c.entries[key]
	if !ok {
		return false
	}
	if !c.now().Before(expires) {
		delete(c.entries, key)
		return false
	}
	return true
}

func (c *credentialCache) add(key string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	now := c.now()
	if len(c.entries) >= 4096 {
		for candidate, expires := range c.entries {
			if !now.Before(expires) {
				delete(c.entries, candidate)
			}
		}
	}
	c.entries[key] = now.Add(verifiedTTL)
}

// decoyHash is a real hash of a password nobody knows. Checking a password
// against it costs exactly what checking against an account's hash costs, so an
// email that does not exist takes as long to reject as a wrong password does.
var decoyHash = sync.OnceValue(func() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		panic("auth: unable to read random bytes: " + err.Error())
	}
	hash, err := HashPassword(hex.EncodeToString(buffer))
	if err != nil {
		panic("auth: unable to derive the decoy hash: " + err.Error())
	}
	return hash
})

// Authenticate is the one path every password check takes: the interactive
// login, the npm login, and Basic credentials on a repository request. It is
// throttled per client address and per account, takes the same time whether the
// account exists or not, and remembers a verified pair for a while so a client
// that presents Basic credentials on every request is not charged a key
// derivation each time. An account without a password, one that signs in only
// through the identity provider, is rejected the same way as a missing one.
func (r *Resolver) Authenticate(address, login, password string) (*models.User, error) {
	if ok, wait := r.guard.Allow(address, login); !ok {
		return nil, ThrottledError{RetryAfter: wait}
	}

	user, err := r.store.UserByEmail(login)
	if err != nil || user.Disabled || user.PasswordHash == "" {
		VerifyPassword(password, decoyHash())
		r.guard.Fail(address, login)
		return nil, ErrInvalidCredentials
	}

	key := credentialKey(login, password, user.PasswordHash)
	if r.verified.has(key) {
		return user, nil
	}
	if !VerifyPassword(password, user.PasswordHash) {
		r.guard.Fail(address, login)
		return nil, ErrInvalidCredentials
	}

	r.verified.add(key)
	return user, nil
}

// Guard exposes the throttle for checks that are not a login but still cost a
// key derivation, such as confirming the current password before changing it.
func (r *Resolver) Guard() *Guard { return r.guard }
