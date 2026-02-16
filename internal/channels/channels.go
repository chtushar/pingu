package channels

import "net/http"

type Channel interface {
	Name() string
	RegisterRoutes(mux *http.ServeMux)
}
