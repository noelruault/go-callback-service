package web

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/pkg/errors"
	"go.opencensus.io/trace"
)

// Respond converts a Go value to JSON and sends it to the client.
func Respond(ctx context.Context, w http.ResponseWriter, data interface{}, statusCode int) error {
	// Set the status code for the request logger middleware.
	// If the context is missing this value, request the service
	// to be shutdown gracefully.
	v, ok := ctx.Value(KeyValues).(*Values)
	if !ok {
		log.Fatal("web value missing from context")
	}
	v.StatusCode = statusCode

	if statusCode == http.StatusNoContent {
		w.WriteHeader(statusCode)
		return nil
	}

	// Convert the response value to JSON.
	res, err := json.Marshal(data)
	if err != nil {
		return err
	}

	// Respond with the provided JSON.
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	if _, err := w.Write(res); err != nil {
		return err
	}

	return nil
}

// PublicError is an error that returns a string code that can be presented to the API user.
type PublicError interface {
	error
	Code() string
	Detail() string
}

// ErrorResponse is the form used for API responses from failures in the API.
type ErrorResponse struct {
	Error  string `json:"error"`
	Detail string `json:"message"`
}

type ErrorSpan struct {
	*trace.Span
}

// RespondError sends an error reponse back to the client.
func RespondError(ctx context.Context, w http.ResponseWriter, err error, statusCode int) error {
	_, span := trace.StartSpan(ctx, "web.RespondError")
	defer span.End()
	errSpan := ErrorSpan{span}

	// If the error was of the type PublicError the handler has a specific
	// status code and error message to return.
	switch err := errors.Cause(err).(type) {
	case PublicError:
		errSpan.zipkinSetErrorOnSpan(err.Code(), err.Detail(), int32(statusCode))
		er := ErrorResponse{
			Error:  err.Code(),
			Detail: err.Detail(),
		}
		if err := Respond(ctx, w, er, statusCode); err != nil {
			return err
		}
	default:
		errSpan.zipkinSetErrorOnSpan("server_error", "", int32(http.StatusInternalServerError))
		// If not, the handler sent any arbitrary error value so use 500.
		if err := Respond(ctx, w,
			ErrorResponse{Error: http.StatusText(http.StatusInternalServerError)},
			http.StatusInternalServerError,
		); err != nil {
			return err
		}
	}
	return nil
}

// zipkinSetErrorOnSpan tags the span with an error code to:
// - Visualise it with a red color on the Zipkin UI
// - Propagate the error code and (optional) message
func (span *ErrorSpan) zipkinSetErrorOnSpan(errcode, errmessage string, statusCode int32) {
	span.AddAttributes(trace.StringAttribute("error", errcode))
	if errmessage != "" {
		span.AddAttributes(trace.StringAttribute("message", errmessage))
	}
	span.SetStatus(trace.Status{
		Code: statusCode, // Code status determines the color of the span in the UI
	})
}
