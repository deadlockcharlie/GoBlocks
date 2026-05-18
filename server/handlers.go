package server

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"blockstore/config"
	"blockstore/observability"
	"blockstore/replication"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
)

type Handler struct {
	replClient *replication.Client
}

func NewHandler(replClient *replication.Client) *Handler {
	return &Handler{
		replClient: replClient,
	}
}

func (h *Handler) Metrics(w http.ResponseWriter, r *http.Request) {
	promhttp.Handler().ServeHTTP(w, r)

}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	ctx, span := observability.Tracer.Start(r.Context(), "Health")
	defer span.End()

	span.SetAttributes(
		attribute.String("replica.name", h.replClient.Node.Name),
	)
	observability.HealthCount.Add(ctx, 1)
	span.SetStatus(codes.Ok, "Health endpoint responded successfully")
	fmt.Fprintln(w, "ok")
}

func (h *Handler) PutBlock(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span := observability.Tracer.Start(r.Context(), "PutBlock")
	defer span.End()
	defer func() {
		duration := float64(time.Since(start).Milliseconds())
		observability.PutDuration.Record(ctx, duration)
	}()

	id := strings.TrimPrefix(r.URL.Path, "/block/")
	span.SetAttributes(
		attribute.String("replica.name", h.replClient.Node.Name),
		attribute.String("block.id", id),
	)
	observability.PutCount.Add(ctx, 1)
	block, err := parseBlock(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Block size must be %d bytes", config.BlockSize), http.StatusBadRequest)
		span.SetStatus(codes.Error, "[PutBlock] Incorrect block size. Expected 4KB")
		observability.ErrorCount.Add(ctx, 1, metric.WithAttributes(attribute.String("operation", "put"), attribute.String("error", "invalid_size")))
		return
	}

	if h.replClient != nil {
		err = h.replClient.PutBlock(ctx, id, block)
		if err != nil {
			log.Printf("Replication failed: %v", err)
			// TODO: rollback or implement proper 2PC - known limitation for now
			http.Error(w, "Replication failed", http.StatusServiceUnavailable)
			span.SetStatus(codes.Error, "[PutBlock] Replication failed")
			observability.ErrorCount.Add(ctx, 1, metric.WithAttributes(attribute.String("operation", "put"), attribute.String("error", "replication_failed")))
			return
		}
	}

	observability.BlocksStored.Add(ctx, 1)
	w.WriteHeader(http.StatusCreated)
	span.SetStatus(codes.Ok, "[PutBlock] successful for block id "+id)
}

func (h *Handler) GetBlock(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span := observability.Tracer.Start(r.Context(), "GetBlock")
	defer span.End()
	defer func() {
		duration := float64(time.Since(start).Milliseconds())
		observability.GetDuration.Record(ctx, duration)
	}()

	span.SetAttributes(attribute.String("replica.Name", h.replClient.Node.Name))
	observability.GetCount.Add(ctx, 1)
	id := strings.TrimPrefix(r.URL.Path, "/block/")

	block, err := h.replClient.GetBlock(ctx, id)
	if err != nil {
		http.Error(w, "Block with this id does not exist", http.StatusNotFound)
		span.SetStatus(codes.Error, "[GetBlock] Block with this id does not exist.")
		observability.ErrorCount.Add(ctx, 1, metric.WithAttributes(attribute.String("operation", "get"), attribute.String("error", "not_found")))
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(block[:])
	span.SetStatus(codes.Ok, "[GetBlock] successful for block id "+id)
}

func (h *Handler) DeleteBlock(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	id := strings.TrimPrefix(r.URL.Path, "/block/")
	ctx, span := observability.Tracer.Start(r.Context(), "DeleteBlock")
	defer span.End()
	defer func() {
		duration := float64(time.Since(start).Milliseconds())
		observability.DeleteDuration.Record(ctx, duration)
	}()

	span.SetAttributes(attribute.String("replica.Name", h.replClient.Node.Name))
	observability.DeleteCount.Add(ctx, 1)
	err := h.replClient.DeleteBlock(ctx, id)
	if err != nil {
		http.Error(w, "block not found", http.StatusNotFound)
		span.SetStatus(codes.Error, "[DeleteBlock] Block with this id does not exist")
		observability.ErrorCount.Add(ctx, 1, metric.WithAttributes(attribute.String("operation", "delete"), attribute.String("error", "not_found")))
		return
	}

	observability.BlocksStored.Add(ctx, -1)
	w.WriteHeader(http.StatusNoContent)
	span.SetStatus(codes.Ok, "[GetBlock] successful for block id "+id)
}

func (h *Handler) InternalPutBlock(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/internal/block/")
	ctx, span := observability.Tracer.Start(r.Context(), "InternalPutBlock")
	span.SetAttributes(attribute.String("replica.Name", h.replClient.Node.Name))
	observability.InternalPutCount.Add(ctx, 1)
	log.Printf("Internal PUT for block: %s", id)

	block, err := parseBlock(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Block size must be %d bytes", config.BlockSize), http.StatusBadRequest)
		span.SetStatus(codes.Error, "[InternalPutBlock] Incorrect block size. Expected 4KB")
		return
	}

	err = h.replClient.Node.Store.Put(ctx, id, block)
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
	ctx, span := observability.Tracer.Start(r.Context(), "InternalDeleteBlock")
	span.SetAttributes(attribute.String("replica.Name", h.replClient.Node.Name))
	observability.InternalDeleteCount.Add(ctx, 1)
	h.replClient.Node.Store.Delete(ctx, id)
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
