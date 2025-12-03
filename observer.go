package brisa

import "time"

// Observer defines an interface for components that wish to monitor the lifecycle
// of SMTP sessions and middleware chain executions. This provides a non-intrusive
// way to implement observability features like metrics and tracing.
type Observer interface {
	// OnSessionStart is called immediately after a new session is created and
	// its context is initialized.
	OnSessionStart(ctx *Context)

	// OnSessionEnd is called just before a session is terminated and its context
	// is put back into the pool.
	OnSessionEnd(ctx *Context)

	// OnChainStart is called just before a middleware chain begins execution.
	OnChainStart(ctx *Context, chainType ChainType)

	// OnChainEnd is called immediately after a middleware chain finishes execution.
	// It provides the final action of the chain and the total execution duration.
	OnChainEnd(ctx *Context, chainType ChainType, duration time.Duration)
}
