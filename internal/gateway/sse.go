package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type SSEWriter struct {
	w  http.ResponseWriter
	rc *http.ResponseController
}

func NewSSEWriter(w http.ResponseWriter) *SSEWriter {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	return &SSEWriter{
		w:  w,
		rc: http.NewResponseController(w),
	}
}

func (s *SSEWriter) Send(event string, data any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event, b)
	return s.rc.Flush()
}
