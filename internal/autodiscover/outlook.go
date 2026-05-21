package autodiscover

import (
	"encoding/xml"
	"html/template"
	"net/http"
	"time"
)

type outlookRequest struct {
	XMLName xml.Name `xml:"Autodiscover"`
	Request struct {
		EMailAddress string `xml:"EMailAddress"`
	} `xml:"Request"`
}

var outlookResponseTemplate = template.Must(template.New("outlook").Parse(`<?xml version="1.0" encoding="utf-8"?>
<Autodiscover xmlns="http://schemas.microsoft.com/exchange/autodiscover/responseschema/2006">
  <Response xmlns="http://schemas.microsoft.com/exchange/autodiscover/outlook/responseschema/2006a">
    <User>
      <DisplayName>{{.DisplayName}}</DisplayName>
      <AutoDiscoverSMTPAddress>{{.Email}}</AutoDiscoverSMTPAddress>
    </User>
    <Account>
      <AccountType>email</AccountType>
      <Action>settings</Action>
      <Protocol>
        <Type>IMAP</Type>
        <Server>{{.IMAPHost}}</Server>
        <Port>993</Port>
        <SSL>On</SSL>
        <AuthRequired>on</AuthRequired>
        <SPA>off</SPA>
      </Protocol>
      <Protocol>
        <Type>SMTP</Type>
        <Server>{{.SMTPHost}}</Server>
        <Port>465</Port>
        <SSL>On</SSL>
        <AuthRequired>on</AuthRequired>
        <SPA>off</SPA>
      </Protocol>
    </Account>
  </Response>
</Autodiscover>
`))

func (h *Handler) outlookAutodiscover(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req outlookRequest
	if err := xml.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeOutlookError(w, "Invalid Request")
		return
	}

	data, err := h.buildData(ctx, r, req.Request.EMailAddress)
	if err != nil {
		h.writeOutlookError(w, "Invalid Request")
		return
	}

	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = outlookResponseTemplate.Execute(w, data)
}

func (h *Handler) writeOutlookError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	now := time.Now().Format(time.RFC3339)
	_, _ = w.Write([]byte("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n"))
	_, _ = w.Write([]byte(`<Autodiscover xmlns="http://schemas.microsoft.com/exchange/autodiscover/responseschema/2006">` + "\n"))
	_, _ = w.Write([]byte(`  <Response xmlns="http://schemas.microsoft.com/exchange/autodiscover/outlook/responseschema/2006a">` + "\n"))
	_, _ = w.Write([]byte(`    <Error Time="` + now + `" Id="0">` + "\n"))
	_, _ = w.Write([]byte(`      <ErrorCode>600</ErrorCode>` + "\n"))
	_, _ = w.Write([]byte(`      <Message>` + template.HTMLEscapeString(message) + `</Message>` + "\n"))
	_, _ = w.Write([]byte(`      <DebugData />` + "\n"))
	_, _ = w.Write([]byte(`    </Error>` + "\n"))
	_, _ = w.Write([]byte(`  </Response>` + "\n"))
	_, _ = w.Write([]byte(`</Autodiscover>`))
}
