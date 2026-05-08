package replication

import (
	"log"
	"slices"
	"sort"
	"sync"
)

const VnodeCountPerNode = 2

type HashRing struct {
	// TODO: In all the places these are used, make sure that migration switches them around correctly.

	VNodes    []VNode
	OldVNodes []VNode

	lock             sync.RWMutex
	Rebalancing      bool
	RebalancingEpoch int64
}

func NewHashRing() *HashRing {
	return &HashRing{
		VNodes:           []VNode{},
		OldVNodes:        []VNode{},
		Rebalancing:      false,
		RebalancingEpoch: 0,
	}
}

func (ring *HashRing) ResolveVNodes(node *ReplicaInfo) {
	ring.OldVNodes = ring.VNodes
	for i := 0; i < VnodeCountPerNode; i++ {
		vnode := NewVNode(node, i)
		if slices.ContainsFunc(ring.VNodes, func(v VNode) bool { return v.Hash == vnode.Hash }) {
			continue
		}
		ring.VNodes = append(ring.VNodes, NewVNode(node, i))
	}
	sort.Slice(ring.VNodes, func(i, j int) bool {
		return ring.VNodes[i].Hash < ring.VNodes[j].Hash
	})
}

func (ring *HashRing) GetNodesForBlock(blockID string, replicationFactor int) []ReplicaInfo {
	activeRing := ring.GetActiveRing()
	if len(activeRing) == 0 {
		return nil
	}

	blockHash := getHash(blockID)
	log.Print("Block hash is ", blockHash)
	var nodes []ReplicaInfo
	// This search is implemented as a range function over the list. This can be improved by making the ring into a tree.
	// TODO: Implement a tree structure to better search the vnodes. R-B tree or B-Tree
	startIndex := sort.Search(len(activeRing), func(i int) bool {
		return activeRing[i].Hash >= blockHash
	})
	if startIndex >= len(activeRing) {
		startIndex = 0
	}
	currentIndex := startIndex
	visited := 0 // safety counter to prevent infinite loops.

	for len(nodes) < replicationFactor && visited < len(activeRing) {
		vnode := activeRing[currentIndex]

		// Only add if not already in the list
		if !slices.ContainsFunc(nodes, func(node ReplicaInfo) bool {
			return vnode.Node.Name == node.Name
		}) {
			nodes = append(nodes, *vnode.Node)
		}

		// Increment and wrap around
		currentIndex = (currentIndex + 1) % len(activeRing)
		visited++
	}

	return nodes
}

func (ring *HashRing) StartRebalancing(epoch int64) {
	ring.lock.Lock()
	defer ring.lock.Unlock()

	ring.Rebalancing = true
	ring.RebalancingEpoch = epoch
}

func (ring *HashRing) CommitRebalancing() {
	ring.lock.Lock()
	defer ring.lock.Unlock()

	ring.Rebalancing = false
}

func (ring *HashRing) GetActiveRing() []VNode {
	ring.lock.RLock()
	defer ring.lock.RUnlock()

	if ring.Rebalancing {
		return ring.OldVNodes
	}
	return ring.VNodes
}
