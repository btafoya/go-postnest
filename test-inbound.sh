#!/usr/bin/env bash

# Source - https://stackoverflow.com/a/46173801
# Posted by muru
# Retrieved 2026-05-19, License - CC BY-SA 3.0
#
# Usage: POSTNEST_WEBHOOK_TOKEN=your-token ./test-inbound.sh

set -euo pipefail

TOKEN="${POSTNEST_WEBHOOK_TOKEN:?env var POSTNEST_WEBHOOK_TOKEN required}"
uuid=$(uuidgen)

# Use a heredoc so variables expand in the JSON payload.
json=$(cat <<EOF
{
  "FromName": "premadev.com Support",
  "From": "support@premadev.com",
  "FromFull": {
    "Email": "support@premadev.com",
    "Name": "premadev.com Support",
    "MailboxHash": ""
  },
  "To": "\"Brian Tafoya\" <btafoya@premadev.com>",
  "ToFull": [
    {
      "Email": "btafoya@premadev.com",
      "Name": "Brian Tafoya",
      "MailboxHash": "SampleHash"
    }
  ],
  "Cc": "\"First Cc\" <firstcc@premadev.com>, secondCc@premadev.com",
  "CcFull": [
    {
      "Email": "firstcc@premadev.com",
      "Name": "First Cc",
      "MailboxHash": ""
    },
    {
      "Email": "secondCc@premadev.com",
      "Name": "",
      "MailboxHash": ""
    }
  ],
  "Bcc": "\"First Bcc\" <firstbcc@premadev.com>, secondbcc@premadev.com",
  "BccFull": [
    {
      "Email": "firstbcc@premadev.com",
      "Name": "First Bcc",
      "MailboxHash": ""
    },
    {
      "Email": "secondbcc@premadev.com",
      "Name": "",
      "MailboxHash": ""
    }
  ],
  "OriginalRecipient": "btafoya@premadev.com",
  "Subject": "Test subject",
  "MessageID": "${uuid}",
  "ReplyTo": "replyto@premadev.com",
  "MailboxHash": "SampleHash",
  "Date": "Fri, 01 Aug 2014 16:45:32 -0400",
  "TextBody": "This is a test text body.",
  "HtmlBody": "<html><body><p>This is a test html body.</p></body></html>",
  "StrippedTextReply": "This is the reply text",
  "Tag": "TestTag",
  "Headers": [
    {
      "Name": "X-Header-Test",
      "Value": ""
    },
    {
      "Name": "X-Spam-Status",
      "Value": "No"
    },
    {
      "Name": "X-Spam-Score",
      "Value": "-0.1"
    },
    {
      "Name": "X-Spam-Tests",
      "Value": "DKIM_SIGNED,DKIM_VALID,DKIM_VALID_AU,SPF_PASS"
    }
  ],
  "Attachments": [
    {
      "Name": "test.txt",
      "Content": "VGhpcyBpcyBhdHRhY2htZW50IGNvbnRlbnRzLCBiYXNlLTY0IGVuY29kZWQu",
      "ContentType": "text/plain",
      "ContentLength": 45
    }
  ]
}
EOF
)

curl -s -o /dev/null -w "HTTP Status: %{http_code}\n" \
  "http://192.168.25.165:2626/webhooks/postmark/inbound?token=${TOKEN}" \
  -X POST \
  -H "Content-Type: application/json" \
  -d "$json"
