# ServeMux

The `josestg/servemux` is a wrapper for the `net/http.ServeMux` package that modifies the handler signature to return an 
error and accept optional middleware.

## Requirements

1. Go >= 1.22

## Install

```go
go get github.com/josestg/servemux
```

## Example

```go
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"os"

	"github.com/josestg/servemux"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{}))

	mux := servemux.New()

	mux.SetGlobalMiddlewares(
		logged(log),
		// add more here...
	)

	mux.SetLastResortErrorHandler(func(w http.ResponseWriter, r *http.Request, err error) {
		ctx := r.Context()
		log.ErrorContext(ctx, "unhandled error", "error", err)

		body := map[string]any{"msg": "please wait a moment!"}
		w.WriteHeader(http.StatusInternalServerError)
		if wErr := json.NewEncoder(w).Encode(body); wErr != nil {
			log.ErrorContext(ctx, "cannot write generic response", "write_error", err)
		}
	})

	mux.HandleFunc("GET /func/hello/{name}", helloHandler)                               // register using servemux.HandlerFunc
	mux.Handle("GET /handler/hello/{name}", servemux.HandlerFunc(helloHandler))          //  or using servemux.Handle
	mux.Route(servemux.Route{Pattern: "GET /route/hello/{name}", Handler: helloHandler}) // or using servemux.Route

	log.Info("server is started")
	if err := http.ListenAndServe(":8081", mux); err != nil {
		log.Error("listen and serve failed", "error", err)
	}
}

func logged(log *slog.Logger) servemux.Middleware {
	return func(h servemux.Handler) servemux.Handler {
		return servemux.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
			l := log.With("method", r.Method, "url", r.URL)
			if err := h.ServeHTTP(w, r); err != nil {
				l.ErrorContext(r.Context(), "request failed", "error", err)
			} else {
				l.InfoContext(r.Context(), "request succeeded")
			}
			return nil
		})
	}
}

func helloHandler(w http.ResponseWriter, r *http.Request) error {
	// simulate error.
	n := rand.Intn(10)
	if n%2 == 1 {
		return errors.New("a dummy error")
	}
	_, err := fmt.Fprintf(w, "Hi, %s!", r.PathValue("name"))
	return err
}
```