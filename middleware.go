package brisa

// Action 表示中间件执行后要采取的操作，同时也用作状态标志。
// 它的值被设计为位标志，以便与 IgnoreFlags 进行位运算。
type Action int // 使用 int 以便清晰地与 IgnoreFlags (也是 int) 进行位运算

const (
	// Pass 继续执行下一个中间件。作为默认/初始状态。
	Pass Action = 1 << iota // 1
	// Reject 拒绝邮件并停止处理。
	Reject // 2
	// Deliver 标记邮件为待投递。
	Deliver // 4
	// Quarantine 标记邮件为隔离。
	Quarantine // 8
)

// IgnoreFlags 定义了中间件可以忽略的状态。
const (
	// IgnoreDeliver 当上下文状态为 Deliver 时，跳过此中间件。
	IgnoreDeliver Action = Deliver
	// IgnoreQuarantine 当上下文状态为 Quarantine 时，跳过此中间件。
	IgnoreQuarantine Action = Quarantine
	// DefaultIgnoreFlags 是中间件的默认忽略标志，默认忽略已投递或已隔离的状态。
	DefaultIgnoreFlags = IgnoreDeliver | IgnoreQuarantine
)

// Handler 是处理会话上下文的函数，是中间件的核心逻辑。
type Handler func(ctx *Context) Action

// Middleware 是一个包含处理逻辑和元数据的结构体。
type Middleware struct {
	// Handler 是此中间件要执行的函数。
	Handler Handler
	// IgnoreFlags 是一个位掩码，指示该中间件应跳过哪些上下文状态。
	IgnoreFlags Action
}

// MiddlewareChain 是中间件的切片。
type MiddlewareChain []Middleware

// execute 遍历并执行链中的所有中间件。
// 它会传递给定的上下文给每个中间件。
// 如果一个中间件的 Handler 被执行，它的返回 Action 会更新 Context 的状态。
// 如果 Action 是 Reject，则立即停止并返回 Reject。
func (mc MiddlewareChain) execute(ctx *Context) Action {
	for _, m := range mc {
		// 如果上下文的当前状态位与中间件的忽略标志位有重叠，则跳过此中间件。
		if (m.IgnoreFlags & ctx.Status) != 0 {
			continue
		}

		action := m.Handler(ctx)

		// 如果 action 是 Reject，则立即停止并返回。
		// 其他 action (Pass, Deliver, Quarantine) 则更新上下文状态。
		if action == Reject { // Reject 是一个终止状态
			return action
		}

		// 将上下文状态更新为当前中间件的决定。
		// 注意：这里允许后续的中间件覆盖之前的状态（例如，从Deliver改为Quarantine）。
		// 这种灵活性是特意设计的，以支持需要“改判”的复杂场景。
		ctx.Status = action
	}
	return ctx.Status
}

// middlewareChains holds all middleware chains for the Brisa server.
// It's used to build a complete set of middleware chains that can be atomically
// applied to a Brisa instance.
type middlewareChains struct {
	ConnChain     MiddlewareChain
	MailFromChain MiddlewareChain
	RcptToChain   MiddlewareChain
	DataChain     MiddlewareChain
}

// NewMiddlewareChains creates a new, empty middlewareChains.
func NewMiddlewareChains() *middlewareChains {
	return &middlewareChains{}
}

// register is a helper method to add middleware to a specified chain and handle default IgnoreFlags.
func (c *middlewareChains) register(chain *MiddlewareChain, m Middleware) {
	if m.IgnoreFlags == 0 {
		m.IgnoreFlags = DefaultIgnoreFlags
	}
	*chain = append(*chain, m)
}

// RegisterConnMiddleware adds middleware to the connection chain.
func (c *middlewareChains) RegisterConnMiddleware(m Middleware) {
	c.register(&c.ConnChain, m)
}

// RegisterMailFromMiddleware adds middleware to the MAIL FROM chain.
func (c *middlewareChains) RegisterMailFromMiddleware(m Middleware) {
	c.register(&c.MailFromChain, m)
}

// RegisterRcptToMiddleware adds middleware to the RCPT TO chain.
func (c *middlewareChains) RegisterRcptToMiddleware(m Middleware) {
	c.register(&c.RcptToChain, m)
}

// RegisterDataMiddleware adds middleware to the DATA chain.
func (c *middlewareChains) RegisterDataMiddleware(m Middleware) {
	c.register(&c.DataChain, m)
}
