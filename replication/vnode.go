package replication

import (
	"crypto/sha256"
	"encoding/binary"
	"strconv"
)

type VNode struct {
	Hash uint32
	Node *ReplicaInfo
}

func getHash(identifier string) uint32 {
	hasher := sha256.New()
	hasher.Write([]byte(identifier))
	return binary.BigEndian.Uint32(hasher.Sum(nil)[0:4])
}

func NewVNode(node *ReplicaInfo, vnodeNo int) VNode {
	hash := getHash(node.Name + "-vnode-" + strconv.Itoa(vnodeNo))

	return VNode{
		Hash: hash,
		Node: node,
	}
}
