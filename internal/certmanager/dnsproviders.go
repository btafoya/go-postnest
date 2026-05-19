package certmanager

import (
	"fmt"
	"os"
	"sort"

	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/providers/dns/cloudflare"
	"github.com/go-acme/lego/v4/providers/dns/digitalocean"
	"github.com/go-acme/lego/v4/providers/dns/gcloud"
	"github.com/go-acme/lego/v4/providers/dns/namesilo"
	"github.com/go-acme/lego/v4/providers/dns/route53"
)

// CredField describes one credential input for a DNS provider.
type CredField struct {
	Key      string `json:"key"`      // env var lego reads
	Label    string `json:"label"`    // UI label
	Secret   bool   `json:"secret"`   // render as password / mask on read
	Required bool   `json:"required"` // must be set for the provider
	Textarea bool   `json:"textarea"` // multi-line input (e.g. JSON blobs)
}

// ProviderSpec is the curated, typed config for a supported DNS provider.
type ProviderSpec struct {
	Name    string      `json:"name"`
	Display string      `json:"display"`
	Fields  []CredField `json:"fields"`
}

// providerSpecs is the curated allowlist. Adding a provider here is the only
// supported way to expand DNS support; lego ships ~100 more but they are not
// exposed without explicit, validated config.
var providerSpecs = map[string]ProviderSpec{
	"cloudflare": {
		Name:    "cloudflare",
		Display: "Cloudflare",
		Fields: []CredField{
			{Key: "CLOUDFLARE_DNS_API_TOKEN", Label: "API Token", Secret: true, Required: true},
		},
	},
	"route53": {
		Name:    "route53",
		Display: "AWS Route 53",
		Fields: []CredField{
			{Key: "AWS_ACCESS_KEY_ID", Label: "Access Key ID", Required: true},
			{Key: "AWS_SECRET_ACCESS_KEY", Label: "Secret Access Key", Secret: true, Required: true},
			{Key: "AWS_REGION", Label: "Region", Required: true},
			{Key: "AWS_HOSTED_ZONE_ID", Label: "Hosted Zone ID", Required: false},
		},
	},
	"digitalocean": {
		Name:    "digitalocean",
		Display: "DigitalOcean",
		Fields: []CredField{
			{Key: "DO_AUTH_TOKEN", Label: "Auth Token", Secret: true, Required: true},
		},
	},
	"gcloud": {
		Name:    "gcloud",
		Display: "Google Cloud DNS",
		Fields: []CredField{
			{Key: "GCE_PROJECT", Label: "GCP Project ID", Required: true},
			{Key: "GCE_SERVICE_ACCOUNT", Label: "Service Account JSON", Secret: true, Required: true, Textarea: true},
		},
	},
	"namesilo": {
		Name:    "namesilo",
		Display: "Namesilo",
		Fields: []CredField{
			{Key: "NAMESILO_API_KEY", Label: "API Key", Secret: true, Required: true},
		},
	},
}

// ProviderSpecs returns all supported provider specs sorted by name.
func ProviderSpecs() []ProviderSpec {
	out := make([]ProviderSpec, 0, len(providerSpecs))
	for _, s := range providerSpecs {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// SupportedProvider reports whether name is in the curated allowlist.
func SupportedProvider(name string) bool {
	_, ok := providerSpecs[name]
	return ok
}

// BuildProvider constructs a lego challenge.Provider for the named provider
// using decrypted credentials. The gcloud provider requires its service
// account JSON on disk, so it is written to a private temp file whose path is
// fed to lego via GCE_SERVICE_ACCOUNT.
func BuildProvider(name string, creds map[string]string) (challenge.Provider, error) {
	spec, ok := providerSpecs[name]
	if !ok {
		return nil, fmt.Errorf("unsupported DNS provider: %s", name)
	}

	for _, f := range spec.Fields {
		if f.Required && creds[f.Key] == "" {
			return nil, fmt.Errorf("DNS provider %s: missing required credential %s", name, f.Key)
		}
	}

	switch name {
	case "cloudflare":
		setEnv(spec, creds)
		return cloudflare.NewDNSProvider()
	case "route53":
		setEnv(spec, creds)
		return route53.NewDNSProvider()
	case "digitalocean":
		setEnv(spec, creds)
		return digitalocean.NewDNSProvider()
	case "namesilo":
		setEnv(spec, creds)
		return namesilo.NewDNSProvider()
	case "gcloud":
		path, err := writeServiceAccount(creds["GCE_SERVICE_ACCOUNT"])
		if err != nil {
			return nil, err
		}
		_ = os.Setenv("GCE_PROJECT", creds["GCE_PROJECT"])
		_ = os.Setenv("GCE_SERVICE_ACCOUNT", path)
		return gcloud.NewDNSProvider()
	default:
		return nil, fmt.Errorf("unsupported DNS provider: %s", name)
	}
}

func setEnv(spec ProviderSpec, creds map[string]string) {
	for _, f := range spec.Fields {
		if v := creds[f.Key]; v != "" {
			_ = os.Setenv(f.Key, v)
		}
	}
}

func writeServiceAccount(jsonBlob string) (string, error) {
	f, err := os.CreateTemp("", "postnest-gcloud-*.json")
	if err != nil {
		return "", fmt.Errorf("gcloud: temp service account file: %w", err)
	}
	if _, err := f.WriteString(jsonBlob); err != nil {
		f.Close()
		return "", fmt.Errorf("gcloud: write service account: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(f.Name(), 0600); err != nil {
		return "", err
	}
	return f.Name(), nil
}
