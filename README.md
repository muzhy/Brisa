# brisa: A Modern and Modular SMTP Gateway Framework for Go

`brisa` is a modular, extensible, and observable SMTP receiving gateway framework built in Go. It's designed to be a robust entry point for mail systems, efficiently receiving emails and processing them through a flexible middleware pipeline.

`brisa` intentionally separates the act of receiving an email from its final disposition. It's up to you to decide what happens to the email—whether it's saved to the filesystem, pushed to a message queue, or discarded—by implementing custom middleware.

### Core Design Philosophy

*   **Separation of Concerns**: `brisa` provides a reliable SMTP engine and middleware execution pipeline. The final handling of the mail (saving, delivering, etc.) is completely decoupled.
*   **Disposition via Middleware**: The framework does not include any hard-coded logic for saving or delivering mail. All final actions are accomplished by adding one or more disposition middlewares to the end of the processing chain.
*   **Modularity and Extensibility**: The entire email processing flow is built on a chain of middleware, making it easy to compose, add, or remove validation and processing logic.
*   **Observability First**: Designed from the ground up with integrated structured logging, with clear extension points for metrics and tracing to ensure system status is transparent and easy to monitor.

## Features

*   **Middleware-based Architecture**: Process emails through a flexible and composable pipeline.
*   **Hot-Reloadable Configuration**: Atomically update middleware chains on a running server with zero downtime.
*   **Clean Separation of Concerns**: Focus on your business logic, not the SMTP protocol intricacies.
*   **Extensible by Design**: Add custom logic for validation, content analysis, or any other processing you need.
*   **Built-in Context Management**: A pooled `Context` object efficiently carries state through the middleware chain.
*   **Action-based Flow Control**: Use a simple `Action` system (`Pass`, `Reject`, `Deliver`, etc.) to control the processing flow.

## Core Concepts

#### The `Router` and `MiddlewareChain`

The `Router` holds named `MiddlewareChain`s. You attach your middleware to these chains, which correspond to different stages of the SMTP conversation or final disposition actions.

#### SMTP Event Chains

*   `OnConn`: Fires when a client connects. Useful for IP blacklisting or connection rate limiting.
*   `OnMailFrom`: Fires after the `MAIL FROM` command. Useful for sender validation.
*   `OnRcptTo`: Fires after the `RCPT TO` command. Useful for recipient validation.
*   `OnData`: Fires before the email body (`DATA`) is processed. Useful for content analysis, spam filtering, etc.

#### Disposition Chains

After the `OnData` chain completes, `brisa` examines the final `Action` status. Based on that status, it executes a corresponding disposition chain:

*   `OnDeliver`: For processing emails marked for delivery.
*   `OnQuarantine`: For handling emails sent to quarantine.
*   `OnDiscard`: For emails that should be silently dropped.
*   `OnReject`: For handling the rejection process (e.g., custom logging).

#### The `Context`

A `Context` object is created for each session and passed through the middleware chain. It carries the session state (like sender, recipient, IP address), the email data (`io.Reader`), a structured logger, and a key-value store for passing data between middlewares.

#### The `Action` System

Each middleware `Handler` returns an `Action`:

*   `Pass`: Continues to the next middleware.
*   `Deliver`, `Quarantine`, `Discard`: Sets the email's disposition status. Middleware can choose to skip execution if a certain status is already set by using `IgnoreFlags`.
*   `Reject`: Immediately stops the current chain and rejects the SMTP command.

## Installation

```sh
go get github.com/muzhy/brisa
```

## Quick Start

Here’s how to set up a simple SMTP server with custom middleware.

```go
package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/emersion/go-smtp"
	"github.com/google/uuid"
	"github.com/muzhy/brisa"
	"github.com/muzhy/brisa/middleware"
)

// 1. Define custom middleware handlers.

// This simple middleware logs the sender and recipient of each email.
func loggingMiddleware(ctx *brisa.Context) brisa.Action {
	ctx.Logger.Info("Processing email", "from", ctx.From, "to", ctx.To)
	return brisa.Pass // Continue to the next middleware
}

// This middleware decides the final action. It simulates "delivering" the email
// by consuming its content and logging it. In a real application, this would
// save the email to a file, database, or message queue.
func finalDeliveryMiddleware(ctx *brisa.Context) brisa.Action {
	mailID := uuid.NewString() // In a real app, you might get this from the context.
	ctx.Logger.Info("Email marked for delivery. Saving message.", "mail_id", mailID)

	// Here, you would consume ctx.Reader to get the email content.
	// For example, to save it to a file:
	//
	// f, err := os.Create(fmt.Sprintf("%s.eml", mailID))
	// if err != nil {
	//     ctx.Logger.Error("failed to create email file", "error", err)
	//     return brisa.Reject // Or another appropriate action
	// }
	// defer f.Close()
	// io.Copy(f, ctx.Reader)

	// For this example, we'll just discard it.
	io.Copy(io.Discard, ctx.Reader)

	return brisa.Deliver
}

func main() {
	// Initialize a logger.
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// 2. Create and configure the middleware router.
	router := brisa.Router{}

	// Use a pre-built middleware for IP blacklisting on new connections.
	ipBlacklistHandler, err := middleware.NewIPBlacklistHandler([]string{"192.168.1.100"})
	if err != nil {
		logger.Error("failed to create IPBlacklist handler", "error", err)
		return
	}
	router.OnConn(&brisa.Middleware{
		Handler: ipBlacklistHandler,
	})

	// Add our custom logging middleware to the `Data` chain.
	// It will run before the email content is processed.
	router.OnData(&brisa.Middleware{
		Handler: loggingMiddleware,
		// This middleware should run even if a previous middleware has already
		// set the action. So we set IgnoreFlags to 0 (the default is to skip).
		IgnoreFlags: 0,
	})

	// 3. Add a "disposition" middleware.
	// This runs in the `Deliver` chain, which executes after the `Data` chain
	// if the email's final action is `Deliver`.
	router.OnDeliver(&brisa.Middleware{
		Handler: finalDeliveryMiddleware,
	})

	// 4. Create a new Brisa instance and apply the router.
	b := brisa.New(logger)
	b.UpdateRouter(&router) // Atomically updates the router

	// 5. Configure and start the SMTP server using the standard library.
	s := smtp.NewServer(b)
	s.Addr = ":1025"
	s.Domain = "localhost"
	s.ReadTimeout = 10 * time.Second
	s.WriteTimeout = 10 * time.Second
	s.MaxMessageBytes = 1024 * 1024
	s.MaxRecipients = 50
	s.AllowInsecureAuth = true

	logger.Info("starting Brisa SMTP server...", "address", s.Addr)
	if err := s.ListenAndServe(); err != nil {
		logger.Error("server failed to start", "error", err)
		os.Exit(1)
	}
}
```

## Writing Custom Middleware

A middleware is just a function with the signature `func(ctx *brisa.Context) brisa.Action`.

```go
import "strings"

func MyCustomMiddleware(ctx *brisa.Context) brisa.Action {
    // Access session information
    clientIP := ctx.Session.GetClientIP()
    ctx.Logger.Info("My middleware is running", "ip", clientIP.String())

    // Example: check the sender
    if strings.HasSuffix(ctx.From, "@spammer.com") {
        return brisa.Reject // Reject the email
    }

    // Pass data to subsequent middleware
    ctx.Set("my-key", "my-value")

    // Continue processing
    return brisa.Pass
}

// Then add it to a chain:
router.OnMailFrom(&brisa.Middleware{
    Handler: MyCustomMiddleware,
})
```

## Configuration & Hot-Reloading

`brisa` is designed for dynamic configuration. The `UpdateRouter` method on a `*Brisa` instance is thread-safe and atomically replaces the entire set of middleware chains. This allows you to rebuild your `Router` from a configuration source (e.g., YAML, TOML) and apply it to a running server without any downtime.

```go
// Imagine this function is called when you receive a signal or an admin request.
func reloadConfiguration(b *brisa.Brisa, newIPs []string) {
    log.Println("Reloading configuration...")
    
    // 1. Build a completely new router instance from the new config.
    newRouter := brisa.Router{}
    ipBlacklist, _ := middleware.NewIPBlacklistHandler(newIPs)
    newRouter.OnConn(&brisa.Middleware{ Handler: ipBlacklist })
    // ... configure other chains ...

    // 2. Atomically apply the new router.
    // New sessions will immediately start using the new middleware chains.
    b.UpdateRouter(&newRouter)

    log.Println("Successfully applied new configuration.")
}
```

## Roadmap

*   Implement a standard middleware for saving received emails to the local filesystem.
*   Add more examples of disposition middleware (e.g., pushing to RabbitMQ/Kafka).
*   Integrate metrics (e.g., Prometheus) for monitoring throughput, rejections, etc.
*   Add support for distributed tracing (e.g., OpenTelemetry).
*   Add more built-in middleware for common tasks (e.g., SPF/DKIM checks).
