package config

import (
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultMaxUploadBytes = 256 << 20
	DefaultProxyTimeout   = 60 * time.Second
)

type Config struct {
	DevMode      bool
	Port         int
	DatabasePath string
	StorageDir   string

	MaxUploadBytes int64
	// MaxBlobBytes limits a container layer, which routinely runs past whatever
	// is a sane ceiling for a jar or a tarball. Zero means unlimited.
	MaxBlobBytes int64

	ProxyTimeout time.Duration
	ViteURL      string

	// BaseURL is the address clients reach this server at, used wherever a
	// response has to carry an absolute URL back. Empty derives it from the
	// request, which trusts whatever X-Forwarded-Host the proxy in front passed on.
	BaseURL string
	// SetupToken gates the first-run wizard. Empty generates one at startup and
	// prints it to the log.
	SetupToken string

	OIDC OIDC
}

// OIDC configures single sign-on through an OpenID Connect provider. An empty
// Issuer leaves it off.
type OIDC struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	// Name labels the sign-in button.
	Name        string
	Scopes      []string
	GroupsClaim string
	// AdminGroups, when set, hands the IdP control over who is an administrator.
	AdminGroups []string
	// AllowedGroups, when set, turns away anyone in none of them.
	AllowedGroups []string
	// LinkExisting lets a first sign-in claim an account that already has the
	// same email, which trusts the IdP to have verified that address.
	LinkExisting bool
}

func Parse(arguments []string) (Config, error) {
	flags := flag.NewFlagSet("arca", flag.ContinueOnError)

	config := Config{}
	var proxyTimeoutSeconds int64
	var oidcScopes, oidcAdminGroups, oidcAllowedGroups string

	flags.BoolVar(&config.DevMode, "dev", envBool("DEV_MODE", false), "proxy the frontend to a running vite dev server instead of serving the embedded bundle")
	flags.Int64Var(&proxyTimeoutSeconds, "proxy-timeout", envInt64("PROXY_TIMEOUT_SECONDS", int64(DefaultProxyTimeout/time.Second)), "how long a proxy repository waits for its upstream, in seconds")
	flags.IntVar(&config.Port, "port", envInt("PORT", 8080), "port to listen on")
	flags.StringVar(&config.DatabasePath, "db", envString("DATABASE_PATH", "./data/arca.db"), "path to the SQLite database file")
	flags.StringVar(&config.StorageDir, "storage", envString("STORAGE_DIR", "./data/storage"), "directory holding uploaded artifacts")
	flags.Int64Var(&config.MaxUploadBytes, "max-upload", envInt64("MAX_UPLOAD_BYTES", DefaultMaxUploadBytes), "largest artifact accepted, in bytes")
	flags.Int64Var(&config.MaxBlobBytes, "max-blob", envInt64("MAX_BLOB_BYTES", 0), "largest container image layer accepted, in bytes, or 0 for no limit")
	flags.StringVar(&config.ViteURL, "vite-url", envString("VITE_URL", "http://localhost:5173"), "vite dev server to proxy in dev mode")
	flags.StringVar(&config.BaseURL, "base-url", envString("BASE_URL", ""), "public URL this server is reached at, such as https://artifacts.example.com; derived from each request when empty")
	flags.StringVar(&config.SetupToken, "setup-token", envString("SETUP_TOKEN", ""), "token the first-run setup wizard asks for; generated and logged when empty")

	flags.StringVar(&config.OIDC.Issuer, "oidc-issuer", envString("OIDC_ISSUER", ""), "OpenID Connect issuer URL; enables single sign-on when set")
	flags.StringVar(&config.OIDC.ClientID, "oidc-client-id", envString("OIDC_CLIENT_ID", ""), "OpenID Connect client id")
	flags.StringVar(&config.OIDC.ClientSecret, "oidc-client-secret", envString("OIDC_CLIENT_SECRET", ""), "OpenID Connect client secret")
	flags.StringVar(&config.OIDC.Name, "oidc-name", envString("OIDC_NAME", "SSO"), "provider name shown on the sign-in button")
	flags.StringVar(&oidcScopes, "oidc-scopes", envString("OIDC_SCOPES", "openid profile email"), "space-separated scopes to request")
	flags.StringVar(&config.OIDC.GroupsClaim, "oidc-groups-claim", envString("OIDC_GROUPS_CLAIM", "groups"), "ID token claim that lists the user's groups")
	flags.StringVar(&oidcAdminGroups, "oidc-admin-groups", envString("OIDC_ADMIN_GROUPS", ""), "comma-separated groups whose members are administrators; empty leaves roles alone")
	flags.StringVar(&oidcAllowedGroups, "oidc-allowed-groups", envString("OIDC_ALLOWED_GROUPS", ""), "comma-separated groups allowed to sign in; empty allows everyone")
	flags.BoolVar(&config.OIDC.LinkExisting, "oidc-link-existing", envBool("OIDC_LINK_EXISTING", false), "link a first sign-in to an existing account with the same email")

	if err := flags.Parse(arguments); err != nil {
		return Config{}, err
	}
	config.OIDC.Scopes = strings.Fields(oidcScopes)
	config.OIDC.AdminGroups = commaList(oidcAdminGroups)
	config.OIDC.AllowedGroups = commaList(oidcAllowedGroups)
	if config.OIDC.Issuer != "" && (config.OIDC.ClientID == "" || config.OIDC.ClientSecret == "") {
		err := errors.New("oidc-issuer requires oidc-client-id and oidc-client-secret")
		fmt.Fprintln(flags.Output(), err)
		return Config{}, err
	}
	if config.MaxUploadBytes <= 0 {
		config.MaxUploadBytes = DefaultMaxUploadBytes
	}
	if config.MaxBlobBytes < 0 {
		config.MaxBlobBytes = 0
	}

	config.ProxyTimeout = time.Duration(proxyTimeoutSeconds) * time.Second
	if config.ProxyTimeout <= 0 {
		config.ProxyTimeout = DefaultProxyTimeout
	}

	baseURL, err := NormalizeBaseURL(config.BaseURL)
	if err != nil {
		fmt.Fprintln(flags.Output(), err)
		return Config{}, err
	}
	config.BaseURL = baseURL

	return config, nil
}

// NormalizeBaseURL accepts an absolute http or https URL and strips what a base
// must not carry: a query, a fragment, and a trailing slash. Empty stays empty.
func NormalizeBaseURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", errors.New("base-url must be an absolute http or https URL")
	}
	if parsed.User != nil {
		return "", errors.New("base-url must not carry credentials")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	parsed.RawPath = ""

	return parsed.String(), nil
}

// commaList splits a comma-separated value, dropping blanks around and between
// entries.
func commaList(value string) []string {
	var entries []string
	for _, entry := range strings.Split(value, ",") {
		if entry = strings.TrimSpace(entry); entry != "" {
			entries = append(entries, entry)
		}
	}
	return entries
}

func envString(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	return int(envInt64(key, int64(fallback)))
}

func envInt64(key string, fallback int64) int64 {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBool(key string, fallback bool) bool {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}
