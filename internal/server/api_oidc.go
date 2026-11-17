package server

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/charmbracelet/log"
	"golang.org/x/oauth2"

	"arca/internal/auth"
	"arca/internal/store"
	"arca/internal/store/models"
)

const (
	oidcCookie     = "cn_oidc"
	oidcCookiePath = "/api/auth/oidc"
	oidcCookieTTL  = 10 * 60 // seconds

	setupIdentityCookie = "cn_setup_identity"
)

// oidcRefusal is a sign-in turned away for a reason the login page explains;
// code is what it is told in ?error=. Every other failure shows as "failed".
type oidcRefusal struct {
	code   string
	reason string
}

func (r oidcRefusal) Error() string { return r.reason }

func (s *Server) oidcRedirectURI(request *http.Request) string {
	return s.publicBase(request) + "/api/auth/oidc/callback"
}

func setOIDCCookie(writer http.ResponseWriter, request *http.Request, value string, maxAge int) {
	http.SetCookie(writer, &http.Cookie{
		Name:     oidcCookie,
		Value:    value,
		Path:     oidcCookiePath,
		HttpOnly: true,
		Secure:   auth.IsSecure(request),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	})
}

// setSetupIdentityCookie hands the browser the secret of an identity waiting
// for setup. It lasts the browser session; a negative maxAge clears it.
func setSetupIdentityCookie(writer http.ResponseWriter, request *http.Request, value string, maxAge int) {
	http.SetCookie(writer, &http.Cookie{
		Name:     setupIdentityCookie,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   auth.IsSecure(request),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	})
}

// setupIdentity returns the identity the request's setup cookie points at.
func (s *Server) setupIdentity(request *http.Request) (auth.OIDCClaims, bool) {
	cookie, err := request.Cookie(setupIdentityCookie)
	if err != nil {
		return auth.OIDCClaims{}, false
	}
	claims, ok := s.setupIdentities.Load(cookie.Value)
	if !ok {
		return auth.OIDCClaims{}, false
	}
	return claims.(auth.OIDCClaims), true
}

// oidcFailed sends the browser back to the login page with a code it can show,
// and keeps the actual cause in the server log.
func oidcFailed(writer http.ResponseWriter, request *http.Request, err error) {
	code := "failed"
	var refusal oidcRefusal
	if errors.As(err, &refusal) {
		code = refusal.code
	}
	log.Warnf("OIDC sign-in failed (%s): %v", code, err)
	http.Redirect(writer, request, "/login?error="+code, http.StatusFound)
}

// routeOIDCLogin starts a sign-in. The state, nonce and PKCE verifier ride in a
// short-lived cookie scoped to the callback; none of them is useful to anyone
// who can read it, so it is not signed.
func (s *Server) routeOIDCLogin(writer http.ResponseWriter, request *http.Request) error {
	if s.oidc == nil {
		return fail(http.StatusNotFound, "Not found")
	}

	state, nonce, verifier := store.NewID(), store.NewID(), oauth2.GenerateVerifier()
	target, err := s.oidc.AuthURL(request.Context(), s.oidcRedirectURI(request), state, nonce, verifier)
	if err != nil {
		oidcFailed(writer, request, err)
		return nil
	}

	setOIDCCookie(writer, request, state+"."+nonce+"."+verifier, oidcCookieTTL)
	http.Redirect(writer, request, target, http.StatusFound)
	return nil
}

func (s *Server) routeOIDCCallback(writer http.ResponseWriter, request *http.Request) error {
	if s.oidc == nil {
		return fail(http.StatusNotFound, "Not found")
	}

	if err := s.completeOIDCLogin(writer, request); err != nil {
		// A refused sign-in must not leave an earlier one waiting for setup:
		// the provider may since have taken that person out of a group.
		if cookie, err := request.Cookie(setupIdentityCookie); err == nil {
			s.setupIdentities.Delete(cookie.Value)
			setSetupIdentityCookie(writer, request, "", -1)
		}
		oidcFailed(writer, request, err)
		return nil
	}
	http.Redirect(writer, request, "/", http.StatusFound)
	return nil
}

func (s *Server) completeOIDCLogin(writer http.ResponseWriter, request *http.Request) error {
	cookie, err := request.Cookie(oidcCookie)
	setOIDCCookie(writer, request, "", -1)
	if err != nil {
		return errors.New("sign-in cookie is missing or expired")
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 3 {
		return errors.New("sign-in cookie is malformed")
	}
	state, nonce, verifier := parts[0], parts[1], parts[2]

	query := request.URL.Query()
	if subtle.ConstantTimeCompare([]byte(state), []byte(query.Get("state"))) != 1 {
		return errors.New("state does not match")
	}
	code := query.Get("code")
	if code == "" {
		return fmt.Errorf("no authorization code, provider returned error %q", query.Get("error"))
	}

	claims, err := s.oidc.Exchange(request.Context(), s.oidcRedirectURI(request), code, verifier, nonce)
	if err != nil {
		return err
	}
	if err := s.checkAllowedGroups(claims); err != nil {
		return err
	}

	initialized, err := s.store.IsInitialized()
	if err != nil {
		return err
	}
	if !initialized {
		// Before setup nothing is provisioned, which is what keeps sign-in
		// through the provider from creating accounts on an uninitialized
		// instance. The identity waits for the wizard to make it the
		// administrator instead. With admin groups configured only a member
		// may do that, so the role synced at later sign-ins never disagrees
		// with what setup granted.
		if admins := s.config.OIDC.AdminGroups; len(admins) > 0 && !inAnyGroup(claims.Groups, admins) {
			return oidcRefusal{"not_admin", claims.Email + " is in none of the admin groups and cannot set up the instance"}
		}
		// Signing in again replaces the identity this browser parked before.
		if cookie, err := request.Cookie(setupIdentityCookie); err == nil {
			s.setupIdentities.Delete(cookie.Value)
		}
		secret := store.NewID()
		s.setupIdentities.Store(secret, claims)
		setSetupIdentityCookie(writer, request, secret, 0)
		return nil
	}

	user, err := s.provisionOIDCUser(claims)
	if err != nil {
		return err
	}
	return s.startSession(writer, request, user.ID)
}

func inAnyGroup(groups, wanted []string) bool {
	return slices.ContainsFunc(groups, func(group string) bool { return slices.Contains(wanted, group) })
}

// checkAllowedGroups refuses a sign-in in none of the allowed groups, when
// those are configured.
func (s *Server) checkAllowedGroups(claims auth.OIDCClaims) error {
	allowed := s.config.OIDC.AllowedGroups
	if len(allowed) > 0 && !inAnyGroup(claims.Groups, allowed) {
		return oidcRefusal{"denied", claims.Email + " is in none of the allowed groups"}
	}
	return nil
}

// provisionOIDCUser finds or creates the account behind a verified sign-in and
// brings its email and, when admin groups are configured, its role in line with
// what the provider says. The name is taken from the provider only when the
// account is created, so a name changed in Arca stays.
func (s *Server) provisionOIDCUser(claims auth.OIDCClaims) (*models.User, error) {
	settings := s.config.OIDC

	var user *models.User
	err := s.store.Transaction(func(tx *store.Store) error {
		var err error
		if user, err = tx.UserByIdentity(settings.Issuer, claims.Subject); err != nil {
			return err
		}

		if user == nil {
			taken, err := tx.EmailTaken(claims.Email)
			if err != nil {
				return err
			}
			switch {
			case !taken:
				// An empty hash matches no password, so this account can only
				// sign in through the provider.
				user = &models.User{Email: claims.Email, Name: claims.Name, PasswordHash: "", Role: models.RoleUser}
				if err := tx.CreateUser(user); err != nil {
					return err
				}
			case !settings.LinkExisting:
				return oidcRefusal{"email_taken", claims.Email + " belongs to an unlinked account and link-existing is off"}
			default:
				if user, err = tx.UserByEmail(claims.Email); err != nil {
					return err
				}
			}
			if err := tx.CreateIdentity(&models.Identity{Issuer: settings.Issuer, Subject: claims.Subject, UserID: user.ID}); err != nil {
				return err
			}
		}

		if user.Disabled {
			return oidcRefusal{"denied", claims.Email + " is disabled"}
		}

		changes := map[string]any{}
		if claims.Email != user.Email {
			taken, err := tx.EmailTaken(claims.Email)
			if err != nil {
				return err
			}
			if !taken {
				changes["email"] = claims.Email
			}
		}

		if len(settings.AdminGroups) > 0 {
			role := models.RoleUser
			if inAnyGroup(claims.Groups, settings.AdminGroups) {
				role = models.RoleAdmin
			}
			// Demoting the last enabled administrator would lock everyone out of
			// managing the instance, so that one keeps the role.
			if user.IsAdmin() && role == models.RoleUser {
				admins, err := tx.CountEnabledAdmins()
				if err != nil {
					return err
				}
				if admins <= 1 {
					role = models.RoleAdmin
				}
			}
			changes["role"] = role
		}

		if len(changes) == 0 {
			return nil
		}
		_, err = tx.UpdateUser(user.ID, changes)
		return err
	})
	return user, err
}
