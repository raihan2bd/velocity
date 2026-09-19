package velocity

import (
	"errors"
	"fmt"
	"net/http"
)

// HTTPError is a safe, typed error that can be returned by a HandlerFunc.
// Message is safe to expose to clients; Err is retained only for logging.
type HTTPError struct {
	Status  int
	Code    string
	Message string
	Err     error
}

func (e *HTTPError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	if e.Message != "" {
		return e.Message
	}
	return http.StatusText(e.Status)
}

func (e *HTTPError) Unwrap() error { return e.Err }

// NewHTTPError creates an error that the default error handler can render.
func NewHTTPError(status int, message string) *HTTPError {
	return &HTTPError{Status: status, Message: message}
}

// WrapHTTPError adds an internal cause without disclosing it in the response.
func WrapHTTPError(status int, message string, err error) *HTTPError {
	return &HTTPError{Status: status, Message: message, Err: err}
}

var (
	ErrBadRequest          = NewHTTPError(http.StatusBadRequest, "bad request")
	ErrUnauthorized        = NewHTTPError(http.StatusUnauthorized, "unauthorized")
	ErrForbidden           = NewHTTPError(http.StatusForbidden, "forbidden")
	ErrNotFound            = NewHTTPError(http.StatusNotFound, "not found")
	ErrMethodNotAllowed    = NewHTTPError(http.StatusMethodNotAllowed, "method not allowed")
	ErrPayloadTooLarge     = NewHTTPError(http.StatusRequestEntityTooLarge, "request body too large")
	ErrUnsupportedMedia    = NewHTTPError(http.StatusUnsupportedMediaType, "unsupported media type")
	ErrInternalServerError = NewHTTPError(http.StatusInternalServerError, "internal server error")
)

// ValidationError identifies a field that did not satisfy a validation tag.
type ValidationError struct {
	Field string `json:"field"`
	Tag   string `json:"tag"`
	Value string `json:"-"`
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s failed %s validation", e.Field, e.Tag)
}

// ValidationErrors is returned by Bind* methods after successful decoding when
// validation tags fail.
type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string { return "validation failed" }

// AsHTTPError converts framework errors to their HTTP representation.
func AsHTTPError(err error) *HTTPError {
	if err == nil {
		return nil
	}
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		if httpErr.Status < 400 || httpErr.Status > 599 {
			return ErrInternalServerError
		}
		return httpErr
	}
	var validation ValidationErrors
	if errors.As(err, &validation) {
		return &HTTPError{Status: http.StatusUnprocessableEntity, Code: "VALIDATION_FAILED", Message: "validation failed", Err: err}
	}
	return &HTTPError{Status: http.StatusInternalServerError, Message: "internal server error", Err: err}
}
