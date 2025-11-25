package brisa

import (
	"errors"
	"io"
	"log/slog"
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
	logger *slog.Logger
}

// New creates a new Brisa instance with initial middleware chains.
func New(logger *slog.Logger) *Brisa {
	if logger == nil {
		logger = slog.Default()
	}

	b := &Brisa{
		logger: logger,
	}
	// Initialize with empty chains.
	b.chains.Store(NewMiddlewareChains())

	return b
}

// UpdateChains atomically replaces the current middleware chains with a new set.
// This is the method you would call when your configuration changes.
func (b *Brisa) UpdateChains(set *middlewareChains) {
	b.chains.Store(set)
	b.logger.Info("Middleware chains updated")
}

// NewSession is called after client greeting (EHLO, HELO).
func (b *Brisa) NewSession(c *smtp.Conn) (smtp.Session, error) {
	id := uuid.NewString()
	ctx := NewContext()
	ctx.logger = b.logger.With("session_id", id)

	s := &Session{
		ctx:    ctx,
		id:     id,
		conn:   c,
		chains: b.chains.Load(),
	}
	// Link session back to context
	s.ctx.Session = s

	// Execute connection-level middleware immediately.
	if action := s.chains.ConnChain.execute(ctx); action == Reject {
		ctx.Logger().Warn("Connection rejected by middleware")
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

func (s *Session) GetClientIP() net.Addr {
	return s.conn.Conn().RemoteAddr()
}

// Mail is called when a sender is specified.
func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	//TODO new email, one session may have mulit email, need generate id for email
	s.ctx.Logger().Info("MAIL FROM command received", "from", from)
	if action := s.chains.MailFromChain.execute(s.ctx); action == Reject {
		return ErrRejected
	}
	return nil
}

// Rcpt is called for each recipient.
func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	s.ctx.Logger().Info("RCPT TO command received", "to", to)
	action := s.chains.RcptToChain.execute(s.ctx)

	// Process the result based on the action
	if action == Reject {
		return ErrRejected
	}
	if action != Pass {
		s.ctx.Logger().Debug("Middleware chain for RcptTo finished with specific action", "action", action)
	}
	return nil
}

// Data is called when a message is received.
func (s *Session) Data(r io.Reader) error {
	if _, err := io.ReadAll(r); err != nil {
		return err
	}
	s.ctx.Logger().Info("Data received")
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
	s.ctx.Logger().Info("Session closed")
	FreeContext(s.ctx)

	return nil
}
