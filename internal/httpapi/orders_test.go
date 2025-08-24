package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mrussa/L0/internal/cache"
	"github.com/mrussa/L0/internal/repo"
	"github.com/stretchr/testify/require"
)

type fakeRepo struct {
	Order repo.Order
	Err   error
}

func (f fakeRepo) GetOrder(ctx context.Context, id string) (repo.Order, error) {
	return f.Order, f.Err
}

type nopLogger struct{}

func (nopLogger) Printf(string, ...any) {}

func doJSON(t *testing.T, h http.Handler, method, path string, body io.Reader, hdr map[string]string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var m map[string]any
	ct := rr.Header().Get("Content-Type")
	if strings.HasPrefix(ct, "application/json") && rr.Body.Len() > 0 {
		_ = json.Unmarshal(rr.Body.Bytes(), &m)
	}
	return rr, m
}
func newAPI(src OrderSource, c *cache.OrdersCache) *OrdersAPI {
	return New(src, c, nopLogger{}.Printf, "testver")
}

func TestRootRedirectAndNotFound(t *testing.T) {
	t.Parallel()
	api := newAPI(fakeRepo{}, cache.New())
	h := api.Routes()

	rr, _ := doJSON(t, h, http.MethodGet, "/", nil, nil)
	require.Equal(t, http.StatusTemporaryRedirect, rr.Code)
	require.Equal(t, "/ui/", rr.Header().Get("Location"))

	rr, m := doJSON(t, h, http.MethodGet, "/unknown", nil, nil)
	require.Equal(t, http.StatusNotFound, rr.Code)
	require.Equal(t, "application/json; charset=utf-8", rr.Header().Get("Content-Type"))
	require.Equal(t, "not_found", m["error"])
	require.NotEmpty(t, rr.Header().Get(headerRequestID))
	require.Equal(t, rr.Header().Get(headerRequestID), m["request_id"])
}

func TestHealthz_OK_and_MethodNotAllowed(t *testing.T) {
	t.Parallel()
	api := newAPI(fakeRepo{}, cache.New())
	h := api.Routes()

	rr, m := doJSON(t, h, http.MethodGet, "/healthz", nil, map[string]string{headerRequestID: "rid-123"})
	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, "ok", m["status"])
	require.Equal(t, "testver", m["version"])
	require.Equal(t, "rid-123", m["request_id"])

	rr, m = doJSON(t, h, http.MethodPost, "/healthz", bytes.NewBufferString("{}"), nil)
	require.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	require.Equal(t, "GET, HEAD", rr.Header().Get("Allow"))
	require.Equal(t, "method_not_allowed", m["error"])

	rr, _ = doJSON(t, h, http.MethodHead, "/healthz", nil, nil)
	require.Equal(t, http.StatusOK, rr.Code)
}

func TestOrder_Index_Method_and_BadUID(t *testing.T) {
	t.Parallel()
	api := newAPI(fakeRepo{}, cache.New())
	h := api.Routes()

	rr, m := doJSON(t, h, http.MethodPost, "/order", nil, nil)
	require.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	require.Equal(t, http.MethodGet, rr.Header().Get("Allow"))
	require.Equal(t, "method_not_allowed", m["error"])

	rr, m = doJSON(t, h, http.MethodGet, "/order", nil, nil)
	require.Equal(t, http.StatusBadRequest, rr.Code)
	require.Equal(t, "bad_request", m["error"])
	require.Contains(t, m["message"], "/order/{order_uid}")

	rr, m = doJSON(t, h, http.MethodGet, "/order/", nil, nil)
	require.Equal(t, http.StatusBadRequest, rr.Code)
	require.Equal(t, "bad_request", m["error"])

	long := strings.Repeat("x", 101)
	rr, m = doJSON(t, h, http.MethodGet, "/order/"+long, nil, nil)
	require.Equal(t, http.StatusBadRequest, rr.Code)
	require.Equal(t, "bad_request", m["error"])
}

func TestOrder_CacheHit(t *testing.T) {
	t.Parallel()
	c := cache.New()
	want := repo.Order{OrderUID: "u1"}
	c.Set("u1", want)

	api := newAPI(fakeRepo{}, c)
	h := api.Routes()

	rr, _ := doJSON(t, h, http.MethodGet, "/order/u1", nil, nil)
	require.Equal(t, http.StatusOK, rr.Code)

	var got repo.Order
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	require.Equal(t, want, got)

	require.Equal(t, "application/json; charset=utf-8", rr.Header().Get("Content-Type"))
	require.NotEmpty(t, rr.Header().Get(headerRequestID))
}

func TestOrder_DBBranches(t *testing.T) {
	t.Parallel()
	api := newAPI(fakeRepo{Err: repo.ErrBadUID}, cache.New())
	h := api.Routes()
	rr, m := doJSON(t, h, http.MethodGet, "/order/u2", nil, nil)
	require.Equal(t, http.StatusBadRequest, rr.Code)
	require.Equal(t, "bad_request", m["error"])

	api = newAPI(fakeRepo{Err: repo.ErrNotFound}, cache.New())
	h = api.Routes()
	rr, m = doJSON(t, h, http.MethodGet, "/order/u3", nil, nil)
	require.Equal(t, http.StatusNotFound, rr.Code)
	require.Equal(t, "not_found", m["error"])
	require.Equal(t, "order not found", m["message"])

	api = newAPI(fakeRepo{Err: errors.New("boom")}, cache.New())
	h = api.Routes()
	rr, m = doJSON(t, h, http.MethodGet, "/order/u4", nil, nil)
	require.Equal(t, http.StatusInternalServerError, rr.Code)
	require.Equal(t, "internal", m["error"])

	order := repo.Order{OrderUID: "u5"}
	api = newAPI(fakeRepo{Order: order}, cache.New())
	h = api.Routes()
	rr, _ = doJSON(t, h, http.MethodGet, "/order/u5", nil, nil)
	require.Equal(t, http.StatusOK, rr.Code)

	got, ok := api.cache.Get("u5")
	require.True(t, ok)
	require.Equal(t, order, got)
}

func TestOrder_PathWithExtraSegments_Trimmed(t *testing.T) {
	t.Parallel()
	c := cache.New()
	c.Set("u1", repo.Order{OrderUID: "u1"})
	api := newAPI(fakeRepo{}, c)
	h := api.Routes()

	rr, _ := doJSON(t, h, http.MethodGet, "/order/u1/extra/segment", nil, nil)
	require.Equal(t, http.StatusOK, rr.Code)
}

func TestOrder_ID_MethodNotAllowed(t *testing.T) {
	t.Parallel()
	api := newAPI(fakeRepo{}, cache.New())
	h := api.Routes()

	rr, m := doJSON(t, h, http.MethodPost, "/order/u1", nil, nil)
	require.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	require.Equal(t, http.MethodGet, rr.Header().Get("Allow"))
	require.Equal(t, "method_not_allowed", m["error"])
}

func TestUI_Handler_ExistsButMissingFile(t *testing.T) {
	t.Parallel()
	api := newAPI(fakeRepo{}, cache.New())
	h := api.Routes()

	req := httptest.NewRequest(http.MethodGet, "/ui/__definitely_missing__/file.txt", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusNotFound, rr.Code)
}
