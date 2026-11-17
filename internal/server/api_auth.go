package server

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"arca/internal/auth"
	"arca/internal/format"
	"arca/internal/store"
	"arca/internal/store/models"
	"arca/internal/version"
)

var emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

const minimumPasswordLength = 10

type identity struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}

func identityOf(user *models.User) *identity {
	if user == nil {
		return nil
	}
	return &identity{ID: user.ID, Email: user.Email, Name: user.Name, Role: user.Role}
}

func validateEmail(value string) (string, error) {
	email, err := requireText(value, "email", 254)
	if err != nil {
		return "", err
	}
	email = strings.ToLower(email)
	if !emailPattern.MatchString(email) {
		return "", fail(http.StatusBadRequest, "email is not a valid address")
	}
	return email, nil
}

func validatePassword(value string) (string, error) {
	password, err := requireText(value, "password", 200)
	if err != nil {
		return "", err
	}
	if len(password) < minimumPasswordLength {
		return "", fail(http.StatusBadRequest, "password must be at least %d characters", minimumPasswordLength)
	}
	return password, nil
}

func (s *Server) routeBootstrap(writer http.ResponseWriter, request *http.Request) error {
	initialized, err := s.store.IsInitialized()
	if err != nil {
		return err
	}

	// Before setup runs there is nothing stored yet, so the wizard is handed the
	// default to start its welcome text from.
	welcomeText := s.store.WelcomeText()
	if !initialized {
		welcomeText = store.DefaultWelcomeText
	}

	var oidc any
	if s.oidc != nil {
		oidc = map[string]string{"name": s.config.OIDC.Name}
	}

	// Who the wizard's administrator step would be, once signed in through the
	// provider.
	var setupIdentity any
	if claims, ok := s.setupIdentity(request); !initialized && ok {
		setupIdentity = map[string]string{"name": claims.Name, "email": claims.Email}
	}

	writeJSON(writer, http.StatusOK, map[string]any{
		"initialized":   initialized,
		"instanceName":  s.store.InstanceName(),
		"themeColor":    s.store.ThemeColor(),
		"welcomeText":   welcomeText,
		"logoUrl":       logoURL(s.store.Logo()),
		"build":         version.Current(),
		"user":          identityOf(currentUser(request)),
		"oidc":          oidc,
		"setupIdentity": setupIdentity,
	})
	return nil
}

var seedRepositories = []struct {
	name        string
	format      string
	policy      string
	description string
}{
	{"maven-releases", format.Maven2, models.PolicyRelease, "Release artifacts"},
	{"maven-snapshots", format.Maven2, models.PolicySnapshot, "Snapshot artifacts"},
	{"npm-hosted", format.NPM, models.PolicyMixed, "npm packages"},
}

// routeSetup creates the first administrator. Three things keep it from being a
// free admin account for whoever finds the instance first: the route disappears
// once setup has run, the caller has to present the token printed to the server
// log, and the initialized marker is claimed inside the transaction with an
// insert that can only succeed once, so two wizards submitting at the same
// moment cannot both end up with an administrator.
func (s *Server) routeSetup(writer http.ResponseWriter, request *http.Request) error {
	initialized, err := s.store.IsInitialized()
	if err != nil {
		return err
	}
	if initialized {
		return fail(http.StatusNotFound, "Not found")
	}

	var body struct {
		SetupToken       string  `json:"setupToken"`
		InstanceName     string  `json:"instanceName"`
		ThemeColor       string  `json:"themeColor"`
		WelcomeText      *string `json:"welcomeText"`
		Name             string  `json:"name"`
		Email            string  `json:"email"`
		Password         string  `json:"password"`
		SeedRepositories *bool   `json:"seedRepositories"`
		PublicByDefault  bool    `json:"publicByDefault"`
	}
	if err := s.decodeBody(request, &body); err != nil {
		return err
	}

	presented := strings.TrimSpace(body.SetupToken)
	if presented == "" {
		presented = strings.TrimSpace(request.Header.Get("X-Setup-Token"))
	}
	if !s.setupTokenMatches(presented) {
		return fail(http.StatusUnauthorized, "The setup token is missing or wrong. It is printed in the server log at startup.")
	}

	instanceName, err := requireText(body.InstanceName, "instanceName", 80)
	if err != nil {
		return err
	}
	themeColor, err := validateThemeColor(orDefault(body.ThemeColor, store.DefaultThemeColor))
	if err != nil {
		return err
	}
	welcomeText := store.DefaultWelcomeText
	if body.WelcomeText != nil {
		welcomeText, err = validateWelcomeText(*body.WelcomeText)
		if err != nil {
			return err
		}
	}
	administrator := &models.User{ID: store.NewID(), Role: models.RoleAdmin}

	// An identity signed in through the provider becomes the administrator
	// instead of the name, email and password.
	claims, viaOIDC := s.setupIdentity(request)
	if viaOIDC {
		// The wizard sends back the email it showed, so signing in as someone
		// else in another tab cannot swap the administrator under it.
		if body.Email != claims.Email {
			return fail(http.StatusConflict, "The signed-in identity has changed. Reload the page and check who it shows.")
		}
		administrator.Name, administrator.Email = claims.Name, claims.Email
	} else {
		if administrator.Name, err = requireText(body.Name, "name", 120); err != nil {
			return err
		}
		if administrator.Email, err = validateEmail(body.Email); err != nil {
			return err
		}
		password, err := validatePassword(body.Password)
		if err != nil {
			return err
		}
		if administrator.PasswordHash, err = auth.HashPassword(password); err != nil {
			return err
		}
	}

	visibility := models.VisibilityPrivate
	if body.PublicByDefault {
		visibility = models.VisibilityPublic
	}

	err = s.store.Transaction(func(tx *store.Store) error {
		// Claimed first, so a losing race fails before it has written anything.
		if err := tx.ClaimInitialization(); err != nil {
			return err
		}
		if err := tx.CreateUser(administrator); err != nil {
			return err
		}
		if viaOIDC {
			if err := tx.CreateIdentity(&models.Identity{Issuer: s.config.OIDC.Issuer, Subject: claims.Subject, UserID: administrator.ID}); err != nil {
				return err
			}
		}
		if err := tx.SetSetting(store.SettingInstanceName, instanceName); err != nil {
			return err
		}
		if err := tx.SetSetting(store.SettingThemeColor, themeColor); err != nil {
			return err
		}
		if err := tx.SetSetting(store.SettingWelcomeText, welcomeText); err != nil {
			return err
		}
		if body.SeedRepositories != nil && !*body.SeedRepositories {
			return nil
		}
		for _, seed := range seedRepositories {
			repository := &models.Repository{
				Name:          seed.name,
				Format:        seed.format,
				Policy:        seed.policy,
				Visibility:    visibility,
				Description:   seed.description,
				AllowRedeploy: seed.policy == models.PolicySnapshot,
			}
			if err := tx.CreateRepository(repository); err != nil {
				return err
			}
		}
		return nil
	})
	if errors.Is(err, store.ErrAlreadyInitialized) {
		return fail(http.StatusNotFound, "Not found")
	}
	if err != nil {
		return err
	}
	// The token has done its job; forgetting it closes the route for good
	// without another database read.
	s.setupToken = ""
	s.setupIdentities.Clear()
	setSetupIdentityCookie(writer, request, "", -1)

	if err := s.startSession(writer, request, administrator.ID); err != nil {
		return err
	}

	writeJSON(writer, http.StatusCreated, identityOf(administrator))
	return nil
}

func (s *Server) setupTokenMatches(presented string) bool {
	expected := s.setupToken
	if expected == "" || presented == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(presented)) == 1
}

func (s *Server) startSession(writer http.ResponseWriter, request *http.Request, userID string) error {
	token, err := auth.CreateSession(s.store, userID)
	if err != nil {
		return err
	}
	auth.AttachSessionCookie(writer, request, token)
	return nil
}

func (s *Server) routeLogin(writer http.ResponseWriter, request *http.Request) error {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := s.decodeBody(request, &body); err != nil {
		return err
	}

	email, err := requireText(body.Email, "email", 254)
	if err != nil {
		return err
	}
	password, err := requireText(body.Password, "password", 200)
	if err != nil {
		return err
	}

	user, err := s.resolver.Authenticate(auth.ClientAddress(request), strings.ToLower(email), password)
	if err != nil {
		return authenticationFailure(writer, err)
	}

	if err := s.startSession(writer, request, user.ID); err != nil {
		return err
	}

	writeJSON(writer, http.StatusOK, identityOf(user))
	return nil
}

// authenticationFailure turns a refused password into the response for it. A
// throttled client is told how long to wait; every other failure gets the same
// words, so the response never says which half of the pair was wrong.
func authenticationFailure(writer http.ResponseWriter, err error) error {
	var throttled auth.ThrottledError
	if errors.As(err, &throttled) {
		writer.Header().Set("Retry-After", strconv.Itoa(int(throttled.RetryAfter.Seconds())+1))
		return fail(http.StatusTooManyRequests, "Too many failed attempts, try again later")
	}
	return fail(http.StatusUnauthorized, "Invalid email or password")
}

func (s *Server) routeLogout(writer http.ResponseWriter, request *http.Request) error {
	if token := auth.SessionToken(request); token != "" {
		if err := auth.DestroySession(s.store, token); err != nil {
			return err
		}
	}
	auth.ClearSessionCookie(writer, request)
	return noContent(writer)
}

func (s *Server) routeMe(writer http.ResponseWriter, request *http.Request) error {
	user, err := requireUser(request)
	if err != nil {
		return err
	}
	writeJSON(writer, http.StatusOK, identityOf(user))
	return nil
}

func (s *Server) routeChangePassword(writer http.ResponseWriter, request *http.Request) error {
	user, err := requireUser(request)
	if err != nil {
		return err
	}

	var body struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := s.decodeBody(request, &body); err != nil {
		return err
	}

	current, err := requireText(body.CurrentPassword, "currentPassword", 200)
	if err != nil {
		return err
	}
	next, err := validatePassword(body.NewPassword)
	if err != nil {
		return err
	}

	// Confirming the current password is a password check like any other, so
	// it draws on the same failure budget as a login would.
	address := auth.ClientAddress(request)
	if ok, wait := s.resolver.Guard().Allow(address, user.Email); !ok {
		writer.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		return fail(http.StatusTooManyRequests, "Too many failed attempts, try again later")
	}
	fresh, err := s.store.UserByID(user.ID)
	if err != nil {
		return err
	}
	if !auth.VerifyPassword(current, fresh.PasswordHash) {
		s.resolver.Guard().Fail(address, user.Email)
		return fail(http.StatusUnauthorized, "Current password is incorrect")
	}

	hash, err := auth.HashPassword(next)
	if err != nil {
		return err
	}
	if _, err := s.store.UpdateUser(user.ID, map[string]any{"password_hash": hash}); err != nil {
		return err
	}
	if err := s.store.DeleteUserSessions(user.ID); err != nil {
		return err
	}
	if err := s.startSession(writer, request, user.ID); err != nil {
		return err
	}

	return noContent(writer)
}
