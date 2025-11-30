package brisa

import (
	"io"
	"log/slog"
	"reflect"
	"testing"

	"github.com/emersion/go-smtp"
)

func TestBrisa_NewSession(t *testing.T) {
	// b := &Brisa{}
	b := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	s, err := b.NewSession(&smtp.Conn{})
	if err != nil {
		t.Errorf("Expected no error, but got %v", err)
	}
	if s == nil {
		t.Errorf("Expected a session, but got nil")
	}
	if _, ok := s.(*Session); !ok {
		t.Errorf("Expected a *Session, but got %T", s)
	}
}

func TestRouter_Use(t *testing.T) {
	// Create a couple of distinct middleware for testing.
	mw1 := &Middleware{IgnoreFlags: 1}
	mw2 := &Middleware{IgnoreFlags: 2}
	mw3 := &Middleware{IgnoreFlags: 3}

	t.Run("add middleware to a new chain", func(t *testing.T) {
		router := &Router{}
		router.Use(ChainConn, mw1)

		chain, ok := (*router)[ChainConn]
		if !ok {
			t.Fatal("expected chain 'conn' to exist, but it doesn't")
		}
		if len(chain) != 1 {
			t.Fatalf("expected chain 'conn' to have length 1, but got %d", len(chain))
		}
		if !reflect.DeepEqual(chain[0], *mw1) {
			t.Errorf("middleware in chain is not the one we added. Got %+v, want %+v", chain[0], *mw1)
		}
	})

	t.Run("append middleware to an existing chain in correct order", func(t *testing.T) {
		router := &Router{
			ChainConn: {*mw1},
		}
		router.Use(ChainConn, mw2)

		chain := (*router)[ChainConn]
		if len(chain) != 2 {
			t.Fatalf("expected chain 'conn' to have length 2, but got %d", len(chain))
		}
		if !reflect.DeepEqual(chain[0], *mw1) {
			t.Errorf("first middleware is incorrect. Got %+v, want %+v", chain[0], *mw1)
		}
		if !reflect.DeepEqual(chain[1], *mw2) {
			t.Errorf("second middleware is incorrect. Got %+v, want %+v", chain[1], *mw2)
		}
	})

	t.Run("use chainable calls", func(t *testing.T) {
		router := &Router{}
		router.Use(ChainData, mw1).Use(ChainData, mw2).Use(ChainConn, mw3)

		dataChain, ok := (*router)[ChainData]
		if !ok || len(dataChain) != 2 {
			t.Fatalf("expected chain 'data' to have 2 middlewares, but got %d", len(dataChain))
		}
		if !reflect.DeepEqual(dataChain[1], *mw2) {
			t.Error("chainable call order for 'data' chain is incorrect")
		}

		connChain, ok := (*router)[ChainConn]
		if !ok || len(connChain) != 1 {
			t.Fatalf("expected chain 'conn' to have 1 middleware, but got %d", len(connChain))
		}
		if !reflect.DeepEqual(connChain[0], *mw3) {
			t.Error("middleware for 'conn' chain is incorrect")
		}
	})
}

func TestRouter_Clone(t *testing.T) {
	// Setup original router
	originalRouter := &Router{}
	mw1 := &Middleware{IgnoreFlags: 1}
	mw2 := &Middleware{IgnoreFlags: 2}
	originalRouter.Use(ChainConn, mw1).Use(ChainConn, mw2)

	// Perform the clone
	clonedRouter := originalRouter.Clone()

	t.Run("clone is not the same instance", func(t *testing.T) {
		if clonedRouter == originalRouter {
			t.Fatal("cloned router should be a new instance, but it points to the same router")
		}
	})

	t.Run("initial content is identical", func(t *testing.T) {
		if !reflect.DeepEqual(originalRouter, clonedRouter) {
			t.Errorf("cloned router content is not identical to the original.\nOriginal: %+v\nCloned:   %+v", *originalRouter, *clonedRouter)
		}
	})

	t.Run("modifying original router does not affect clone", func(t *testing.T) {
		// Modify original: add a new middleware to an existing chain
		mw3 := &Middleware{IgnoreFlags: 3}
		originalRouter.Use(ChainConn, mw3)

		// Modify original: add a new chain
		originalRouter.Use(ChainData, mw1)

		if len((*clonedRouter)[ChainConn]) != 2 {
			t.Errorf("modifying original chain should not affect clone. Cloned chain length is %d, want 2", len((*clonedRouter)[ChainConn]))
		}

		if _, exists := (*clonedRouter)[ChainData]; exists {
			t.Error("adding a new chain to original router should not affect clone, but it does")
		}
	})

	t.Run("modifying clone does not affect original", func(t *testing.T) {
		clonedRouter.Use(ChainRcptTo, mw1)
		if _, exists := (*originalRouter)[ChainRcptTo]; exists {
			t.Error("adding a new chain to cloned router should not affect original, but it does")
		}
	})
}

func TestBrisa_UpdateRouter(t *testing.T) {
	// 1. 创建一个 Brisa 实例
	b := New(slog.New(slog.NewTextHandler(io.Discard, nil)))

	// 2. 创建一个初始的 Router
	originalRouter := &Router{}
	mw1 := &Middleware{IgnoreFlags: 1}
	mw2 := &Middleware{IgnoreFlags: 2}
	originalRouter.Use(ChainConn, mw1)

	// 3. 调用 UpdateRouter
	b.UpdateRouter(originalRouter)

	// 检查初始状态是否正确复制
	internalRouter := b.router.Load()
	if !reflect.DeepEqual(originalRouter, internalRouter) {
		t.Fatalf("内部 router 应该是原始 router 的深拷贝。\n原始: %+v\n内部:   %+v", *originalRouter, *internalRouter)
	}

	// 4. 在调用 UpdateRouter 后修改原始的 router
	//    - 向现有链中添加新的中间件
	originalRouter.Use(ChainConn, mw2)
	//    - 添加一个全新的链
	originalRouter.Use(ChainData, mw1)

	// 5. 验证内部的 router 没有被修改
	internalRouterAfterModification := b.router.Load()

	// 验证内部 router 实例没有改变
	if internalRouter != internalRouterAfterModification {
		t.Fatal("Brisa 实例内部的 router 指针不应该在外部修改后发生变化")
	}

	// 验证内部 router 的内容没有被修改
	if len((*internalRouterAfterModification)[ChainConn]) != 1 {
		t.Errorf("修改原始 router 不应影响内部 router。内部 'conn' 链的长度应为 1，但得到 %d", len((*internalRouterAfterModification)[ChainConn]))
	}
	if _, exists := (*internalRouterAfterModification)[ChainData]; exists {
		t.Error("向原始 router 添加新链不应影响内部 router，但 'data' 链存在于内部 router 中")
	}
}
