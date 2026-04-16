package server

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"blockstore/config"
	"blockstore/replication"
	"blockstore/storage"
)

// buildTestClient creates a replication.Client backed by a local node with a hash ring
// containing only itself, so all blocks are stored locally without network calls.
func buildTestClient(nodeName string) *replication.Client {
	store := storage.NewStore()
	ring := replication.NewHashRing()
	replica := &replication.ReplicaInfo{Name: nodeName, Address: "127.0.0.1", Port: "9999"}
	ring.ResolveVNodes(replica)

	node := &replication.Node{
		Name:              nodeName,
		Address:           "127.0.0.1",
		Port:              "9999",
		ReplicationFactor: 1,
		Replicas:          map[string]replication.ReplicaInfo{},
		HashRing:          ring,
		Store:             store,
	}
	return replication.NewClient(node)
}

func makeTestBlock(fill byte) [config.BlockSize]byte {
	var b [config.BlockSize]byte
	for i := range b {
		b[i] = fill
	}
	return b
}

// ----- Health endpoint -----

func TestHealthEndpoint(t *testing.T) {
	handler := NewHandler(buildTestClient("node1"))
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handler.Health(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "ok\n" {
		t.Errorf("expected 'ok', got %s", w.Body.String())
	}
}

// ----- PutBlock -----

func TestPutBlock_ValidBlock(t *testing.T) {
	client := buildTestClient("node1")
	handler := NewHandler(client)

	block := makeTestBlock(0xAB)
	req := httptest.NewRequest(http.MethodPut, "/block/test1", bytes.NewReader(block[:]))
	w := httptest.NewRecorder()

	handler.PutBlock(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201 Created, got %d", w.Code)
	}
}

func TestPutBlock_InvalidBlockSize(t *testing.T) {
	handler := NewHandler(buildTestClient("node1"))

	req := httptest.NewRequest(http.MethodPut, "/block/test1", bytes.NewReader([]byte("too small")))
	w := httptest.NewRecorder()

	handler.PutBlock(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestPutBlock_EmptyBody(t *testing.T) {
	handler := NewHandler(buildTestClient("node1"))

	req := httptest.NewRequest(http.MethodPut, "/block/test1", bytes.NewReader([]byte{}))
	w := httptest.NewRecorder()

	handler.PutBlock(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for empty body, got %d", w.Code)
	}
}

// ----- GetBlock -----

func TestGetBlock_ExistingBlock(t *testing.T) {
	client := buildTestClient("node1")
	block := makeTestBlock(0xCC)
	if err := client.Node.Store.Put("myblock", block); err != nil {
		t.Fatalf("setup Put failed: %v", err)
	}

	handler := NewHandler(client)
	req := httptest.NewRequest(http.MethodGet, "/block/myblock", nil)
	w := httptest.NewRecorder()

	handler.GetBlock(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if len(w.Body.Bytes()) != config.BlockSize {
		t.Errorf("expected %d bytes, got %d", config.BlockSize, len(w.Body.Bytes()))
	}
}

func TestGetBlock_NonExistentBlock(t *testing.T) {
	handler := NewHandler(buildTestClient("node1"))
	req := httptest.NewRequest(http.MethodGet, "/block/nonexistent", nil)
	w := httptest.NewRecorder()

	handler.GetBlock(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ----- DeleteBlock -----

func TestDeleteBlock_ExistingBlock(t *testing.T) {
	client := buildTestClient("node1")
	block := makeTestBlock(0x01)
	if err := client.Node.Store.Put("delme", block); err != nil {
		t.Fatalf("setup Put failed: %v", err)
	}

	handler := NewHandler(client)
	req := httptest.NewRequest(http.MethodDelete, "/block/delme", nil)
	w := httptest.NewRecorder()

	handler.DeleteBlock(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

// ----- InternalPutBlock -----

func TestInternalPutBlock_ValidBlock(t *testing.T) {
	client := buildTestClient("node1")
	handler := NewHandler(client)
	block := makeTestBlock(0xBB)

	req := httptest.NewRequest(http.MethodPut, "/internal/block/internal1", bytes.NewReader(block[:]))
	w := httptest.NewRecorder()

	handler.InternalPutBlock(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	got, _ := client.Node.Store.Get("internal1")
	if got != block {
		t.Error("block was not stored correctly via internal PUT")
	}
}

func TestInternalPutBlock_InvalidSize(t *testing.T) {
	handler := NewHandler(buildTestClient("node1"))
	req := httptest.NewRequest(http.MethodPut, "/internal/block/internal1", bytes.NewReader([]byte("short")))
	w := httptest.NewRecorder()

	handler.InternalPutBlock(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ----- InternalDeleteBlock -----

func TestInternalDeleteBlock(t *testing.T) {
	client := buildTestClient("node1")
	block := makeTestBlock(0xDD)
	if err := client.Node.Store.Put("todelete", block); err != nil {
		t.Fatalf("setup Put failed: %v", err)
	}

	handler := NewHandler(client)
	req := httptest.NewRequest(http.MethodDelete, "/internal/block/todelete", nil)
	w := httptest.NewRecorder()

	handler.InternalDeleteBlock(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
	got, _ := client.Node.Store.Get("todelete")
	var empty [config.BlockSize]byte
	if got != empty {
		t.Error("block should be empty after internal delete")
	}
}

// ----- parseBlock -----

func TestParseBlock_CorrectSize(t *testing.T) {
	block := makeTestBlock(0xEE)
	got, err := parseBlock(bytes.NewReader(block[:]))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != block {
		t.Error("parseBlock returned wrong block data")
	}
}

func TestParseBlock_TooSmall(t *testing.T) {
	_, err := parseBlock(bytes.NewReader([]byte("too small")))
	if err == nil {
		t.Error("expected error for too-small block")
	}
}

func TestParseBlock_TooBig(t *testing.T) {
	big := make([]byte, config.BlockSize+100)
	// io.ReadFull reads exactly BlockSize, so this should succeed (extra bytes ignored)
	_, err := parseBlock(bytes.NewReader(big))
	if err != nil {
		t.Errorf("unexpected error for oversized reader: %v", err)
	}
}

// ----- Integration: full PUT → GET → DELETE cycle -----

func TestHandlerIntegration_PutGetDelete(t *testing.T) {
	client := buildTestClient("node1")
	handler := NewHandler(client)

	blockID := "integration-block"
	block := makeTestBlock(0x42)

	// PUT
	putReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/block/%s", blockID), bytes.NewReader(block[:]))
	putW := httptest.NewRecorder()
	handler.PutBlock(putW, putReq)
	if putW.Code != http.StatusCreated {
		t.Fatalf("PUT failed with status %d", putW.Code)
	}

	// GET
	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/block/%s", blockID), nil)
	getW := httptest.NewRecorder()
	handler.GetBlock(getW, getReq)
	if getW.Code != http.StatusOK {
		t.Fatalf("GET failed with status %d", getW.Code)
	}

	var gotBlock [config.BlockSize]byte
	copy(gotBlock[:], getW.Body.Bytes())
	if gotBlock != block {
		t.Error("GET returned different block than PUT")
	}

	// DELETE
	delReq := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/block/%s", blockID), nil)
	delW := httptest.NewRecorder()
	handler.DeleteBlock(delW, delReq)
	if delW.Code != http.StatusNoContent {
		t.Fatalf("DELETE failed with status %d", delW.Code)
	}

	// GET after DELETE should be 404
	getReq2 := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/block/%s", blockID), nil)
	getW2 := httptest.NewRecorder()
	handler.GetBlock(getW2, getReq2)
	if getW2.Code != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", getW2.Code)
	}
}

func TestHandlerIntegration_MultipleBlocks(t *testing.T) {
	client := buildTestClient("node1")
	handler := NewHandler(client)

	blocks := map[string]byte{
		"block-a": 0x01,
		"block-b": 0x02,
		"block-c": 0x03,
	}

	// PUT all blocks
	for id, fill := range blocks {
		block := makeTestBlock(fill)
		req := httptest.NewRequest(http.MethodPut, "/block/"+id, bytes.NewReader(block[:]))
		w := httptest.NewRecorder()
		handler.PutBlock(w, req)
		if w.Code != http.StatusCreated {
			t.Errorf("PUT %s failed with status %d", id, w.Code)
		}
	}

	// GET all blocks and verify
	for id, fill := range blocks {
		expected := makeTestBlock(fill)
		req := httptest.NewRequest(http.MethodGet, "/block/"+id, nil)
		w := httptest.NewRecorder()
		handler.GetBlock(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("GET %s failed with status %d", id, w.Code)
		}
		var got [config.BlockSize]byte
		copy(got[:], w.Body.Bytes())
		if got != expected {
			t.Errorf("block %s has wrong content", id)
		}
	}
}

