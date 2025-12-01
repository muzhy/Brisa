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

// ChainType defines the type for middleware chain names, providing type safety.
type ChainType string

const (
	ChainConn     ChainType = "conn"
	ChainMailFrom ChainType = "mail_from"
	ChainRcptTo   ChainType = "rcpt_to"
	ChainData     ChainType = "data"
)

var (
	// ErrRejected is returned when a middleware rejects the connection.
	ErrRejected = errors.New("rejected by middleware")
)

// Router holds all named middleware chains for the Brisa server.
// It's used to build a complete set of middleware chains that can be atomically
// applied to a Brisa instance. It is not safe for concurrent use; concurrency
// should be managed by the consumer (e.g., Brisa) through atomic replacement
// of the entire instance.
type Router map[ChainType]MiddlewareChain

// Use adds a middleware to the specified chain.
func (r *Router) Use(chainName ChainType, m *Middleware) *Router {
	(*r)[chainName] = append((*r)[chainName], *m)
	return r
}

// OnConn adds a middleware to the Conn chain.
func (r *Router) OnConn(m *Middleware) *Router {
	return r.Use(ChainConn, m)
}

// OnMailFrom adds a middleware to the MailFrom chain.
func (r *Router) OnMailFrom(m *Middleware) *Router {
	return r.Use(ChainMailFrom, m)
}

// OnRcptTo adds a middleware to the RcptTo chain.
func (r *Router) OnRcptTo(m *Middleware) *Router {
	return r.Use(ChainRcptTo, m)
}

// OnData adds a middleware to the Data chain.
func (r *Router) OnData(m *Middleware) *Router {
	return r.Use(ChainData, m)
}

// Clone creates a deep copy of the Router.
// It returns a new Router instance with a new underlying map, and each
// middleware chain is also a new slice with its own backing array. This ensures
// that modifications to the original Router or its chains do not affect the clone.
func (r *Router) Clone() *Router {
	newRouter := make(Router, len(*r))
	for chainType, chain := range *r {
		// Create a new slice with the same length and capacity.
		chainCopy := make(MiddlewareChain, len(chain), cap(chain))
		copy(chainCopy, chain) // Copy elements to the new slice's backing array.
		newRouter[chainType] = chainCopy
	}
	return &newRouter
}

// Brisa implements SMTP server methods.
type Brisa struct {
	router atomic.Pointer[Router]
	logger *slog.Logger
}

// New creates a new Brisa instance with initial router
func New(logger *slog.Logger) *Brisa {
	if logger == nil {
		logger = slog.Default()
	}

	b := &Brisa{
		logger: logger,
	}
	// Initialize with empty chains.
	b.router.Store(&Router{})

	return b
}

// UpdateChains atomically replaces the current middleware chains with a new set.
// This is the method you would call when your configuration changes.
// To ensure thread safety, this method clones the provided router to create a
// completely independent deep copy. This prevents race conditions where the
// caller might modify the router or its middleware chains after application.
func (b *Brisa) UpdateRouter(router *Router) {
	// Atomically store a deep copy of the router.
	b.router.Store(router.Clone())
	b.logger.Info("Middleware chains updated")
}

// NewSession is called after client greeting (EHLO, HELO).
func (b *Brisa) NewSession(c *smtp.Conn) (smtp.Session, error) {
	id := uuid.NewString()
	ctx := NewContext()
	ctx.Logger = b.logger.With("session_id", id)

	s := &Session{
		ctx:    ctx,
		id:     id,
		conn:   c,
		router: b.router.Load(),
	}
	// Link session back to context
	s.ctx.Session = s

	err := s.execute(ChainConn)
	if err != nil {
		return nil, ErrRejected
	}

	return s, nil
}

// ------- Session ---------
type Session struct {
	ctx    *Context
	id     string
	conn   *smtp.Conn
	router *Router
}

func (s *Session) GetClientIP() net.Addr {
	return s.conn.Conn().RemoteAddr()
}

// Mail is called when a sender is specified.
func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	//TODO new email, one session may have mulit email, need generate id for email
	s.ctx.Logger.Info("MAIL FROM command received", "from", from)
	return s.execute(ChainMailFrom)
}

// Rcpt is called for each recipient.
func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	s.ctx.Logger.Info("RCPT TO command received", "to", to)
	return s.execute(ChainRcptTo)
}

// Data is called when a message is received.
func (s *Session) Data(r io.Reader) error {
	s.ctx.Reader = r

	err := s.execute(ChainData)

	if err != nil {
		return err
	} else {
		if s.ctx.Status == Pass {
			s.ctx.Status = Deliver
		}
	}

	// TODO handle action

	return nil
}

// Reset is called when a transaction is aborted.
func (s *Session) Reset() {
	// TODO
}

// Logout is called when a client closes the connection.
func (s *Session) Logout() error {
	s.ctx.Logger.Info("Session closed")
	FreeContext(s.ctx)

	return nil
}

// execute is a helper method to run a middleware chain for a given SMTP command.
// It fetches the appropriate chain, executes it, and handles panics or rejections.
func (s *Session) execute(chainType ChainType) error {
	chain, ok := (*s.router)[chainType]
	if !ok {
		// No middleware chain is defined for this command, so we allow it.
		return nil
	}

	if action, err := chain.Execute(s.ctx); err != nil || action == Reject {
		if err != nil {
			s.ctx.Logger.Error("middleware panicked, rejecting command", "error", err, "ChainType", string(chainType))
		}
		s.ctx.Status = Reject // Ensure context reflects the final decision.
		return ErrRejected
	}

	return nil
}
