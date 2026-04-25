package handler

import "net/http"

// SyncHandler provides an HTTP handler to trigger registry sync.
type SyncHandler struct {
	triggerSync func()
}

// NewSyncHandler creates a handler that invokes triggerSync on POST.
func NewSyncHandler(triggerSync func()) *SyncHandler {
	return &SyncHandler{triggerSync: triggerSync}
}

// Trigger handles POST /api/v1/sync.
func (h *SyncHandler) Trigger(w http.ResponseWriter, _ *http.Request) {
	go h.triggerSync()
	writeJSON(w, http.StatusAccepted, envelope{
		Data: map[string]string{"message": "sync triggered"},
	})
}
