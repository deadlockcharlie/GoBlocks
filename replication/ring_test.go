package replication

import (
	"testing"
)


// ----- AddNode / ResolveVNodes tests -----

func TestHashRing_Empty(t *testing.T) {
	ring := NewHashRing()
	nodes := ring.GetNodesForBlock("someblock", 3)
	if nodes != nil {
		t.Error("empty ring should return nil")
	}
}

func TestHashRing_AddOneNode(t *testing.T) {
	ring := NewHashRing()
	r := &ReplicaInfo{Name: "node1", Address: "127.0.0.1", Port: "3001"}
	ring.ResolveVNodes(r)

	if len(ring.VNodes) != VnodeCountPerNode {
		t.Errorf("expected %d vnodes, got %d", VnodeCountPerNode, len(ring.VNodes))
	}
}

func TestHashRing_AddMultipleNodes(t *testing.T) {
	ring := NewHashRing()
	for _, name := range []string{"node1", "node2", "node3"} {
		ring.ResolveVNodes(&ReplicaInfo{Name: name, Address: "127.0.0.1", Port: "3001"})
	}
	if len(ring.VNodes) != VnodeCountPerNode*3 {
		t.Errorf("expected %d vnodes, got %d", VnodeCountPerNode*3, len(ring.VNodes))
	}
}

func TestHashRing_IsSorted(t *testing.T) {
	ring := NewHashRing()
	ring.ResolveVNodes(&ReplicaInfo{Name: "node1"})
	ring.ResolveVNodes(&ReplicaInfo{Name: "node2"})

	for i := 1; i < len(ring.VNodes); i++ {
		if ring.VNodes[i].Hash < ring.VNodes[i-1].Hash {
			t.Errorf("ring vnodes are not sorted at index %d", i)
		}
	}
}

func TestHashRing_NoDuplicateVNodes(t *testing.T) {
	ring := NewHashRing()
	r := &ReplicaInfo{Name: "node1"}
	// Adding the same node twice should not add duplicate vnodes
	ring.ResolveVNodes(r)
	ring.ResolveVNodes(r)
	if len(ring.VNodes) != VnodeCountPerNode {
		t.Errorf("expected %d vnodes after adding same node twice, got %d", VnodeCountPerNode, len(ring.VNodes))
	}
}

// ----- GetNodesForBlock tests -----

func TestGetNodesForBlock_SingleNode(t *testing.T) {
	ring := NewHashRing()
	r := &ReplicaInfo{Name: "node1", Address: "127.0.0.1", Port: "3001"}
	ring.ResolveVNodes(r)

	nodes := ring.GetNodesForBlock("block-abc", 1)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].Name != "node1" {
		t.Errorf("expected node1, got %s", nodes[0].Name)
	}
}

func TestGetNodesForBlock_ReplicationFactor(t *testing.T) {
	ring := NewHashRing()
	for _, name := range []string{"node1", "node2", "node3"} {
		ring.ResolveVNodes(&ReplicaInfo{Name: name, Address: "127.0.0.1", Port: "3001"})
	}

	nodes := ring.GetNodesForBlock("block-xyz", 2)
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}
}

func TestGetNodesForBlock_UniquePhysicalNodes(t *testing.T) {
	ring := NewHashRing()
	for _, name := range []string{"node1", "node2", "node3"} {
		ring.ResolveVNodes(&ReplicaInfo{Name: name, Address: "127.0.0.1", Port: "3001"})
	}

	nodes := ring.GetNodesForBlock("block-unique", 3)
	seen := make(map[string]bool)
	for _, n := range nodes {
		if seen[n.Name] {
			t.Errorf("duplicate physical node in replication set: %s", n.Name)
		}
		seen[n.Name] = true
	}
}

func TestGetNodesForBlock_ReplicationFactorExceedsNodes(t *testing.T) {
	ring := NewHashRing()
	ring.ResolveVNodes(&ReplicaInfo{Name: "node1"})
	ring.ResolveVNodes(&ReplicaInfo{Name: "node2"})

	// Requesting more replicas than available nodes
	nodes := ring.GetNodesForBlock("block-overflow", 5)
	if len(nodes) > 2 {
		t.Errorf("should not return more unique nodes than available, got %d", len(nodes))
	}
}

func TestGetNodesForBlock_Deterministic(t *testing.T) {
	ring := NewHashRing()
	for _, name := range []string{"node1", "node2", "node3"} {
		ring.ResolveVNodes(&ReplicaInfo{Name: name})
	}

	blockID := "stable-block"
	first := ring.GetNodesForBlock(blockID, 2)
	second := ring.GetNodesForBlock(blockID, 2)

	if len(first) != len(second) {
		t.Fatal("GetNodesForBlock is not deterministic")
	}
	for i := range first {
		if first[i].Name != second[i].Name {
			t.Errorf("GetNodesForBlock returned different nodes: %v vs %v", first, second)
		}
	}
}

func TestGetNodesForBlock_WrapAround(t *testing.T) {
	ring := NewHashRing()
	ring.ResolveVNodes(&ReplicaInfo{Name: "node1"})
	ring.ResolveVNodes(&ReplicaInfo{Name: "node2"})

	// This block should resolve to something regardless of its position on the ring
	nodes := ring.GetNodesForBlock("zzzzzzzzzzzzzzzzzzzzzzzz", 1)
	if len(nodes) == 0 {
		t.Error("expected at least one node for any block, wrap-around may be broken")
	}
}

func TestGetNodesForBlock_EmptyBlockID(t *testing.T) {
	ring := NewHashRing()
	ring.ResolveVNodes(&ReplicaInfo{Name: "node1"})

	// Should not panic on empty block ID
	nodes := ring.GetNodesForBlock("", 1)
	if len(nodes) == 0 {
		t.Error("expected 1 node for empty block ID")
	}
}

func TestGetNodesForBlock_ZeroReplicationFactor(t *testing.T) {
	ring := NewHashRing()
	ring.ResolveVNodes(&ReplicaInfo{Name: "node1"})

	nodes := ring.GetNodesForBlock("block", 0)
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes for replication factor 0, got %d", len(nodes))
	}
}

func TestHashRing_SameBlockSameNode(t *testing.T) {
	ring := NewHashRing()
	ring.ResolveVNodes(&ReplicaInfo{Name: "node1"})
	ring.ResolveVNodes(&ReplicaInfo{Name: "node2"})
	ring.ResolveVNodes(&ReplicaInfo{Name: "node3"})

	blockID := "consistent-block"
	n1 := ring.GetNodesForBlock(blockID, 1)
	n2 := ring.GetNodesForBlock(blockID, 1)

	if n1[0].Name != n2[0].Name {
		t.Errorf("same block should always map to same node: got %s vs %s", n1[0].Name, n2[0].Name)
	}
}

