package main

import (
	"brisa"
	"brisa/middleware"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/emersion/go-smtp"
)

func main() {

	// Create logger for application
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create MiddlewareFactory and Register IPBlackList middleware
	factory := brisa.NewMiddlewareFactory(logger)
	if err := factory.Register("ip_blacklist", middleware.IPBlacklistFactory); err != nil {
		logger.Error("register ip_balcklist failed", "error", err.Error())
		return
	}

	// Define IPBlacklist config
	ipBlacklistCfg := map[string]any{
		"IPs": []string{
			"192.168.1.100", // Block a specific IP
		},
	}

	// Config middleware chains
	chains := brisa.NewMiddlewareChains()
	if m, err := factory.Create("ip_blacklist", ipBlacklistCfg); err != nil {
		logger.Error("create ip_blacklist failed", "error", err.Error())
		return
	} else {
		// chains.RegisterConnMiddleware(*m)
		chains.Register(brisa.ChainConn, *m)
	}

	// Create Brisa and set chain
	b := brisa.New(logger)
	b.UpdateChains(chains)

	// Start server
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
