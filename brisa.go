package brisa

import (
	"errors"
	"io"
	"log/slog"
	"net"
	"sync/atomic"
	"time"

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
	// Deliver Quarantine Reject 用于定义确定Action之后要执行的动作
	ChainDeliver    ChainType = "deliver"
	ChainQuarantine ChainType = "quarantine"
	ChainReject     ChainType = "reject"
	ChainDiscard    ChainType = "discard"
)

// Router holds all named middleware chains for the Brisa server.
// It's used to build a complete set of middleware chains that can be atomically
// applied to a Brisa instance. It is not safe for concurrent use; concurrency
// should be managed by the consumer (e.g., Brisa) through atomic replacement
// of the entire instance.
type Router map[ChainType]MiddlewareChain

// Use adds one or more middlewares to the specified chain.
func (r *Router) Use(chainName ChainType, middlewares ...*Middleware) *Router {
	for _, m := range middlewares {
		(*r)[chainName] = append((*r)[chainName], *m)
	}
	return r
}

// OnConn adds a middleware to the Conn chain.
func (r *Router) OnConn(m ...*Middleware) *Router {
	return r.Use(ChainConn, m...)
}

// OnMailFrom adds a middleware to the MailFrom chain.
func (r *Router) OnMailFrom(m ...*Middleware) *Router {
	return r.Use(ChainMailFrom, m...)
}

// OnRcptTo adds a middleware to the RcptTo chain.
func (r *Router) OnRcptTo(m ...*Middleware) *Router {
	return r.Use(ChainRcptTo, m...)
}

// OnData adds a middleware to the Data chain.
func (r *Router) OnData(m ...*Middleware) *Router {
	return r.Use(ChainData, m...)
}

// OnDeliver adds one or more middlewares to the Deliver chain.
func (r *Router) OnDeliver(m ...*Middleware) *Router {
	return r.Use(ChainDeliver, m...)
}

// OnQuarantine adds one or more middlewares to the Quarantine chain.
func (r *Router) OnQuarantine(m ...*Middleware) *Router {
	return r.Use(ChainQuarantine, m...)
}

// OnReject adds one or more middlewares to the Reject chain.
func (r *Router) OnReject(m ...*Middleware) *Router {
	return r.Use(ChainReject, m...)
}

// OnDiscard adds one or more middlewares to the Discard chain.
func (r *Router) OnDiscard(m ...*Middleware) *Router {
	return r.Use(ChainDiscard, m...)
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
	router    atomic.Pointer[Router]
	logger    *slog.Logger
	observers []Observer
}

// New creates a new Brisa instance with an initial logger and optional observers.
func New(logger *slog.Logger, observers ...Observer) *Brisa {
	if logger == nil {
		logger = slog.Default()
	}

	b := &Brisa{
		logger:    logger,
		observers: observers,
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
		ctx:        ctx,
		id:         id,
		conn:       c,
		router:     b.router.Load(),
		baseLogger: ctx.Logger,
		observers:  b.observers,
	}
	// Link session back to context
	s.ctx.Session = s

	for _, o := range b.observers {
		o.OnSessionStart(s.ctx)
	}

	err := s.execute(ChainConn)
	if err != nil {
		return nil, err
	}

	return s, nil
}

// ------- Session ---------
type Session struct {
	ctx        *Context
	id         string
	conn       *smtp.Conn
	router     *Router
	baseLogger *slog.Logger
	observers  []Observer
}

func (s *Session) GetClientIP() net.Addr {
	return s.conn.Conn().RemoteAddr()
}

// Mail is called when a sender is specified.
func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	s.resetMailTransaction()

	// generate mail_id for each email
	mailId := uuid.NewString()
	s.ctx.Logger = s.baseLogger.With("mail_id", mailId)

	s.ctx.From = from
	s.ctx.FromOptions = opts
	return s.execute(ChainMailFrom)
}

// Rcpt is called for each recipient.
func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	s.ctx.To = append(s.ctx.To, to)
	s.ctx.ToOptions = append(s.ctx.ToOptions, opts)
	return s.execute(ChainRcptTo)
}

// Data is called when a message is received.
func (s *Session) Data(r io.Reader) error {
	s.ctx.Reader = r
	defer func() {
		// Ensure the reader is always consumed to avoid client timeout.
		// If no middleware consumes it, discard the data.
		// This is a safe fallback. A dedicated middleware should ideally handle this.
		io.Copy(io.Discard, s.ctx.Reader)
	}()

	err := s.execute(ChainData)
	if err != nil {
		return err
	}

	// If after all data middleware, the status is still Pass, it means no middleware
	// made a final decision (like Deliver, Quarantine, or Reject).
	// In this case, we can treat it as an implicit delivery.
	if s.ctx.Action == Pass {
		s.ctx.Action = Deliver
	}

	switch s.ctx.Action {
	case Deliver:
		err := s.execute(ChainDeliver)
		if err != nil {
			return err
		}
	case Quarantine:
		err := s.execute(ChainQuarantine)
		if err != nil {
			return err
		}
	case Discard:
		err := s.execute(ChainDiscard)
		if err != nil {
			// Errors in the Discard chain probably shouldn't fail the SMTP transaction,
			// as the intent is to successfully receive and then drop the mail.
			s.ctx.Logger.Error("discard middleware execute failed", "error", err)
		}
	default:
		s.ctx.Logger.Error("Should never here. Illegal action in data finised middleware chain", "action", s.ctx.Action)
		return ErrInvalidAction
	}

	return nil
}

// Reset is called when a transaction is aborted.
func (s *Session) Reset() {
	s.resetMailTransaction()
}

// resetMailTransaction resets the state for a single mail transaction,
// allowing the session to be reused for another mail.
func (s *Session) resetMailTransaction() {
	s.ctx.ResetMailFields()
	s.ctx.Logger = s.baseLogger // Revert to the session-level logger.
}

// Logout is called when a client closes the connection.
func (s *Session) Logout() error {
	for _, o := range s.observers {
		o.OnSessionEnd(s.ctx)
	}
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

	for _, o := range s.observers {
		o.OnChainStart(s.ctx, chainType)
	}
	startTime := time.Now()

	action, err := chain.Execute(s.ctx)

	duration := time.Since(startTime)
	for _, o := range s.observers {
		o.OnChainEnd(s.ctx, chainType, duration)
	}

	if err != nil || action == Reject {
		s.ctx.Action = Reject // Ensure context reflects the final decision.

		// Execute reject chain if it exists.
		// Errors from the reject chain are logged but not returned to the client,
		// as a primary decision to reject has already been made.
		if rejectChain, ok := (*s.router)[ChainReject]; ok {
			if _, rejectErr := rejectChain.Execute(s.ctx); rejectErr != nil {
				s.ctx.Logger.Error("reject middleware execute failed", "error", rejectErr)
			}
		}

		// Determine which SMTP error to return.
		if err != nil {
			s.ctx.Logger.Error("middleware execute failed, rejecting command", "error", err, "ChainType", string(chainType))
			// If the middleware returned a specific smtp.SMTPError, use it.
			var smtpErr *smtp.SMTPError
			if errors.As(err, &smtpErr) {
				return smtpErr
			}
			// Otherwise, it was likely a panic, so return a generic internal server error.
			return ErrInternalServer
		}

		// If there was no error but the action is Reject, return the default policy rejection.
		return ErrRejectedByPolicy
	}

	return nil
}
