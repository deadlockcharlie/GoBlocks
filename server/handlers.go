package server

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"blockstore/config"
	"blockstore/replication"
	"blockstore/storage"
)

type Handler struct {
	store      *storage.BlockStore
	replClient *replication.Client
}

func NewHandler(store *storage.BlockStore, replClient *replication.Client) *Handler {
	return &Handler{
		store:      store,
		replClient: replClient,
	}
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "ok")
}

func (h *Handler) PutBlock(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/block/")

	block, err := storage.ReadBlock(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Block size must be %d bytes", config.BlockSize), http.StatusBadRequest)
		return
	}

	// Write to local store first
	err = h.store.Put(id, block)
	if err != nil {
		http.Error(w, "Failed to write block", http.StatusInternalServerError)
		return
	}

	// If primary, replicate to followers
	if h.replClient != nil {
		err = h.replClient.ReplicateToAll(id, h.replClient.Node.Name, block)
		if err != nil {
			log.Printf("Replication failed: %v", err)
			// TODO: rollback or implement proper 2PC - known limitation for now
			http.Error(w, "Replication failed", http.StatusServiceUnavailable)
			return
		}
	}

	w.WriteHeader(http.StatusCreated)
}

func (h *Handler) GetBlock(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/block/")

	block, ok := h.store.Get(id)
	if !ok {
		http.Error(w, "Block with this id does not exist", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(block[:])
}

func (h *Handler) DeleteBlock(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/block/")

	ok := h.store.Delete(id)
	if !ok {
		http.Error(w, "block not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) InternalPutBlock(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/internal/block/")
	log.Printf("Internal PUT for block: %s", id)

	block, err := storage.ReadBlock(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Block size must be %d bytes", config.BlockSize), http.StatusBadRequest)
		return
	}

	err = h.store.Put(id, block)
	if err != nil {
		http.Error(w, "Failed to write block", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) InternalDeleteBlock(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/internal/block/")

	ok := h.store.Delete(id)
	if !ok {
		http.Error(w, "block not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
