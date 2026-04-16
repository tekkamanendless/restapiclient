package restapiclient

import (
	"errors"
	"fmt"
)

// ErrClient is a client error.
// All client-related (caller-related) errors will be of this type.
var ErrClient = errors.New("restapiclient: client error")

// ErrClientInvalidInput is returned when the input parameter is invalid.
var ErrClientInvalidInput = fmt.Errorf("%w: invalid input", ErrClient)

// ErrClientInvalidOutput is returned when the output parameter is invalid.
var ErrClientInvalidOutput = fmt.Errorf("%w: invalid output", ErrClient)
