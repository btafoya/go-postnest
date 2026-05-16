package smtp

import (
	"errors"
	"testing"

	"github.com/emersion/go-sasl"
)

func TestLoginServer_Next(t *testing.T) {
	var authenticated bool
	ls := &loginServer{
		authenticate: func(username, password string) error {
			if username == "alice" && password == "secret" {
				authenticated = true
				return nil
			}
			return errors.New("auth failed")
		},
	}

	// Step 0: server sends Username challenge when no initial response
	challenge, done, err := ls.Next(nil)
	if err != nil {
		t.Fatalf("step 0 error: %v", err)
	}
	if done {
		t.Fatal("expected not done at step 0")
	}
	if string(challenge) != "Username:" {
		t.Errorf("challenge = %q, want Username:", string(challenge))
	}

	// Client sends username, server sends Password challenge
	challenge, done, err = ls.Next([]byte("alice"))
	if err != nil {
		t.Fatalf("step 1 error: %v", err)
	}
	if done {
		t.Fatal("expected not done at step 1")
	}
	if string(challenge) != "Password:" {
		t.Errorf("challenge = %q, want Password:", string(challenge))
	}

	// Client sends password, authentication completes
	_, done, err = ls.Next([]byte("secret"))
	if err != nil {
		t.Fatalf("step 2 error: %v", err)
	}
	if !done {
		t.Fatal("expected done at step 2")
	}
	if !authenticated {
		t.Error("expected authenticate callback to be invoked")
	}
}

func TestLoginServer_InvalidCredentials(t *testing.T) {
	ls := &loginServer{
		authenticate: func(username, password string) error {
			return errors.New("auth failed")
		},
	}

	_, _, _ = ls.Next(nil) // Username challenge
	_, _, _ = ls.Next([]byte("bob")) // Password challenge
	_, done, err := ls.Next([]byte("wrong"))
	if err == nil {
		t.Fatal("expected auth error")
	}
	if !done {
		t.Error("expected done on auth failure")
	}
}

func TestAuthMechanisms(t *testing.T) {
	mechs := (&smtpSession{}).AuthMechanisms()
	foundPlain := false
	foundLogin := false
	for _, m := range mechs {
		if m == sasl.Plain {
			foundPlain = true
		}
		if m == "LOGIN" {
			foundLogin = true
		}
	}
	if !foundPlain {
		t.Error("expected PLAIN mechanism")
	}
	if !foundLogin {
		t.Error("expected LOGIN mechanism")
	}
}
