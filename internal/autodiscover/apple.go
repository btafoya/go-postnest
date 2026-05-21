package autodiscover

import (
	"bytes"
	"crypto/x509"
	"errors"
	"html/template"
	"net/http"

	"github.com/smallstep/pkcs7"
)

var appleTemplate = template.Must(template.New("apple").Parse(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>PayloadContent</key>
  <array>
    <dict>
      <key>PayloadType</key>
      <string>com.apple.mail.managed</string>
      <key>PayloadVersion</key>
      <integer>1</integer>
      <key>PayloadIdentifier</key>
      <string>com.postnest.mail.{{.Email}}</string>
      <key>PayloadUUID</key>
      <string>{{.UUID}}</string>
      <key>PayloadDisplayName</key>
      <string>{{.DisplayName}}</string>
      <key>EmailAccountType</key>
      <string>EmailTypeIMAP</string>
      <key>EmailAddress</key>
      <string>{{.Email}}</string>
      <key>EmailAccountName</key>
      <string>{{.DisplayName}}</string>
      <key>EmailAccountDescription</key>
      <string>{{.Host}} Mail</string>
      <key>IncomingMailServerHostName</key>
      <string>{{.IMAPHost}}</string>
      <key>IncomingMailServerPortNumber</key>
      <integer>993</integer>
      <key>IncomingMailServerUseSSL</key>
      <true/>
      <key>IncomingMailServerAuthentication</key>
      <string>EmailAuthPassword</string>
      <key>IncomingMailServerUsername</key>
      <string>{{.Email}}</string>
      <key>OutgoingMailServerHostName</key>
      <string>{{.SMTPHost}}</string>
      <key>OutgoingMailServerPortNumber</key>
      <integer>465</integer>
      <key>OutgoingMailServerUseSSL</key>
      <true/>
      <key>OutgoingMailServerAuthentication</key>
      <string>EmailAuthPassword</string>
      <key>OutgoingMailServerUsername</key>
      <string>{{.Email}}</string>
      <key>OutgoingPasswordSameAsIncomingPassword</key>
      <true/>
    </dict>
    <dict>
      <key>PayloadType</key>
      <string>com.apple.mail.managed</string>
      <key>PayloadVersion</key>
      <integer>1</integer>
      <key>PayloadIdentifier</key>
      <string>com.postnest.dav.{{.Email}}</string>
      <key>PayloadUUID</key>
      <string>{{.DAVUUID}}</string>
      <key>PayloadDisplayName</key>
      <string>Contacts &amp; Calendars</string>
      <key>CalDAVAccountDescription</key>
      <string>{{.Host}} Calendar</string>
      <key>CalDAVHostName</key>
      <string>{{.Host}}</string>
      <key>CalDAVPort</key>
      <integer>443</integer>
      <key>CalDAVUseSSL</key>
      <true/>
      <key>CalDAVPrincipalURL</key>
      <string>https://{{.Host}}/dav/calendars/{{.Email}}/</string>
      <key>CalDAVUsername</key>
      <string>{{.Email}}</string>
    </dict>
    <dict>
      <key>PayloadType</key>
      <string>com.apple.caldav.account</string>
      <key>PayloadVersion</key>
      <integer>1</integer>
      <key>PayloadIdentifier</key>
      <string>com.postnest.dav.wellknown.{{.Email}}</string>
      <key>PayloadUUID</key>
      <string>{{.DAVWellKnownUUID}}</string>
      <key>PayloadDisplayName</key>
      <string>CalDAV Well-Known</string>
      <key>CalDAVAccountDescription</key>
      <string>{{.Host}} Calendar (Well-Known)</string>
      <key>CalDAVHostName</key>
      <string>{{.Host}}</string>
      <key>CalDAVPort</key>
      <integer>443</integer>
      <key>CalDAVUseSSL</key>
      <true/>
      <key>CalDAVPrincipalURL</key>
      <string>https://{{.Host}}/.well-known/caldav</string>
      <key>CalDAVUsername</key>
      <string>{{.Email}}</string>
    </dict>
  </array>
  <key>PayloadType</key>
  <string>Configuration</string>
  <key>PayloadVersion</key>
  <integer>1</integer>
  <key>PayloadIdentifier</key>
  <string>com.postnest.autodiscover</string>
  <key>PayloadUUID</key>
  <string>{{.ProfileUUID}}</string>
  <key>PayloadDisplayName</key>
  <string>{{.Host}} Mail Configuration</string>
</dict>
</plist>
`))

func (h *Handler) appleMobileConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	email := r.URL.Query().Get("emailaddress")
	if email == "" {
		email = r.URL.Query().Get("email")
	}
	if email == "" {
		http.Error(w, "Configuration not found.", http.StatusNotFound)
		return
	}

	data, err := h.buildData(ctx, r, email)
	if err != nil {
		http.Error(w, "Configuration not found.", http.StatusNotFound)
		return
	}

	var buf bytes.Buffer
	if err := appleTemplate.Execute(&buf, data); err != nil {
		h.log.Error("failed to render mobileconfig", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	plist := buf.Bytes()

	// Try to sign with TLS certificate
	if h.certManager != nil {
		if signed, err := h.signMobileConfig(plist); err == nil {
			w.Header().Set("Content-Type", "application/x-apple-aspen-config")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(signed)
			return
		} else {
			h.log.Warn("mobileconfig signing failed, returning unsigned", "error", err)
		}
	}

	w.Header().Set("Content-Type", "application/x-apple-aspen-config+xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(plist)
}

func (h *Handler) signMobileConfig(plist []byte) ([]byte, error) {
	cert, err := h.certManager.GetCertificate(nil)
	if err != nil {
		return nil, err
	}
	if cert == nil || len(cert.Certificate) == 0 {
		return nil, errors.New("no certificate available")
	}

	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return nil, err
	}

	p7, err := pkcs7.NewSignedData(plist)
	if err != nil {
		return nil, err
	}

	err = p7.AddSigner(x509Cert, cert.PrivateKey, pkcs7.SignerInfoConfig{})
	if err != nil {
		return nil, err
	}

	return p7.Finish()
}
