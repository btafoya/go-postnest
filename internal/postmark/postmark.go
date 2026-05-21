package postmark

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	lib "github.com/mrz1836/postmark"
)

// Client is a thin wrapper around the mrz1836/postmark library.
type Client struct{}

// NewClient creates a Postmark client.
func NewClient() *Client {
	return &Client{}
}

// OutboundMessage is a message to send via Postmark.
type OutboundMessage struct {
	From          string
	To            []string
	Cc            []string
	Bcc           []string
	Subject       string
	TextBody      string
	HTMLBody      string
	MessageStream string
	Attachments   []Attachment
}

// Attachment is an outbound attachment.
type Attachment struct {
	Name        string
	ContentType string
	Content     []byte
}

// SendResponse is the result from Postmark.
type SendResponse struct {
	MessageID string
	ErrorCode int
	Message   string
}

// SendEmail sends a single email via Postmark.
func (c *Client) SendEmail(ctx context.Context, apiToken string, msg *OutboundMessage) (*SendResponse, error) {
	client := lib.NewClient(apiToken, "")
	client.HTTPClient = &http.Client{Timeout: 30 * time.Second}

	email := lib.Email{
		From:          msg.From,
		To:            joinAddresses(msg.To),
		Cc:            joinAddresses(msg.Cc),
		Bcc:           joinAddresses(msg.Bcc),
		Subject:       msg.Subject,
		TextBody:      msg.TextBody,
		HTMLBody:      msg.HTMLBody,
		MessageStream: msg.MessageStream,
	}
	for _, a := range msg.Attachments {
		email.Attachments = append(email.Attachments, lib.Attachment{
			Name:        a.Name,
			Content:     base64.StdEncoding.EncodeToString(a.Content),
			ContentType: a.ContentType,
		})
	}

	res, err := client.SendEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("postmark send: %w", err)
	}
	return &SendResponse{
		MessageID: res.MessageID,
		ErrorCode: int(res.ErrorCode),
		Message:   res.Message,
	}, nil
}

func joinAddresses(addrs []string) string {
	out := ""
	for i, a := range addrs {
		if i > 0 {
			out += ", "
		}
		out += a
	}
	return out
}

// InboundPayload represents a Postmark inbound webhook.
type InboundPayload struct {
	FromName          string              `json:"FromName"`
	From              string              `json:"From"`
	To                string              `json:"To"`
	Subject           string              `json:"Subject"`
	TextBody          string              `json:"TextBody"`
	HTMLBody          string              `json:"HtmlBody"`
	MessageID         string              `json:"MessageID"`
	OriginalRecipient string              `json:"OriginalRecipient"`
	Date              string              `json:"Date"`
	Headers           []InboundHeader     `json:"Headers"`
	Attachments       []InboundAttachment `json:"Attachments"`
}

// InboundHeader is a raw header from Postmark.
type InboundHeader struct {
	Name  string `json:"Name"`
	Value string `json:"Value"`
}

// InboundAttachment is an attachment from Postmark inbound.
type InboundAttachment struct {
	Name        string `json:"Name"`
	ContentType string `json:"ContentType"`
	Content     string `json:"Content"` // base64 encoded
	ContentID   string `json:"ContentID"`
}

// ParseInbound converts a raw JSON map to an InboundPayload.
func ParseInbound(payload map[string]any) (*InboundPayload, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var in InboundPayload
	if err := json.Unmarshal(b, &in); err != nil {
		return nil, err
	}
	return &in, nil
}
