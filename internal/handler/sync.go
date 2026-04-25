package handler

import (
	"net/http"
	"sync/atomic"
)

// SyncHandler provides an HTTP handler to trigger registry sync.
type SyncHandler struct {
	triggerSync func()
	running     atomic.Bool
}

// NewSyncHandler creates a handler that invokes triggerSync on POST.
func NewSyncHandler(triggerSync func()) *SyncHandler {
	return &SyncHandler{triggerSync: triggerSync}
}

// Trigger handles POST /api/v1/sync.
func (h *SyncHandler) Trigger(w http.ResponseWriter, _ *http.Request) {
	if !h.running.CompareAndSwap(false, true) {
		writeJSON(w, http.StatusConflict, envelope{
			Data: map[string]string{"message": "sync already in progress"},
		})
		return
	}
	go func() {
		defer h.running.Store(false)
		h.triggerSync()
	}()
	writeJSON(w, http.StatusAccepted, envelope{
		Data: map[string]string{"message": "sync triggered"},
	})
}
