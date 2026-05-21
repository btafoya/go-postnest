package autodiscover

import (
	"html/template"
	"net/http"
)

var thunderbirdTemplate = template.Must(template.New("thunderbird").Parse(`<?xml version="1.0"?>
<clientConfig version="1.1">
  <emailProvider id="{{.Domain}}">
    <domain>{{.Domain}}</domain>
    <displayName>{{.DisplayName}}</displayName>
    <incomingServer type="imap">
      <hostname>{{.IMAPHost}}</hostname>
      <port>993</port>
      <socketType>SSL</socketType>
      <username>%EMAILADDRESS%</username>
      <authentication>password-cleartext</authentication>
    </incomingServer>
    <outgoingServer type="smtp">
      <hostname>{{.SMTPHost}}</hostname>
      <port>465</port>
      <socketType>SSL</socketType>
      <username>%EMAILADDRESS%</username>
      <authentication>password-cleartext</authentication>
      <addThisServer>true</addThisServer>
      <useGlobalPreferredServer>true</useGlobalPreferredServer>
    </outgoingServer>
    <addressBook type="carddav">
      <username>%EMAILADDRESS%</username>
      <authentication>password-cleartext</authentication>
      <serverURL>https://{{.Host}}/dav/addressbooks/%EMAILADDRESS%/default/</serverURL>
    </addressBook>
    <calendar type="caldav">
      <username>%EMAILADDRESS%</username>
      <authentication>password-cleartext</authentication>
      <serverURL>https://{{.Host}}/dav/calendars/%EMAILADDRESS%/default/</serverURL>
    </calendar>
    <addressBook type="carddav">
      <username>%EMAILADDRESS%</username>
      <authentication>password-cleartext</authentication>
      <serverURL>https://{{.Host}}/.well-known/carddav</serverURL>
    </addressBook>
    <calendar type="caldav">
      <username>%EMAILADDRESS%</username>
      <authentication>password-cleartext</authentication>
      <serverURL>https://{{.Host}}/.well-known/caldav</serverURL>
    </calendar>
  </emailProvider>
</clientConfig>
`))

func (h *Handler) thunderbirdAutoconfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	email := r.URL.Query().Get("emailaddress")
	if email == "" {
		http.NotFound(w, r)
		return
	}

	data, err := h.buildData(ctx, r, email)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = thunderbirdTemplate.Execute(w, data)
}
