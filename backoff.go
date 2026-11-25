package restapiclient

import (
	"context"
	"errors"

	"github.com/tekkamanendless/httperror"
)

// DefaultBackOffIsPermanentErrorFunction is the default function to determine if an error is permanent.
//
// It will RETRY on:
//
//   - "Too Early"
//   - "Too Many Reqeusts"
//   - "Internal Server Error"
//   - "Bad Gateway"
//   - "Service Unavailable"
//   - "Gateway Timeout"
func DefaultBackOffIsPermanentErrorFunction(err error) bool {
	if errors.Is(err, context.Canceled) {
		return true // This is a permanent error.
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true // This is a permanent error.
	}

	if errors.Is(err, httperror.ErrStatusTooEarly) {
		return false // Retry.
	}
	if errors.Is(err, httperror.ErrStatusTooManyRequests) {
		return false // Retry.
	}
	if errors.Is(err, httperror.ErrStatusInternalServerError) {
		return false // Retry.
	}
	if errors.Is(err, httperror.ErrStatusBadGateway) {
		return false // Retry.
	}
	if errors.Is(err, httperror.ErrStatusServiceUnavailable) {
		return false // Retry.
	}
	if errors.Is(err, httperror.ErrStatusGatewayTimeout) {
		return false // Retry.
	}

	return true // This is a permanent error.
}
