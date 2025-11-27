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

const (
	ChainConn     = "conn"
	ChainMailFrom = "mail_from"
	ChainRcptTo   = "rcpt_to"
	ChainData     = "data"
)

var (
	// ErrRejected is returned when a middleware rejects the connection.
	ErrRejected = errors.New("rejected by middleware")
)

// Brisa implements SMTP server methods.
type Brisa struct {
	chains atomic.Pointer[MiddlewareChains]
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
func (b *Brisa) UpdateChains(set *MiddlewareChains) {
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

	err := s.executeChain(ChainConn, "Greet")
	if err != nil {
		return nil, ErrRejected
	}

	return s, nil
}

type Session struct {
	ctx    *Context
	id     string
	conn   *smtp.Conn
	chains *MiddlewareChains
}

func (s *Session) GetClientIP() net.Addr {
	return s.conn.Conn().RemoteAddr()
}

// Mail is called when a sender is specified.
func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	//TODO new email, one session may have mulit email, need generate id for email
	s.ctx.Logger().Info("MAIL FROM command received", "from", from)
	return s.executeChain(ChainMailFrom, "MAIL FROM")
}

// Rcpt is called for each recipient.
func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	s.ctx.Logger().Info("RCPT TO command received", "to", to)
	return s.executeChain(ChainRcptTo, "RCPT TO")
}

// Data is called when a message is received.
func (s *Session) Data(r io.Reader) error {
	if _, err := io.ReadAll(r); err != nil {
		return err
	}
	return s.executeChain(ChainData, "DATA")
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

// executeChain is a helper method to run a middleware chain for a given SMTP command.
// It fetches the appropriate chain, executes it, and handles panics or rejections.
func (s *Session) executeChain(chainName string, commandName string) error {
	chain, ok := s.chains.Get(chainName)
	if !ok {
		// No middleware chain is defined for this command, so we allow it.
		return nil
	}

	if action, err := chain.Execute(s.ctx); err != nil || action == Reject {
		if err != nil {
			s.ctx.Logger().Error(commandName+" middleware panicked, rejecting command", "error", err)
		}
		s.ctx.Status = Reject // Ensure context reflects the final decision.
		return ErrRejected
	}

	return nil
}
