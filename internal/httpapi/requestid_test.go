package httpapi

import (
	crand "crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWithRequestID_Middleware(t *testing.T) {
	t.Parallel()
	base := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"rid": RequestID(r),
		})
	})
	h := WithRequestID(base)

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(headerRequestID, "abcd-1234")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, "abcd-1234", rr.Header().Get(headerRequestID))

	var m map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &m))
	require.Equal(t, "abcd-1234", m["rid"])

	req2 := httptest.NewRequest(http.MethodGet, "/y", nil)
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req2)
	gen := rr2.Header().Get(headerRequestID)
	require.NotEmpty(t, gen)
	require.Len(t, gen, 32)

	m = map[string]string{}
	require.NoError(t, json.Unmarshal(rr2.Body.Bytes(), &m))
	require.Equal(t, gen, m["rid"])
}

func TestNewID_HexLen(t *testing.T) {
	t.Parallel()
	id := newID()
	require.NotEmpty(t, id)
	require.Len(t, id, 32)
	for _, ch := range id {
		require.True(t, strings.ContainsRune("0123456789abcdef", ch), "non-hex: %q", ch)
	}
}

type errReader struct{}

func (errReader) Read(p []byte) (int, error) { return 0, errors.New("rng fail") }

func TestNewID_FallbackOnRandError(t *testing.T) {
	orig := crand.Reader
	crand.Reader = errReader{}
	defer func() { crand.Reader = orig }()

	id := newID()
	require.Equal(t, "rid-fallback", id)
}

func TestRequestID_NoMiddleware(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/z", nil)
	require.Equal(t, "", RequestID(req))
}
