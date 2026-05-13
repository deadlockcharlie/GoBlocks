package server

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"blockstore/config"
	"blockstore/replication"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
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
	ctx, span := tracer.Start(r.Context(), "Health")
	defer span.End()

	span.SetAttributes(
		attribute.String("replica.name", h.replClient.Node.Name),
	)
	HealthCount.Add(ctx, 1)
	span.SetStatus(codes.Ok, "Health endpoint responded successfully")
	fmt.Fprintln(w, "ok")
}

func (h *Handler) PutBlock(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "PutBlock")
	defer span.End()

	span.SetAttributes(
		attribute.String("replica.name", h.replClient.Node.Name),
	)
	PutCount.Add(ctx, 1)

	id := strings.TrimPrefix(r.URL.Path, "/block/")

	block, err := parseBlock(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Block size must be %d bytes", config.BlockSize), http.StatusBadRequest)
		span.SetStatus(codes.Error, "[PutBlock] Incorrect block size. Expected 4KB")
		return
	}

	if h.replClient != nil {
		err = h.replClient.PutBlock(id, block)
		if err != nil {
			log.Printf("Replication failed: %v", err)
			// TODO: rollback or implement proper 2PC - known limitation for now
			http.Error(w, "Replication failed", http.StatusServiceUnavailable)
			span.SetStatus(codes.Error, "[PutBlock] Replication failed")
			return
		}
	}

	w.WriteHeader(http.StatusCreated)
	span.SetStatus(codes.Ok, "[PutBlock] successful for block id "+id)
}

func (h *Handler) GetBlock(w http.ResponseWriter, r *http.Request) {

	ctx, span := tracer.Start(r.Context(), "GetBlock")
	defer span.End()

	span.SetAttributes(attribute.String("replica.Name", h.replClient.Node.Name))
	GetCount.Add(ctx, 1)
	id := strings.TrimPrefix(r.URL.Path, "/block/")

	block, err := h.replClient.GetBlock(id)
	if err != nil {
		http.Error(w, "Block with this id does not exist", http.StatusNotFound)
		span.SetStatus(codes.Error, "[GetBlock] Block with this id does not exist.")

		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(block[:])
	span.SetStatus(codes.Ok, "[GetBlock] successful for block id "+id)
}

func (h *Handler) DeleteBlock(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/block/")
	ctx, span := tracer.Start(r.Context(), "DeleteBlock")
	defer span.End()
	span.SetAttributes(attribute.String("replica.Name", h.replClient.Node.Name))
	DeleteCount.Add(ctx, 1)
	err := h.replClient.DeleteBlock(id)
	if err != nil {
		http.Error(w, "block not found", http.StatusNotFound)
		span.SetStatus(codes.Error, "[DeleteBlock] Block with this id does not exist")
		return
	}

	w.WriteHeader(http.StatusNoContent)
	span.SetStatus(codes.Ok, "[GetBlock] successful for block id "+id)
}

func (h *Handler) InternalPutBlock(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/internal/block/")
	ctx, span := tracer.Start(r.Context(), "InternalPutBlock")
	span.SetAttributes(attribute.String("replica.Name", h.replClient.Node.Name))
	InternalPutCount.Add(ctx, 1)
	log.Printf("Internal PUT for block: %s", id)

	block, err := parseBlock(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Block size must be %d bytes", config.BlockSize), http.StatusBadRequest)
		span.SetStatus(codes.Error, "[InternalPutBlock] Incorrect block size. Expected 4KB")
		return
	}

	err = h.replClient.Node.Store.Put(id, block)
	if err != nil {
		http.Error(w, "Failed to write block", http.StatusInternalServerError)
		span.SetStatus(codes.Error, "[InternalPutBlock] Block replication failed")
		return
	}

	w.WriteHeader(http.StatusOK)
	span.SetStatus(codes.Ok, "[InternalPutBlock] successful for block id "+id)
}

func (h *Handler) InternalDeleteBlock(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/internal/block/")
	ctx, span := tracer.Start(r.Context(), "InternalDeleteBlock")
	span.SetAttributes(attribute.String("replica.Name", h.replClient.Node.Name))
	InternalDeleteCount.Add(ctx, 1)
	h.replClient.Node.Store.Delete(id)
	span.SetStatus(codes.Ok, "[InternalDeleteBlock] successful for id"+id)
	w.WriteHeader(http.StatusOK)
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
