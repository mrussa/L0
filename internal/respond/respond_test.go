package respond

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJSON_WritesStatusHeaderAndBody(t *testing.T) {
	rec := httptest.NewRecorder()
	payload := map[string]any{"ok": true, "n": 42}

	JSON(rec, http.StatusCreated, payload)

	require.Equal(t, http.StatusCreated, rec.Code)
	require.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))

	got := map[string]any{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, true, got["ok"])
	require.Equal(t, float64(42), got["n"])
}

func TestError_WrapsJSON(t *testing.T) {
	rec := httptest.NewRecorder()

	Error(rec, http.StatusBadRequest, "bad_request", "invalid input")

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))

	var body ErrorBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "bad_request", body.Error)
	require.Equal(t, "invalid input", body.Message)
	require.Empty(t, body.RequestID)
}

func TestErrorWithID_WrapsJSON(t *testing.T) {
	rec := httptest.NewRecorder()

	ErrorWithID(rec, http.StatusUnauthorized, "unauthorized", "no token", "req-123")

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))

	var body ErrorBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "unauthorized", body.Error)
	require.Equal(t, "no token", body.Message)
	require.Equal(t, "req-123", body.RequestID)
}

func TestNoContent(t *testing.T) {
	rec := httptest.NewRecorder()

	NoContent(rec)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Equal(t, 0, rec.Body.Len())
}

func TestHelpers(t *testing.T) {
	tests := []struct {
		name   string
		call   func(w http.ResponseWriter, msg string)
		status int
		code   string
	}{
		{"BadRequest", BadRequest, http.StatusBadRequest, "bad_request"},
		{"NotFound", NotFound, http.StatusNotFound, "not_found"},
		{"Internal", Internal, http.StatusInternalServerError, "internal"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tt.call(rec, "oops")

			require.Equal(t, tt.status, rec.Code)
			require.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))

			var body ErrorBody
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Equal(t, tt.code, body.Error)
			require.Equal(t, "oops", body.Message)
			require.Empty(t, body.RequestID)
		})
	}
}
