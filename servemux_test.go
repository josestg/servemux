package servemux

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fakeMiddleware(name, start, end string) Middleware {
	return func(next Handler) Handler {
		return HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
			w.Header().Add("X-Middleware", name+start)
			defer w.Header().Add("X-Middleware", end)
			return next.ServeHTTP(w, r)
		})
	}
}

func TestFoldMiddleware(t *testing.T) {
	mid := FoldMiddleware(
		fakeMiddleware("m1", "{", "}"),
		fakeMiddleware("m2", "(", ")"),
		fakeMiddleware("m3", "[", "]"),
	)

	mux := mid.Then(HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
		w.Header().Add("X-Middleware", "h")
		return nil
	}))

	res := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	err := mux.ServeHTTP(res, req)
	expectTrue(t, err == nil)
	expectTrue(t, res.Code == 200)

	traces := "m1{m2(m3[h])}"
	actual := strings.Join(res.Header().Values("X-Middleware"), "")
	expectTrue(t, traces == actual)
}

func TestServeMux_Route(t *testing.T) {
	mux := New()
	mux.Route(Route{
		Pattern: "POST /data",
		Handler: func(w http.ResponseWriter, r *http.Request) error {
			w.WriteHeader(201)
			return nil
		},
	})

	t.Run("POST /data: expect 201", func(t *testing.T) {
		res := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/data", nil)
		mux.ServeHTTP(res, req)
		expectTrue(t, res.Code == 201)
	})

	t.Run("GET /data: expect 405", func(t *testing.T) {
		res := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/data", nil)
		mux.ServeHTTP(res, req)
		expectTrue(t, res.Code == http.StatusMethodNotAllowed)
	})

	t.Run("POST /data/1: expect 404", func(t *testing.T) {
		res := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/data/1", nil)
		mux.ServeHTTP(res, req)
		expectTrue(t, res.Code == http.StatusNotFound)
	})
}

func TestServeMux_HandlerFunc(t *testing.T) {
	mux := New()
	mux.HandleFunc("POST /data", func(w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(201)
		return nil
	})

	t.Run("POST /data: expect 201", func(t *testing.T) {
		res := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/data", nil)
		mux.ServeHTTP(res, req)
		expectTrue(t, res.Code == 201)
	})

	t.Run("GET /data: expect 405", func(t *testing.T) {
		res := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/data", nil)
		mux.ServeHTTP(res, req)
		expectTrue(t, res.Code == http.StatusMethodNotAllowed)
	})

	t.Run("POST /data/1: expect 404", func(t *testing.T) {
		res := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/data/1", nil)
		mux.ServeHTTP(res, req)
		expectTrue(t, res.Code == http.StatusNotFound)
	})
}

func TestServeMux_RouteWithPathParams(t *testing.T) {
	var visited bool
	mux := New()
	mux.Route(Route{
		Pattern: "GET /data/{id}",
		Handler: func(w http.ResponseWriter, r *http.Request) error {
			id := r.PathValue("id")
			expectTrue(t, id == "123")
			visited = true
			return nil
		},
	})

	req := httptest.NewRequest("GET", "/data/123", nil)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	expectTrue(t, visited)
}

func TestServeMux_GlobalMiddleware(t *testing.T) {
	mid := Middleware(func(next Handler) Handler {
		return HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
			w.Header().Add("X-Trace", "mid-started")
			defer w.Header().Add("X-Trace", "mid-ended")
			return next.ServeHTTP(w, r)
		})
	})

	mux := New()
	mux.SetGlobalMiddlewares(mid)

	mux.Route(Route{
		Pattern: "POST /data",
		Handler: func(w http.ResponseWriter, r *http.Request) error {
			w.Header().Add("X-Trace", "POST /data")
			w.WriteHeader(201)
			return nil
		},
	})

	mux.Route(Route{
		Pattern: "GET /data",
		Handler: func(w http.ResponseWriter, r *http.Request) error {
			w.Header().Add("X-Trace", "GET /data")
			w.WriteHeader(200)
			return nil
		},
	})

	t.Run("POST /data: expect 201", func(t *testing.T) {
		res := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/data", nil)
		mux.ServeHTTP(res, req)
		traces := strings.Join(res.Header().Values("X-Trace"), ",")
		expectTrue(t, res.Code == 201)
		expectTrue(t, traces == "mid-started,POST /data,mid-ended")
	})

	t.Run("GET /data: expect 200", func(t *testing.T) {
		res := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/data", nil)
		mux.ServeHTTP(res, req)
		traces := strings.Join(res.Header().Values("X-Trace"), ",")
		expectTrue(t, res.Code == 200)
		expectTrue(t, traces == "mid-started,GET /data,mid-ended")
	})
}

func TestServeMux_SpecificMiddleware(t *testing.T) {
	global := Middleware(func(next Handler) Handler {
		return HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
			w.Header().Add("X-Trace", "global-mid-started")
			defer w.Header().Add("X-Trace", "global-mid-ended")
			return next.ServeHTTP(w, r)
		})
	})

	local := Middleware(func(next Handler) Handler {
		return HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
			w.Header().Add("X-Trace", "local-mid-started")
			defer w.Header().Add("X-Trace", "local-mid-ended")
			return next.ServeHTTP(w, r)
		})
	})

	mux := New()
	mux.SetGlobalMiddlewares(global)

	mux.Route(
		Route{Pattern: "POST /data", Handler: func(w http.ResponseWriter, r *http.Request) error {
			w.Header().Add("X-Trace", "POST /data")
			w.WriteHeader(201)
			return nil
		}},
		local,
	)

	mux.Route(Route{Pattern: "GET /data", Handler: func(w http.ResponseWriter, r *http.Request) error {
		w.Header().Add("X-Trace", "GET /data")
		w.WriteHeader(200)
		return nil
	}})

	t.Run("POST /data: expect 201", func(t *testing.T) {
		res := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/data", nil)
		mux.ServeHTTP(res, req)
		traces := strings.Join(res.Header().Values("X-Trace"), ",")
		expectTrue(t, res.Code == 201)
		expectTrue(t, traces == "global-mid-started,local-mid-started,POST /data,local-mid-ended,global-mid-ended")
	})

	t.Run("GET /data: expect 200", func(t *testing.T) {
		res := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/data", nil)
		mux.ServeHTTP(res, req)
		traces := strings.Join(res.Header().Values("X-Trace"), ",")
		expectTrue(t, res.Code == 200)
		expectTrue(t, traces == "global-mid-started,GET /data,global-mid-ended")
	})
}

func TestServeMux_WithLastResortError(t *testing.T) {
	anError := errors.New("an error")

	t.Run("default", func(t *testing.T) {
		mux := New()
		mux.Route(Route{
			Pattern: "POST /data",
			Handler: func(w http.ResponseWriter, r *http.Request) error { return anError },
		})

		req := httptest.NewRequest("POST", "/data", nil)
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)

		expectTrue(t, res.Code == 500)
	})

	t.Run("custom", func(t *testing.T) {
		mux := New()
		mux.SetLastResortErrorHandler(func(w http.ResponseWriter, r *http.Request, err error) {
			if errors.Is(err, anError) {
				w.WriteHeader(400)
				_, _ = io.WriteString(w, "ErrorResolved")
			}
		})

		mux.Route(Route{
			Pattern: "POST /data",
			Handler: func(w http.ResponseWriter, r *http.Request) error { return anError },
		})

		req := httptest.NewRequest("POST", "/data", nil)
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)

		expectTrue(t, res.Code == 400)
		expectTrue(t, strings.TrimSpace(res.Body.String()) == "ErrorResolved")
	})
}

func expectTrue(t *testing.T, condition bool) {
	t.Helper()
	if !condition {
		t.FailNow()
	}
}
