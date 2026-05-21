package autodiscover

import (
	"bytes"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOutlookResponseTemplate(t *testing.T) {
	data := &autodiscoverData{
		Email:       "test@example.com",
		DisplayName: "Test User",
		IMAPHost:    "imap.example.com",
		SMTPHost:    "smtp.example.com",
	}

	var buf bytes.Buffer
	if err := outlookResponseTemplate.Execute(&buf, data); err != nil {
		t.Fatalf("template execution failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, `<DisplayName>Test User</DisplayName>`) {
		t.Error("missing display name")
	}
	if !strings.Contains(out, `<Server>imap.example.com</Server>`) {
		t.Error("missing IMAP server")
	}
	if !strings.Contains(out, `<Server>smtp.example.com</Server>`) {
		t.Error("missing SMTP server")
	}
	if !strings.Contains(out, `<Port>993</Port>`) {
		t.Error("missing IMAP port")
	}
	if !strings.Contains(out, `<Port>465</Port>`) {
		t.Error("missing SMTP port")
	}

	// Verify valid XML
	var v struct {
		Response struct {
			Account struct {
				Protocol []struct {
					Type   string `xml:"Type"`
					Server string `xml:"Server"`
					Port   string `xml:"Port"`
					SSL    string `xml:"SSL"`
				} `xml:"Protocol"`
			} `xml:"Account"`
		} `xml:"Response"`
	}
	if err := xml.Unmarshal(buf.Bytes(), &v); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}
	if len(v.Response.Account.Protocol) != 2 {
		t.Errorf("expected 2 protocols, got %d", len(v.Response.Account.Protocol))
	}
}

func TestThunderbirdTemplate(t *testing.T) {
	data := &autodiscoverData{
		Email:       "test@example.com",
		DisplayName: "Test User",
		Domain:      "example.com",
		Host:        "mail.example.com",
		IMAPHost:    "imap.example.com",
		SMTPHost:    "smtp.example.com",
	}

	var buf bytes.Buffer
	if err := thunderbirdTemplate.Execute(&buf, data); err != nil {
		t.Fatalf("template execution failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, `id="example.com"`) {
		t.Error("missing domain id")
	}
	if !strings.Contains(out, `hostname>imap.example.com</hostname>`) {
		t.Error("missing IMAP host")
	}
	if !strings.Contains(out, `hostname>smtp.example.com</hostname>`) {
		t.Error("missing SMTP host")
	}
	if !strings.Contains(out, `.well-known/carddav`) {
		t.Error("missing well-known carddav")
	}
	if !strings.Contains(out, `.well-known/caldav`) {
		t.Error("missing well-known caldav")
	}
}

func TestAppleTemplate(t *testing.T) {
	data := &autodiscoverData{
		Email:            "test@example.com",
		DisplayName:      "Test User",
		Host:             "mail.example.com",
		IMAPHost:         "imap.example.com",
		SMTPHost:         "smtp.example.com",
		UUID:             "uuid-1",
		DAVUUID:          "uuid-2",
		ProfileUUID:      "uuid-3",
		DAVWellKnownUUID: "uuid-4",
	}

	var buf bytes.Buffer
	if err := appleTemplate.Execute(&buf, data); err != nil {
		t.Fatalf("template execution failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, `<key>EmailAddress</key>`) {
		t.Error("missing email address key")
	}
	if !strings.Contains(out, `imap.example.com`) {
		t.Error("missing IMAP host")
	}
	if !strings.Contains(out, `smtp.example.com`) {
		t.Error("missing SMTP host")
	}
	if !strings.Contains(out, `Contacts &amp; Calendars`) {
		t.Error("missing contacts & calendars display name")
	}
	if !strings.Contains(out, `.well-known/caldav`) {
		t.Error("missing well-known caldav")
	}
	if !strings.Contains(out, `com.apple.caldav.account`) {
		t.Error("missing caldav account payload type")
	}
}

func TestWriteOutlookError(t *testing.T) {
	w := httptest.NewRecorder()
	h := &Handler{}
	h.writeOutlookError(w, "Invalid Request")

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/xml") {
		t.Errorf("expected xml content type, got %s", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, `<ErrorCode>600</ErrorCode>`) {
		t.Error("missing error code 600")
	}
	if !strings.Contains(body, `Invalid Request`) {
		t.Error("missing error message")
	}
}
