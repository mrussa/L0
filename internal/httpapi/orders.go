package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/mrussa/L0/internal/cache"
	"github.com/mrussa/L0/internal/repo"
	"github.com/mrussa/L0/internal/respond"
)

type OrderSource interface {
	GetOrder(ctx context.Context, id string) (repo.Order, error)
}

type OrdersAPI struct {
	repo    OrderSource
	cache   *cache.OrdersCache
	logf    func(string, ...any)
	version string
}

func New(repo OrderSource, cache *cache.OrdersCache, logf func(string, ...any), version string) *OrdersAPI {
	return &OrdersAPI{
		repo:    repo,
		cache:   cache,
		logf:    logf,
		version: version,
	}
}

func (a *OrdersAPI) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.Handle("/ui/", http.StripPrefix("/ui/", http.FileServer(http.Dir("web"))))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/ui/", http.StatusTemporaryRedirect)
			return
		}
		reqID := RequestID(r)
		respond.ErrorWithID(w, http.StatusNotFound, "not_found", "not found", reqID)
	})

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		reqID := RequestID(r)

		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", http.MethodGet+", "+http.MethodHead)
			respond.ErrorWithID(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", reqID)
			return
		}

		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		respond.JSON(w, http.StatusOK, map[string]any{
			"status":     "ok",
			"cache_size": a.cache.Len(),
			"version":    a.version,
			"request_id": reqID,
		})
	})

	mux.HandleFunc("/order", func(w http.ResponseWriter, r *http.Request) {
		reqID := RequestID(r)
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			respond.ErrorWithID(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", reqID)
			return
		}
		respond.ErrorWithID(w, http.StatusBadRequest, "bad_request", "use /order/{order_uid}", reqID)
	})

	mux.HandleFunc("/order/", func(w http.ResponseWriter, r *http.Request) {
		reqID := RequestID(r)
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			respond.ErrorWithID(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", reqID)
			return
		}

		id := strings.TrimPrefix(r.URL.Path, "/order/")
		if i := strings.IndexByte(id, '/'); i >= 0 {
			id = id[:i]
		}
		if id == "" || len(id) > 100 {
			respond.ErrorWithID(w, http.StatusBadRequest, "bad_request", "bad order_uid", reqID)
			return
		}

		if o, ok := a.cache.Get(id); ok {
			a.logf("cache hit id=%s", id)
			respond.JSON(w, http.StatusOK, o)
			return
		}
		a.logf("cache miss id=%s", id)

		o, err := a.repo.GetOrder(r.Context(), id)
		if err != nil {
			switch {
			case errors.Is(err, repo.ErrBadUID):
				respond.ErrorWithID(w, http.StatusBadRequest, "bad_request", "bad order_uid", reqID)
			case errors.Is(err, repo.ErrNotFound):
				respond.ErrorWithID(w, http.StatusNotFound, "not_found", "order not found", reqID)
			default:
				a.logf("order load failed id=%s err=%v", id, err)
				respond.ErrorWithID(w, http.StatusInternalServerError, "internal", "internal error", reqID)
			}
			return
		}

		a.cache.Set(id, o)
		respond.JSON(w, http.StatusOK, o)
	})

	return WithRequestID(mux)
}
