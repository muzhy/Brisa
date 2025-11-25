package brisa

import (
	"io"
	"log/slog"
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
