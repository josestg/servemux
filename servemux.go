package servemux

import (
	"net/http"
)

// Middleware wraps the Handler with additional logic.
type Middleware func(Handler) Handler

// Then chains the middleware with the handler.
func (m Middleware) Then(h Handler) Handler { return m(h) }

// FoldMiddleware folds set of middlewares into a single middleware.
// For example:
//
//	FoldMiddleware(m1, m2, m3).Then(h)
//	will be equivalent to:
//	m1(m2(m3(h)))
func FoldMiddleware(middlewares ...Middleware) Middleware {
	return foldMiddlewares(middlewares)
}

func foldMiddlewares(middlewares []Middleware) Middleware {
	return func(next Handler) Handler {
		for i := len(middlewares) - 1; i >= 0; i-- {
			next = middlewares[i](next)
		}
		return next
	}
}

// Handler is modified version of http.Handler.
type Handler interface {
	// ServeHTTP is just like http.Handler.ServeHTTP, but it returns an error.
	ServeHTTP(http.ResponseWriter, *http.Request) error
}

// HandlerFunc is a function that implements Handler.
// It is used to create a Handler from an ordinary function.
type HandlerFunc func(http.ResponseWriter, *http.Request) error

// ServeHTTP implements Handler.
func (f HandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) error { return f(w, r) }

// LastResortErrorHandler is the error handler that is called if after all middlewares,
// there is still an error occurs.
type LastResortErrorHandler func(http.ResponseWriter, *http.Request, error)

// Route is used to register a new handler to the ServeMux.
type Route struct {
	Pattern string
	Handler HandlerFunc
}

// ServeMux is a wrapper of net/http.ServeMux with modified Handler.
// Instead of http.Handler, it uses Handler, which returns an error. This modification is used to simplify logic for
// creating a centralized error handler and logging.
type ServeMux struct {
	core *http.ServeMux
	midl Middleware

	// lastResort is the error handler that is called if after all middlewares,
	// there is still an error occurs. This handler is used to catch errors that are not handled by the middlewares.
	//
	// This handler is not part of the httprouter.Router, it is used by the ServeMux.
	lastResort LastResortErrorHandler
}

// New creates a new ServeMux.
func New() *ServeMux {
	mux := ServeMux{
		core:       http.NewServeMux(),
		midl:       func(h Handler) Handler { return h },
		lastResort: nil,
	}

	return &mux
}

// SetGlobalMiddlewares sets middleware that will be applied to all registered routes.
func (mux *ServeMux) SetGlobalMiddlewares(middlewares ...Middleware) {
	if len(middlewares) > 0 {
		mux.midl = foldMiddlewares(middlewares)
	}
}

// SetLastResortErrorHandler sets the last resort handler.
func (mux *ServeMux) SetLastResortErrorHandler(h LastResortErrorHandler) {
	if h != nil {
		mux.lastResort = h
	}
}

// Route is a syntactic sugar for Handle(method, path, handler) by using Route struct.
// This route also accepts variadic Middleware, which is applied to the route handler.
func (mux *ServeMux) Route(r Route, middlewares ...Middleware) {
	chain := foldMiddlewares(middlewares)
	mux.Handle(r.Pattern, chain.Then(r.Handler))
}

// HandleFunc just like Handle, but it accepts HandlerFunc.
func (mux *ServeMux) HandleFunc(pattern string, handler HandlerFunc) {
	mux.Handle(pattern, handler)
}

// Handle registers a new request handler with the given method and path.
func (mux *ServeMux) Handle(pattern string, handler Handler) {
	mux.core.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		err := mux.midl.Then(handler).ServeHTTP(w, r)
		if err != nil {
			if mux.lastResort != nil {
				mux.lastResort(w, r, err)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		}
	})
}

// ServeHTTP satisfies http.Handler.
func (mux *ServeMux) ServeHTTP(w http.ResponseWriter, r *http.Request) { mux.core.ServeHTTP(w, r) }
