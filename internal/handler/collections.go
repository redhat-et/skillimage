package handler

import (
	"errors"
	"net/http"

	"github.com/redhat-et/skillimage/internal/store"
)

type CollectionsHandler struct {
	store *store.Store
}

func NewCollectionsHandler(s *store.Store) *CollectionsHandler {
	return &CollectionsHandler{store: s}
}

func (h *CollectionsHandler) List(w http.ResponseWriter, r *http.Request) {
	collections, err := h.store.ListCollections()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "listing collections", err)
		return
	}
	if collections == nil {
		collections = []store.Collection{}
	}
	writeJSON(w, http.StatusOK, envelope{Data: collections})
}

func (h *CollectionsHandler) Get(w http.ResponseWriter, r *http.Request, name string) {
	col, err := h.store.GetCollection(name)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "collection not found", err)
			return
		}
		writeError(w, http.StatusInternalServerError, "getting collection", err)
		return
	}
	writeJSON(w, http.StatusOK, col)
}
