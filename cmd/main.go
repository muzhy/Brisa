package main

import (
	"brisa"
	"brisa/middleware"
	"log"
	"time"

	"github.com/emersion/go-smtp"
)

func main() {
	// 1. Define middleware configuration (in the future, this will be read from a config file)
	ipBlacklistCfg := middleware.IPBlacklistConfig{
		IPs: []string{
			"192.168.1.100", // Block a specific IP
			"10.0.0.0/8",    // Block a network segment
			// "99.99.99.99",   // An invalid IP for testing validation logic (can be commented out to run the program normally)
		},
	}

	// 2. Create middleware instances using constructors and check for errors
	ipBlacklistHandler, err := middleware.NewIPBlacklistHandler(ipBlacklistCfg)
	if err != nil {
		log.Fatalf("Failed to create IP blacklist middleware: %v", err)
	}

	loggingMiddleware := func(stage string) brisa.Handler {
		return func(ctx *brisa.Context) brisa.Action {
			log.Printf("Executing %s middleware", stage)
			return brisa.Pass
		}
	}

	rejectMiddleware := func(ctx *brisa.Context) brisa.Action {
		log.Println("This middleware rejects the mail.")
		return brisa.Reject
	}

	b := brisa.New()

	// Build middleware chains using middlewareChains
	chains := brisa.NewMiddlewareChains()
	chains.RegisterConnMiddleware(brisa.Middleware{
		Handler: ipBlacklistHandler,
	})
	chains.RegisterMailFromMiddleware(brisa.Middleware{
		Handler: loggingMiddleware("MAIL FROM"),
	})
	chains.RegisterRcptToMiddleware(brisa.Middleware{
		Handler: loggingMiddleware("RCPT TO"),
	})

	chains.RegisterRcptToMiddleware(brisa.Middleware{
		Handler:     rejectMiddleware,
		IgnoreFlags: brisa.IgnoreDeliver, // Example: If the mail is already marked for Deliver, this reject middleware will not be executed
	})

	b.UpdateChains(chains)

	s := smtp.NewServer(b)

	s.Addr = ":1025"
	s.Domain = "localhost"
	s.ReadTimeout = 10 * time.Second
	s.WriteTimeout = 10 * time.Second
	s.MaxMessageBytes = 1024 * 1024
	s.MaxRecipients = 50
	s.AllowInsecureAuth = true

	log.Println("Starting SMTP server at", s.Addr)
	if err := s.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
