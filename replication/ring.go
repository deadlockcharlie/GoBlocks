package replication

import (
	"log"
	"slices"
	"sort"
)

const VnodeCountPerNode = 2

type HashRing struct {
	VNodes []VNode
}

func NewHashRing() *HashRing {
	return &HashRing{
		VNodes: []VNode{},
	}
}

func (ring *HashRing) ResolveVNodes(node *ReplicaInfo) {
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
	if ring.VNodes == nil || len(ring.VNodes) == 0 {
		return nil
	}

	blockHash := getHash(blockID)
	log.Print("Block hash is ", blockHash)
	var nodes []ReplicaInfo
	// THis search is implemented as a range function over the list. This can be improved by making the ring into a tree.
	// TODO: Implement a tree structure to better search the vnodes. R-B tree or B-Tree
	startIndex := sort.Search(len(ring.VNodes), func(i int) bool {
		return ring.VNodes[i].Hash >= blockHash
	})
	if startIndex >= len(ring.VNodes) {
		startIndex = 0
	}
	currentIndex := startIndex
	visited := 0 // safety counter to prevent infinite loops.

	for len(nodes) < replicationFactor && visited < len(ring.VNodes) {
		vnode := ring.VNodes[currentIndex]

		// Only add if not already in the list
		if !slices.ContainsFunc(nodes, func(node ReplicaInfo) bool {
			return vnode.Node.Name == node.Name
		}) {
			nodes = append(nodes, *vnode.Node)
		}

		// Increment and wrap around
		currentIndex = (currentIndex + 1) % len(ring.VNodes)
		visited++
	}

	return nodes
}
