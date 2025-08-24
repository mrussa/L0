package respond

import (
	"encoding/json"
	"net/http"
)

type ErrorBody struct {
	Error     string `json:"error"`
	Message   string `json:"message,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func Error(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, ErrorBody{
		Error:   code,
		Message: message,
	})
}

func ErrorWithID(w http.ResponseWriter, status int, code, message, reqID string) {
	JSON(w, status, ErrorBody{
		Error:     code,
		Message:   message,
		RequestID: reqID,
	})
}

func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}
