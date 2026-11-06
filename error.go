// Package webkit is a small web framework built on net/http, htmx and
// gomponents. The root package defines the base errors that carry an HTTP
// status code for the error handler middleware in package web.
package webkit

import (
	"net/http"

	"github.com/mgnsk/wreck"
)

// Keys for error data in errors.
const (
	KeyHTTPCode = "http_code"
)

// Base errors.
var (
	PreconditionFailed = wreck.New("precondition_failed").With(KeyHTTPCode, http.StatusPreconditionFailed)
	InvalidValue       = wreck.New("invalid_value").With(KeyHTTPCode, http.StatusBadRequest)
	AlreadyExists      = wreck.New("already_exists").With(KeyHTTPCode, http.StatusConflict)
	NotFound           = wreck.New("not_found").With(KeyHTTPCode, http.StatusNotFound)
	Timeout            = wreck.New("timeout").With(KeyHTTPCode, http.StatusRequestTimeout)
	Forbidden          = wreck.New("forbidden").With(KeyHTTPCode, http.StatusForbidden)

	Internal = wreck.New("internal").With(KeyHTTPCode, http.StatusInternalServerError)
)
