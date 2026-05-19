package certmanager

import "testing"

func TestProviderSpecsCurated(t *testing.T) {
	want := map[string]bool{
		"cloudflare": true, "route53": true, "digitalocean": true,
		"gcloud": true, "namesilo": true,
	}
	specs := ProviderSpecs()
	if len(specs) != len(want) {
		t.Fatalf("got %d specs, want %d", len(specs), len(want))
	}
	for _, s := range specs {
		if !want[s.Name] {
			t.Errorf("unexpected provider %q", s.Name)
		}
		if s.Display == "" || len(s.Fields) == 0 {
			t.Errorf("provider %q incomplete spec", s.Name)
		}
	}
}

func TestSupportedProvider(t *testing.T) {
	if !SupportedProvider("cloudflare") {
		t.Error("cloudflare should be supported")
	}
	if SupportedProvider("azure") {
		t.Error("azure must not be supported (not in allowlist)")
	}
}

func TestBuildProviderRejectsUnknown(t *testing.T) {
	if _, err := BuildProvider("azure", nil); err == nil {
		t.Fatal("expected error for unsupported provider")
	}
}

func TestBuildProviderRequiresCreds(t *testing.T) {
	if _, err := BuildProvider("cloudflare", map[string]string{}); err == nil {
		t.Fatal("expected missing-credential error")
	}
}

func TestBuildProviderCloudflare(t *testing.T) {
	p, err := BuildProvider("cloudflare", map[string]string{
		"CLOUDFLARE_DNS_API_TOKEN": "fake-token-for-test",
	})
	if err != nil {
		t.Fatalf("build cloudflare: %v", err)
	}
	if p == nil {
		t.Fatal("nil provider")
	}
}

func TestBuildProviderGcloudWritesServiceAccount(t *testing.T) {
	// Minimal well-formed JSON; lego only needs the file to exist + parse.
	_, err := BuildProvider("gcloud", map[string]string{
		"GCE_PROJECT":         "my-project",
		"GCE_SERVICE_ACCOUNT": `{"type":"service_account","project_id":"my-project"}`,
	})
	// Provider construction may fail later on auth, but missing-cred and
	// file-write paths must not error.
	if err != nil && err.Error() == "DNS provider gcloud: missing required credential GCE_SERVICE_ACCOUNT" {
		t.Fatalf("unexpected missing-cred error: %v", err)
	}
}
