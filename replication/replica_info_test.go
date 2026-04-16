package replication

import (
	"testing"
)

func TestReplicaInfo_String(t *testing.T) {
	r := ReplicaInfo{Name: "node1", Address: "127.0.0.1", Port: "3001"}
	expected := "node1$127.0.0.1:3001"
	if r.String() != expected {
		t.Errorf("expected %s, got %s", expected, r.String())
	}
}

func TestToMap_ValidInput(t *testing.T) {
	input := "node1$127.0.0.1:3001"
	r, err := ToMap(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Name != "node1" {
		t.Errorf("expected name node1, got %s", r.Name)
	}
	if r.Address != "127.0.0.1" {
		t.Errorf("expected address 127.0.0.1, got %s", r.Address)
	}
	if r.Port != "3001" {
		t.Errorf("expected port 3001, got %s", r.Port)
	}
}

func TestToMap_InvalidInput_NoDelimiter(t *testing.T) {
	_, err := ToMap("node1:127.0.0.1:3001")
	if err == nil {
		t.Error("expected error for input without $ delimiter")
	}
}

func TestToMap_InvalidInput_Empty(t *testing.T) {
	_, err := ToMap("")
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestToMap_RoundTrip(t *testing.T) {
	original := ReplicaInfo{Name: "node2", Address: "192.168.1.1", Port: "4000"}
	serialized := original.String()
	deserialized, err := ToMap(serialized)
	if err != nil {
		t.Fatalf("round trip failed: %v", err)
	}
	if deserialized.Name != original.Name ||
		deserialized.Address != original.Address ||
		deserialized.Port != original.Port {
		t.Errorf("round trip mismatch: got %+v, want %+v", deserialized, original)
	}
}

func TestToMap_SpecialCharacters(t *testing.T) {
	// Port or address could have edge cases
	input := "node-with-dash$10.0.0.1:8080"
	r, err := ToMap(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Name != "node-with-dash" {
		t.Errorf("expected node-with-dash, got %s", r.Name)
	}
	if r.Port != "8080" {
		t.Errorf("expected port 8080, got %s", r.Port)
	}
}

