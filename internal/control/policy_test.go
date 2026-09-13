package control

import "testing"

func TestTargetPolicyDefaultDeny(t *testing.T) {
	policy, err := ParseTargetPolicy("")
	if err != nil {
		t.Fatalf("ParseTargetPolicy() error = %v", err)
	}
	if err := policy.Authorize("http://10.250.0.10:8080"); err == nil {
		t.Fatal("Authorize() allowed target with empty policy")
	}
}

func TestTargetPolicyAllowsHostAndCIDR(t *testing.T) {
	policy, err := ParseTargetPolicy("app.loadtest.internal,10.250.0.0/24")
	if err != nil {
		t.Fatalf("ParseTargetPolicy() error = %v", err)
	}
	for _, target := range []string{
		"https://app.loadtest.internal/api/health",
		"http://10.250.0.10:8080/healthz",
	} {
		if err := policy.Authorize(target); err != nil {
			t.Fatalf("Authorize(%q) error = %v", target, err)
		}
	}
	if err := policy.Authorize("https://example.com"); err == nil {
		t.Fatal("Authorize() allowed unlisted hostname")
	}
}

func TestTargetPolicyRejectsCredentialsAndSchemes(t *testing.T) {
	policy, err := ParseTargetPolicy("example.com")
	if err != nil {
		t.Fatalf("ParseTargetPolicy() error = %v", err)
	}
	for _, target := range []string{
		"ftp://example.com/file",
		"https://user:pass@example.com/",
	} {
		if err := policy.Authorize(target); err == nil {
			t.Fatalf("Authorize(%q) unexpectedly succeeded", target)
		}
	}
}
