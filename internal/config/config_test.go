package config

import (
	"slices"
	"testing"
)

func TestNormalizeBaseURL(t *testing.T) {
	cases := []struct {
		raw  string
		want string
		fail bool
	}{
		{"", "", false},
		{"https://artifacts.example.com", "https://artifacts.example.com", false},
		{"https://artifacts.example.com/", "https://artifacts.example.com", false},
		{"http://host:8080/prefix/?x=1#y", "http://host:8080/prefix", false},
		{"artifacts.example.com", "", true},
		{"ftp://artifacts.example.com", "", true},
		{"https://user:pass@artifacts.example.com", "", true},
	}

	for _, testCase := range cases {
		got, err := NormalizeBaseURL(testCase.raw)
		if (err != nil) != testCase.fail {
			t.Errorf("%q: err = %v, want failure %v", testCase.raw, err, testCase.fail)
			continue
		}
		if got != testCase.want {
			t.Errorf("%q: got %q, want %q", testCase.raw, got, testCase.want)
		}
	}
}

func TestParseOIDC(t *testing.T) {
	if _, err := Parse([]string{"-oidc-issuer", "https://id.example.com"}); err == nil {
		t.Fatal("expected an issuer without client credentials to be rejected")
	}

	config, err := Parse([]string{
		"-oidc-issuer", "https://id.example.com",
		"-oidc-client-id", "arca",
		"-oidc-client-secret", "secret",
		"-oidc-admin-groups", " admins, ,ops ",
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	settings := config.OIDC
	if settings.Name != "SSO" || settings.GroupsClaim != "groups" || settings.LinkExisting {
		t.Fatalf("unexpected defaults: %+v", settings)
	}
	if !slices.Equal(settings.Scopes, []string{"openid", "profile", "email"}) {
		t.Fatalf("scopes = %q", settings.Scopes)
	}
	if !slices.Equal(settings.AdminGroups, []string{"admins", "ops"}) {
		t.Fatalf("admin groups = %q", settings.AdminGroups)
	}
	if len(settings.AllowedGroups) != 0 {
		t.Fatalf("allowed groups = %q", settings.AllowedGroups)
	}
}
