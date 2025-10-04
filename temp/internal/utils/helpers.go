package utils

import (
	"context"
	"fmt"
	"net/http"
)

// HandleError is a utility function for handling errors in a consistent manner.
func HandleError(ctx context.Context, err error, message string) {
	if err != nil {
		// Log the error with context
		fmt.Printf("Error: %s: %v\n", message, err)
		// Optionally, you can add more sophisticated logging here
	}
}

// IsHTTPError checks if an error is an HTTP error.
func IsHTTPError(err error) bool {
	if httpErr, ok := err.(interface{ StatusCode() int }); ok {
		return httpErr.StatusCode() >= 400 && httpErr.StatusCode() < 600
	}
	return false
}

// ContextWithTimeout creates a new context with a timeout.
func ContextWithTimeout(timeout int) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
}