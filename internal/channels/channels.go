package channels

import (
	"context"
	"net/http"
)

type Channel interface {
	Name() string
	// RegisterRoutes adds webhook/HTTP handlers to the mux (optional, not all channels need this).
	RegisterRoutes(mux *http.ServeMux)
	// Start begins polling or listening. Blocks until ctx is cancelled.
	Start(ctx context.Context) error
}
