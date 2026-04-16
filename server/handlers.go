package server

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"blockstore/config"
	"blockstore/replication"
)

type Handler struct {
	replClient *replication.Client
}

func NewHandler(replClient *replication.Client) *Handler {
	return &Handler{
		replClient: replClient,
	}
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "ok")
}

func (h *Handler) PutBlock(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/block/")

	block, err := parseBlock(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Block size must be %d bytes", config.BlockSize), http.StatusBadRequest)
		return
	}

	if h.replClient != nil {
		err = h.replClient.PutBlock(id, block)
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

	block, err := h.replClient.GetBlock(id)
	if err != nil {
		http.Error(w, "Block with this id does not exist", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(block[:])
}

func (h *Handler) DeleteBlock(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/block/")

	err := h.replClient.DeleteBlock(id)
	if err != nil {
		http.Error(w, "block not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) InternalPutBlock(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/internal/block/")
	log.Printf("Internal PUT for block: %s", id)

	block, err := parseBlock(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Block size must be %d bytes", config.BlockSize), http.StatusBadRequest)
		return
	}

	err = h.replClient.Node.Store.Put(id, block)
	if err != nil {
		http.Error(w, "Failed to write block", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) InternalDeleteBlock(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/internal/block/")

	h.replClient.Node.Store.Delete(id)
	w.WriteHeader(http.StatusNoContent)
}

func parseBlock(r io.Reader) ([config.BlockSize]byte, error) {
	var block [config.BlockSize]byte
	n, err := io.ReadFull(r, block[:])
	log.Print("read block size: ", n)
	if err != nil {
		return block, err
	}
	if n != config.BlockSize {
		return block, io.ErrShortBuffer
	}
	return block, nil
}
