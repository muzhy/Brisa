package brisa

import (
	"fmt"
	"io"
	"log/slog"
	"sync"
)

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

// MiddlewareFactoryFunc 是一个创建中间件的工厂函数签名。
// 它接收一个通用的 map 配置，并返回一个 brisa.Middleware 实例或一个错误。
type MiddlewareFactoryFunc func(config map[string]any) (Middleware, error)

// Factory 是一个用于创建和管理中间件的工厂。
type MiddlewareFactory struct {
	registry map[string]MiddlewareFactoryFunc
	// 计划后续支持在运行时动态注册中间件，增加锁保护
	mu     sync.RWMutex
	logger *slog.Logger
}

// NewMiddlewareFactory creates a new MiddlewareFactory.
func NewMiddlewareFactory(logger *slog.Logger) *MiddlewareFactory {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &MiddlewareFactory{
		registry: make(map[string]MiddlewareFactoryFunc),
		logger:   logger,
	}
}

// Register registers a MiddlewareFactoryFunc with a given name.
// It is safe for concurrent use.
func (f *MiddlewareFactory) Register(name string, factoryFunc MiddlewareFactoryFunc) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	// 不允许重名注册
	if _, exists := f.registry[name]; exists {
		return fmt.Errorf("middleware factory with name '%s' already exists", name)
	}

	f.registry[name] = factoryFunc
	return nil
}

// 注销已注册到MiddlewareFactory中的插件
func (f *MiddlewareFactory) Unregister(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	// 检查插件是否存在，如果存在再注销，不存再返回错误信息
	if _, exists := f.registry[name]; !exists {
		return fmt.Errorf("middleware factory with name '%s' does not exist", name)
	}

	delete(f.registry, name)
	return nil
}

// Get returns a registered MiddlewareFactoryFunc by name.
// It is safe for concurrent use.
// 返回的`Middleware`使用指针还是使用对象更合适？
func (f *MiddlewareFactory) Create(name string, config map[string]any) (*Middleware, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	factoryFunc, exists := f.registry[name]
	if !exists {
		return nil, fmt.Errorf("middleware factory with name '%s' does not exist", name)
	}

	// 调用特定中间件的工厂函数
	mw, err := factoryFunc(config)
	if err != nil {
		return nil, fmt.Errorf("error creating middleware '%s': %w", name, err)
	}

	// --- 统一处理通用元数据 ---
	// 检查配置中是否有自定义的 ignore_flags
	if flagsVal, ok := config["ignore_flags"]; ok {
		// 尝试将值转换为整数类型
		switch flags := flagsVal.(type) {
		case int:
			mw.IgnoreFlags = Action(flags)
		case float64: // YAML/JSON 解析数字时可能为 float64
			mw.IgnoreFlags = Action(int(flags))
		default:
			f.logger.Warn("Invalid type for 'ignore_flags', expected an integer, using default", "middleware", name, "type", fmt.Sprintf("%T", flagsVal))
		}
	}
	// --------------------------

	return &mw, nil
}

// 列出当前已注册的中间件
// TODO 后续考虑更好的表现形式，单纯依赖注册时使用的名字可能不够
func (f *MiddlewareFactory) List() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	keys := make([]string, 0, len(f.registry))
	for k := range f.registry {
		keys = append(keys, k)
	}
	return keys
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
