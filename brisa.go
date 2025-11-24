package brisa

import (
	"errors"
	"io"
	"log"
	"net"
	"sync/atomic"

	"github.com/emersion/go-smtp"
	"github.com/google/uuid"
)

var (
	// ErrRejected is returned when a middleware rejects the connection.
	ErrRejected = errors.New("rejected by middleware")
)

// Brisa implements SMTP server methods.
type Brisa struct {
	chains atomic.Pointer[middlewareChains]
}

// New creates a new Brisa instance with initial middleware chains.
func New() *Brisa {
	b := &Brisa{}
	// Initialize with empty chains.
	b.chains.Store(NewMiddlewareChains())
	return b
}

// UpdateChains atomically replaces the current middleware chains with a new set.
// This is the method you would call when your configuration changes.
func (b *Brisa) UpdateChains(set *middlewareChains) {
	b.chains.Store(set)
	log.Println("Middleware chains updated")
}

// NewSession is called after client greeting (EHLO, HELO).
func (b *Brisa) NewSession(c *smtp.Conn) (smtp.Session, error) {
	s := &Session{
		conn:   c,
		chains: b.chains.Load(),
	}
	s.init()

	// Execute connection-level middleware immediately.
	if action := s.chains.ConnChain.execute(s.ctx); action == Reject {
		log.Printf("[%s] Connection rejected by middleware from %s", s.id, c.Conn().RemoteAddr())
		return nil, ErrRejected
	}

	return s, nil
}

type Session struct {
	ctx    *Context
	id     string
	conn   *smtp.Conn
	chains *middlewareChains
}

func (s *Session) init() {
	s.id = uuid.NewString() // Generate a unique ID
	s.ctx = NewContext()
	s.ctx.Session = s
}

func (s *Session) GetClientIP() net.Addr {
	return s.conn.Conn().RemoteAddr()
}

// Mail is called when a sender is specified.
func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	//TODO new email, one session may have mulit email, need generate id for email
	log.Printf("[%s] Mail from: %s", s.id, from)
	if action := s.chains.MailFromChain.execute(s.ctx); action == Reject {
		return ErrRejected
	}
	return nil
}

// Rcpt is called for each recipient.
func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	log.Printf("[%s] Rcpt to: %s", s.id, to)
	action := s.chains.RcptToChain.execute(s.ctx)

	// Process the result based on the action
	if action == Reject {
		return ErrRejected
	}
	if action != Pass {
		log.Printf("[%s] Middleware chain for RcptTo stopped with action: %v", s.id, action)
	}
	return nil
}

// Data is called when a message is received.
func (s *Session) Data(r io.Reader) error {
	if _, err := io.ReadAll(r); err != nil {
		return err
	}
	log.Printf("[%s] Data received", s.id)
	if action := s.chains.DataChain.execute(s.ctx); action == Reject {
		return ErrRejected
	}
	return nil
}

// Reset is called when a transaction is aborted.
func (s *Session) Reset() {
	// TODO
}

// Logout is called when a client closes the connection.
func (s *Session) Logout() error {
	log.Printf("[%s] Session closed", s.id)
	FreeContext(s.ctx)

	return nil
}
