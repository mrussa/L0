package respond

import "net/http"

func BadRequest(w http.ResponseWriter, msg string) {
	Error(w, http.StatusBadRequest, "bad_request", msg)
}
func NotFound(w http.ResponseWriter, msg string) {
	Error(w, http.StatusNotFound, "not_found", msg)
}
func Internal(w http.ResponseWriter, msg string) {
	Error(w, http.StatusInternalServerError, "internal", msg)
}
