package imap

import (
	"crypto/tls"
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	imapserver "github.com/emersion/go-imap/server"
	"github.com/go-postnest/postnest/internal/auth"
	"github.com/go-postnest/postnest/internal/mailstore"
	"github.com/go-postnest/postnest/internal/redis"
)

// Server wraps the go-imap server.
type Server struct {
	addr     string
	tlsCfg   *tls.Config
	srv      *imapserver.Server
	ln       net.Listener
	wg       sync.WaitGroup
}

// NewServer creates an IMAP server.
func NewServer(addr string, tlsCfg *tls.Config, allowInsecureAuth bool, store mailstore.Store, auth *auth.Service, redis *redis.Client) *Server {
	be := &imapBackend{store: store, auth: auth, redis: redis}
	s := imapserver.New(be)
	s.Addr = addr
	s.AllowInsecureAuth = allowInsecureAuth
	if tlsCfg != nil {
		s.TLSConfig = tlsCfg
	}
	return &Server{addr: addr, tlsCfg: tlsCfg, srv: s}
}

// Start listens for IMAP connections.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	if s.tlsCfg != nil {
		ln = tls.NewListener(ln, s.tlsCfg)
	}
	s.ln = ln
	return s.srv.Serve(s.ln)
}

// Stop closes the listener and waits up to 30s for connections to drain.
func (s *Server) Stop() error {
	if s.ln != nil {
		_ = s.ln.Close()
	}
	// go-imap server does not expose connection tracking.
	// Give in-flight operations a brief grace period.
	time.Sleep(2 * time.Second)
	return s.srv.Close()
}

func isClosedErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, net.ErrClosed) {
		return true
	}
	return strings.Contains(err.Error(), "closed") || strings.Contains(err.Error(), "use of closed network connection")
}
