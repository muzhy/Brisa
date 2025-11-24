package brisa

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestBrisa_NewSession(t *testing.T) {
	// b := &Brisa{}
	b := New()
	s, err := b.NewSession(nil)
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

func TestSession_Mail(t *testing.T) {
	s := &Session{}
	err := s.Mail("test@example.com", nil)
	if err != nil {
		t.Errorf("Expected no error, but got %v", err)
	}
}

func TestSession_Rcpt(t *testing.T) {
	s := &Session{}
	err := s.Rcpt("test@example.com", nil)
	if err != nil {
		t.Errorf("Expected no error, but got %v", err)
	}
}

type errorReader struct{}

func (r *errorReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("test error")
}

// ensure errorReader implements io.Reader
var _ io.Reader = &errorReader{}

func TestSession_Data(t *testing.T) {
	s := &Session{}

	t.Run("SuccessfulRead", func(t *testing.T) {
		r := strings.NewReader("test data")
		err := s.Data(r)
		if err != nil {
			t.Errorf("Expected no error, but got %v", err)
		}
	})

	t.Run("FailedRead", func(t *testing.T) {
		r := &errorReader{}
		err := s.Data(r)
		if err == nil {
			t.Errorf("Expected an error, but got nil")
		}
	})
}

func TestSession_Reset(t *testing.T) {
	s := &Session{}
	// Just call it to ensure it doesn't panic
	s.Reset()
}

func TestSession_Logout(t *testing.T) {
	s := &Session{}
	err := s.Logout()
	if err != nil {
		t.Errorf("Expected no error, but got %v", err)
	}
}
