package auth

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"arca/internal/config"
)

// oidcTimeout bounds each conversation with the provider, so one that hangs
// fails the sign-in instead of holding the request, and the discovery lock,
// for as long as the browser waits.
const oidcTimeout = 15 * time.Second

// OIDC signs users in through an OpenID Connect provider. Discovery waits for
// the first sign-in and is cached only once it succeeds, so a provider that is
// down neither blocks startup nor stays broken after it comes back.
type OIDC struct {
	settings config.OIDC

	mutex    sync.Mutex
	provider *oidc.Provider
}

// OIDCClaims is what a verified ID token says about the user.
type OIDCClaims struct {
	Subject string
	Email   string
	Name    string
	Groups  []string
}

func NewOIDC(settings config.OIDC) *OIDC {
	return &OIDC{settings: settings}
}

func (o *OIDC) discover(ctx context.Context) (*oidc.Provider, error) {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	if o.provider == nil {
		provider, err := oidc.NewProvider(ctx, o.settings.Issuer)
		if err != nil {
			return nil, fmt.Errorf("discover %s: %w", o.settings.Issuer, err)
		}
		o.provider = provider
	}
	return o.provider, nil
}

func (o *OIDC) client(provider *oidc.Provider, redirectURI string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     o.settings.ClientID,
		ClientSecret: o.settings.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  redirectURI,
		Scopes:       o.settings.Scopes,
	}
}

// AuthURL is where the browser is sent to sign in. The state, nonce and PKCE
// verifier have to be kept by the caller and handed back to Exchange.
func (o *OIDC) AuthURL(ctx context.Context, redirectURI, state, nonce, verifier string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, oidcTimeout)
	defer cancel()

	provider, err := o.discover(ctx)
	if err != nil {
		return "", err
	}
	return o.client(provider, redirectURI).AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), nil
}

// Exchange redeems the code the provider redirected back with and returns the
// claims of the ID token, once its signature, issuer, audience, expiry and
// nonce have been checked.
func (o *OIDC) Exchange(ctx context.Context, redirectURI, code, verifier, nonce string) (OIDCClaims, error) {
	ctx, cancel := context.WithTimeout(ctx, oidcTimeout)
	defer cancel()

	provider, err := o.discover(ctx)
	if err != nil {
		return OIDCClaims{}, err
	}

	token, err := o.client(provider, redirectURI).Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return OIDCClaims{}, fmt.Errorf("exchange code: %w", err)
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		return OIDCClaims{}, errors.New("token response carries no id_token")
	}

	idToken, err := provider.Verifier(&oidc.Config{ClientID: o.settings.ClientID}).Verify(ctx, raw)
	if err != nil {
		return OIDCClaims{}, fmt.Errorf("verify id_token: %w", err)
	}
	if idToken.Nonce != nonce {
		return OIDCClaims{}, errors.New("id_token nonce does not match")
	}

	// The subject is what accounts are matched on, and the verifier lets an
	// empty one through: everyone carrying it would share one account.
	if idToken.Subject == "" {
		return OIDCClaims{}, errors.New("id_token carries no subject")
	}

	var fields struct {
		Email             string `json:"email"`
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
	}
	var all map[string]any
	if err := idToken.Claims(&fields); err != nil {
		return OIDCClaims{}, err
	}
	if err := idToken.Claims(&all); err != nil {
		return OIDCClaims{}, err
	}

	email := strings.ToLower(strings.TrimSpace(fields.Email))
	if email == "" {
		return OIDCClaims{}, errors.New("id_token carries no email")
	}
	name := cmp.Or(strings.TrimSpace(fields.Name), strings.TrimSpace(fields.PreferredUsername), email)

	return OIDCClaims{
		Subject: idToken.Subject,
		Email:   email,
		Name:    name,
		Groups:  stringList(all[o.settings.GroupsClaim]),
	}, nil
}

// stringList reads a groups claim, which providers send either as an array of
// strings or, with a single group, as a plain string.
func stringList(value any) []string {
	switch typed := value.(type) {
	case string:
		return []string{typed}
	case []any:
		var entries []string
		for _, entry := range typed {
			if text, ok := entry.(string); ok {
				entries = append(entries, text)
			}
		}
		return entries
	}
	return nil
}
