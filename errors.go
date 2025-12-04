package brisa

import "github.com/emersion/go-smtp"

var (
	// ErrRejectedByPolicy is returned when a middleware explicitly rejects a command.
	// It signals a permanent failure for the current transaction with a 554 code.
	ErrRejectedByPolicy = &smtp.SMTPError{
		Code:         554,
		EnhancedCode: smtp.EnhancedCode{5, 7, 1},
		Message:      "Message rejected due to policy",
	}

	// ErrInternalServer is returned for unhandled exceptions (panics) or other
	// unexpected errors in the middleware chain. It signals a temporary failure (451)
	// and encourages the client to retry.
	ErrInternalServer = &smtp.SMTPError{
		Code:         451,
		EnhancedCode: smtp.EnhancedCode{4, 3, 0},
		Message:      "Internal server error, please try again later",
	}

	// ErrTryAgainLater is a generic temporary failure that can be returned by
	// middleware to signal the client to retry (421).
	ErrTryAgainLater = &smtp.SMTPError{
		Code:         421,
		EnhancedCode: smtp.EnhancedCode{4, 3, 2},
		Message:      "Service temporarily unavailable, please try again later",
	}

	// ErrInvalidAction is returned when an invalid action is encountered after
	// the data middleware chain has finished. It signals a permanent failure (554).
	ErrInvalidAction = &smtp.SMTPError{
		Code:         554,
		EnhancedCode: smtp.EnhancedCode{5, 3, 5},
		Message:      "Transaction failed due to an invalid internal state",
	}
)
