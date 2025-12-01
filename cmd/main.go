package main

import (
	"brisa"
	"log/slog"
	"os"
	"time"

	"github.com/emersion/go-smtp"

	"brisa/middleware"
)

func main() {
	// init logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// set routter
	router := brisa.Router{}
	// create middleware
	ipBlacklistHandler, err := middleware.NewIPBlacklistHandler([]string{"192.168.1.100"})
	if err != nil {
		logger.Error("create IPBlacklist Handler failed")
		return
	}
	router.OnConn(&brisa.Middleware{
		Handler:     ipBlacklistHandler,
		IgnoreFlags: brisa.DefaultIgnoreFlags,
	})

	b := brisa.New(logger)
	b.UpdateRouter(&router)

	// start server
	s := smtp.NewServer(b)
	s.Addr = ":1025"
	s.Domain = "localhost" // 可在配置中添加
	s.ReadTimeout = 10 * time.Second
	s.WriteTimeout = 10 * time.Second
	s.MaxMessageBytes = 1024 * 1024 // 可在配置中添加
	s.MaxRecipients = 50            // 可在配置中添加
	s.AllowInsecureAuth = true      // 可在配置中添加

	logger.Info("starting SMTP server...", "address", s.Addr)
	if err := s.ListenAndServe(); err != nil {
		logger.Error("server failed to start", "error", err)
		os.Exit(1)
	}
}
