package replication

import (
	"testing"
)

// ----- getHash tests -----

func TestGetHash_Deterministic(t *testing.T) {
	id := "node1-vnode-0"
	if getHash(id) != getHash(id) {
		t.Error("hash should be deterministic")
	}
}

func TestGetHash_DifferentInputs(t *testing.T) {
	ids := []string{"node1-vnode-0", "node1-vnode-1", "node2-vnode-0", "abc", ""}
	hashes := make(map[uint32]bool)
	collisions := 0
	for _, id := range ids {
		h := getHash(id)
		if hashes[h] {
			collisions++
		}
		hashes[h] = true
	}
	// Collisions are possible but should be very rare for small distinct inputs
	if collisions > 1 {
		t.Errorf("Too many hash collisions: %d", collisions)
	}
}

func TestGetHash_EmptyString(t *testing.T) {
	// Should not panic
	h := getHash("")
	if h == 0 {
		// SHA256 of empty string is non-zero, first 4 bytes will be non-zero
		t.Error("Hash of empty string should not be zero")
	}
}

// ----- VNode tests -----

func TestNewVNode_HashDependsOnName(t *testing.T) {
	r1 := &ReplicaInfo{Name: "nodeA"}
	r2 := &ReplicaInfo{Name: "nodeB"}
	v1 := NewVNode(r1, 0)
	v2 := NewVNode(r2, 0)
	if v1.Hash == v2.Hash {
		t.Error("vnodes for different nodes should have different hashes")
	}
}

func TestNewVNode_HashDependsOnIndex(t *testing.T) {
	r := &ReplicaInfo{Name: "node1"}
	v0 := NewVNode(r, 0)
	v1 := NewVNode(r, 1)
	if v0.Hash == v1.Hash {
		t.Error("vnodes with different indices should have different hashes")
	}
}

func TestNewVNode_NodePointer(t *testing.T) {
	r := &ReplicaInfo{Name: "node1", Address: "127.0.0.1", Port: "3001"}
	v := NewVNode(r, 0)
	if v.Node != r {
		t.Error("vnode should hold correct pointer to its physical node")
	}
}

func TestNewVNode_HashConsistency(t *testing.T) {
	r := &ReplicaInfo{Name: "node1"}
	v1 := NewVNode(r, 5)
	v2 := NewVNode(r, 5)
	if v1.Hash != v2.Hash {
		t.Error("creating the same vnode twice should produce the same hash")
	}
}

