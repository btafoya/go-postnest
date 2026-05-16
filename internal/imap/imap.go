package imap

import (
	"crypto/tls"
	"errors"
	"net"
	"strings"

	imapserver "github.com/emersion/go-imap/server"
	"github.com/go-postnest/postnest/internal/auth"
	"github.com/go-postnest/postnest/internal/mailstore"
	"github.com/go-postnest/postnest/internal/redis"
)

// Server wraps the go-imap server.
type Server struct {
	addr string
	srv  *imapserver.Server
}

// NewServer creates an IMAP server.
func NewServer(addr string, tlsCfg *tls.Config, store mailstore.Store, auth *auth.Service, redis *redis.Client) *Server {
	be := &imapBackend{store: store, auth: auth, redis: redis}
	s := imapserver.New(be)
	s.Addr = addr
	s.AllowInsecureAuth = true
	if tlsCfg != nil {
		s.TLSConfig = tlsCfg
	}
	return &Server{addr: addr, srv: s}
}

// Start listens for IMAP connections.
func (s *Server) Start() error {
	if err := s.srv.ListenAndServe(); err != nil && !isClosedErr(err) {
		return err
	}
	return nil
}

// Stop shuts down the IMAP server.
func (s *Server) Stop() error {
	return s.srv.Close()
}

func isClosedErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, net.ErrClosed) {
		return true
	}
	return strings.Contains(err.Error(), "closed")
}
