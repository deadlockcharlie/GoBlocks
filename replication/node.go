package replication

import (
	"blockstore/config"
	"blockstore/storage"
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"maps"
	"net/http"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-zookeeper/zk"
)

type ReplicaInfo struct {
	Name    string
	Address string
	Port    string
}

func (info *ReplicaInfo) String() string {
	return info.Name + "$" + info.Address + ":" + info.Port
}

func ToMap(data string) (ReplicaInfo, error) {
	nameAddr := strings.Split(data, "$")
	if len(nameAddr) != 2 {
		return ReplicaInfo{}, errors.New("invalid replica info")
	}
	name := nameAddr[0]
	address := strings.Split(nameAddr[1], ":")[0]
	port := strings.Split(nameAddr[1], ":")[1]
	return ReplicaInfo{
		Name:    name,
		Address: address,
		Port:    port,
	}, nil
}

// TODO: If there is a topology change during migration, mark a migration pending that needs to start when this cycle ends.

type Node struct {
	Name              string
	Address           string
	Port              string
	Replicas          map[string]ReplicaInfo
	Connection        *zk.Conn
	ringMutex         sync.RWMutex
	ReplicationFactor int
	HashRing          *HashRing
	Store             *storage.BlockStore

	MigrationActive  bool
	MigrationEpoch   int64
	PendingMigration bool // This tracks if a migration is required due to topology changes during an existing migration cycle.
}

const (
	NodeBasePath = "/nodes"

	MigrationBasePath         = "/migration"
	MigrationBarrierPath      = "/migration/barrier"
	MigrationParticipantsPath = "/migration/participants"
	MigrationEpochPath        = "/migration/epoch"
	MigrationGCReadyPath      = "/migration/gc-ready"
)

func NewNode(cfg *config.Config) (*Node, error) {

	conn, _, err := zk.Connect([]string{cfg.ZKAddress}, time.Second*5)

	if err != nil {
		return nil, err
	}
	// Connect to zookeeper. If the node is the first to join a cluster, it creates the base path
	// Future nodes fetch this path from zookeeper and use the node addresses to forward the operation.
	err = createPath(conn, "/nodes")
	if err != nil {
		return nil, err
	}
	ring := NewHashRing()
	Store := storage.NewStore()

	node := &Node{
		Name:              cfg.ReplicaName,
		Address:           cfg.ReplicaAddress,
		Port:              cfg.ReplicaPort,
		ReplicationFactor: cfg.ReplicationFactor,
		Replicas:          map[string]ReplicaInfo{},
		Connection:        conn,
		HashRing:          ring,
		Store:             Store,
	}

	err = node.registerNode(conn, node)
	if err != nil {
		return nil, err
	}

	//discover nodes in the ring and register a watcher:
	err = node.discoverReplicas(conn)
	if err != nil {
		return nil, err
	}

	return node, nil
}

// Creates a path in the zookeeper registry for a replica
func createPath(conn *zk.Conn, path string) error {
	parts := strings.Split(path, "/")
	currentPath := ""

	for _, part := range parts {
		if part == "" {
			continue
		}

		currentPath += "/" + part
		exists, _, err := conn.Exists(currentPath)
		if err != nil {
			return err
		}
		if !exists {
			_, err := conn.Create(currentPath, []byte(""), 0, zk.WorldACL(zk.PermAll))
			if err != nil && !errors.Is(err, zk.ErrNodeExists) {
				return err
			}
			log.Printf("Created path :%s", currentPath)
		}
	}
	return nil

}

func (n *Node) registerNode(conn *zk.Conn, node *Node) error {
	nodePath := NodeBasePath + "/" + node.Name
	//// Ensure that the node name exists
	//if err := createPath(conn, nodePath); err != nil {
	//	return err
	//}
	// registrationPath := nodePath + "/node-"
	data := ReplicaInfo{
		Name:    node.Name,
		Address: node.Address,
		Port:    node.Port,
	}

	//[]byte(node.Address + ":" + node.Port)
	log.Printf("Registering node with data %s", (data).String())
	// An ephemeral node is deleted when a connection terminates.
	createdPath, err := conn.CreateProtectedEphemeralSequential(nodePath, []byte(data.String()), zk.WorldACL(zk.PermAll))
	if err != nil {
		return err
	}
	log.Printf("Node joined the ring %s at path: %s", createdPath, data)
	return nil
}

func (n *Node) discoverReplicas(conn *zk.Conn) error {
	exists, _, err := conn.Exists(NodeBasePath)
	if err != nil {
		return err
	}

	if !exists {
		return nil
	}

	nodes, _, err := conn.Children(NodeBasePath)
	for _, node := range nodes {
		n.Replicas[node] = ReplicaInfo{}
	}
	if err != nil {
		return err
	}
	n.resolveReplicas(conn)
	log.Printf("Found %v nodes from the ring: %v", len(nodes), n.Replicas)
	go n.watchReplicas(conn)

	return nil

}

func (n *Node) watchReplicas(conn *zk.Conn) {
	for {
		_, _, events, err := conn.ChildrenW(NodeBasePath)
		if err != nil {
			log.Printf("Failed to watch nodes: %s", err)
			return
		}
		event := <-events
		if event.Type == zk.EventNodeChildrenChanged {
			nodes, _, err := conn.Children(NodeBasePath)

			if err != nil {
				log.Printf("Failed to fetch nodes on ring change: %s", err)
				continue
			}
			log.Printf("Found nodes on ring change: %v", nodes)
			for _, node := range nodes {
				n.Replicas[node] = ReplicaInfo{}
			}
			n.resolveReplicas(conn)

			if n.MigrationActive {
				log.Printf("Ring change detected during migration. Deferring until stable. ")
				n.PendingMigration = true
				continue
			}

			if n.shouldStartMigration() {
				epoch := time.Now().Unix()
				log.Printf("Ring topology change detected, starting migration")
				if err := n.initMigration(epoch); err != nil {
					log.Printf("Failed to init migration: %v", err)
					continue
				}
				go n.performMigration()
			}
		}
	}
}

// The hashring is populated everytime we resolve replicas. This is the only function where the ring is modified.
func (n *Node) resolveReplicas(conn *zk.Conn) {
	//var resolvedReplicas []string
	for replica, _ := range n.Replicas {
		data, _, err := conn.Get(NodeBasePath + "/" + replica)
		if err != nil {
			log.Printf("Failed to resolve replica %s: %s", replica, err)
			n.Replicas[replica] = ReplicaInfo{}
		}

		dataMap, e := ToMap(string(data))
		if e != nil {
			log.Printf("Failed to resolve replica %s: %s", replica, err)
		}
		n.Replicas[replica] = dataMap
	}
	maps.DeleteFunc(n.Replicas, func(k string, v ReplicaInfo) bool { return v.Address == "" })
	for _, replica := range n.Replicas {
		n.HashRing.ResolveVNodes(&replica)
	}

	log.Printf("Added to vnodes: %v", n.HashRing.VNodes)

}

func (n *Node) initMigration(epoch int64) error {
	paths := []string{
		MigrationBasePath,
		MigrationParticipantsPath,
	}

	for _, path := range paths {
		if err := createPath(n.Connection, path); err != nil {
			return err
		}
	}
	// Create a barrier (to be deleted later)
	_, err := n.Connection.Create(MigrationBarrierPath, []byte{}, 0, zk.WorldACL(zk.PermAll))
	if err != nil && !errors.Is(err, zk.ErrNodeExists) {
		return err
	}

	// Update the local metadata for migration
	n.MigrationActive = true
	n.MigrationEpoch = epoch
	n.HashRing.StartRebalancing(epoch)
	log.Printf("Migration %d started", epoch)
	return nil
}

// Check if migration should start.
func (n *Node) shouldStartMigration() bool {
	if n.MigrationActive {
		return false
	}

	// Check if the topology changed.
	if len(n.HashRing.OldVNodes) != len(n.HashRing.VNodes) {
		return true
	}
	oldSet := make(map[uint32]bool)
	for _, vnode := range n.HashRing.OldVNodes {
		oldSet[vnode.Hash] = true
	}
	for _, vnode := range n.HashRing.VNodes {
		if !oldSet[vnode.Hash] {
			return true
		}
	}
	return false
}

func (n *Node) completeMigration() {
	n.MigrationActive = false
	n.HashRing.CommitRebalancing()

	if n.PendingMigration {
		log.Printf("Pending migration detected, starting migration again")
		n.PendingMigration = false

		if n.shouldStartMigration() {
			epoch := time.Now().Unix()
			if err := n.initMigration(epoch); err != nil {
				log.Printf("Failed to start ring migration on a pending migration demand")
				return
			}
			go n.performMigration()
		}
	}
}

func (n *Node) performMigration() {
	log.Printf("Node %v starting data transfer for migration", n.Name)
	allBlocks := n.Store.GetAllBlocks()
	log.Printf("Found %d blocks to check for migration", len(allBlocks))

	transferCount := 0
	staleCount := 0
	for _, blockID := range allBlocks {
		oldOwners := n.getOwnersInRing(blockID, n.HashRing.OldVNodes)
		newOwners := n.getOwnersInRing(blockID, n.HashRing.VNodes)

		stillOwner := false
		for _, owner := range newOwners {
			if owner.Name == n.Name {
				stillOwner = true
				break
			}
		}

		if !stillOwner {
			// THe node is not an owner anymore, so transfer to the new owner.
			n.Store.MarkStale(context.Background(), blockID) // Mark the node stale to be garbage collected later
			staleCount++
			log.Printf("Block %s marked stale. Will be garbage collected", blockID)
		}

		// Transfer to the new owner that didn't have ownership before

		for _, newOwner := range newOwners {
			if newOwner.Name == n.Name {
				continue
			}

			// check if this owner had this block before?
			hadItBefore := false
			for _, oldOwner := range oldOwners {
				if oldOwner.Name == newOwner.Name {
					hadItBefore = true
					break
				}
			}

			if !hadItBefore {
				block, ok := n.Store.Get(context.Background(), blockID)

				if !ok {
					log.Printf("Block %s disappeared during migration. Fatal error", blockID)
					continue
				}
				if err := n.transferBlock(newOwner, blockID, block); err != nil {
					log.Printf("Failed to transfer block %s to %s: %v", blockID, newOwner.Name, err)

				} else {
					transferCount++
				}

			}
		}
	}
	log.Printf("Migration transfer complete: %d blocks transferred, %d marked stale", transferCount, staleCount)

	// Mark ourselves as ready in ZooKeeper
	n.markMigrationReady()

	// Start watching for barrier deletion (coordinator will delete when all ready)
	go n.watchMigrationBarrier()
}

// getOwnersInRing calculates block owners using a specific vnode ring
func (n *Node) getOwnersInRing(blockID string, vnodes []VNode) []ReplicaInfo {
	if len(vnodes) == 0 {
		return nil
	}

	blockHash := getHash(blockID)
	var nodes []ReplicaInfo

	startIndex := sort.Search(len(vnodes), func(i int) bool {
		return vnodes[i].Hash >= blockHash
	})
	if startIndex >= len(vnodes) {
		startIndex = 0
	}

	currentIndex := startIndex
	visited := 0

	for len(nodes) < n.ReplicationFactor && visited < len(vnodes) {
		vnode := vnodes[currentIndex]

		if !slices.ContainsFunc(nodes, func(node ReplicaInfo) bool {
			return vnode.Node.Name == node.Name
		}) {
			nodes = append(nodes, *vnode.Node)
		}

		currentIndex = (currentIndex + 1) % len(vnodes)
		visited++
	}

	return nodes
}

// transferBlock sends a block to a new owner via internal API
func (n *Node) transferBlock(replica ReplicaInfo, blockID string, block [config.BlockSize]byte) error {
	// You already have PutInReplica in client.go - we can reuse it
	// But for clarity, let's make it explicit this is a migration transfer
	addr := replica.Address + ":" + replica.Port
	url := fmt.Sprintf("http://%s/internal/block/%s", addr, blockID)

	req, err := http.NewRequest("PUT", url, bytes.NewReader(block[:]))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("replica %s returned %d", addr, resp.StatusCode)
	}

	return nil
}

func (n *Node) markMigrationReady() error {
	participantPath := MigrationParticipantsPath + "/" + n.Name
	_, stat, err := n.Connection.Get(participantPath)
	if err != nil {
		return fmt.Errorf("Failed to fetch participant path from zookeper")
	}
	_, err = n.Connection.Set(participantPath, []byte("ready"), stat.Version)
	if err != nil {
		return fmt.Errorf("Failed to mark ready: %v", err)
	}
	log.Printf("Marked as ready in ZooKeeper for migration epoch %d", n.MigrationEpoch)
	go n.tryCoordinate()
	return nil
}

// tryCoordinate attempts to coordinate barrier deletion if all participants are ready
func (n *Node) tryCoordinate() {
	// Simple coordination: any node can check and delete barrier if all ready
	// (ZooKeeper ensures atomic deletion)

	time.Sleep(2 * time.Second) // Small delay to let other nodes update

	participants, _, err := n.Connection.Children(MigrationParticipantsPath)
	if err != nil {
		log.Printf("Failed to get participants: %v", err)
		return
	}

	allReady := true
	for _, participant := range participants {
		data, _, err := n.Connection.Get(MigrationParticipantsPath + "/" + participant)
		if err != nil {
			log.Printf("Failed to get participant %s status: %v", participant, err)
			allReady = false
			break
		}

		if string(data) != "ready" {
			log.Printf("Participant %s not ready yet: %s", participant, string(data))
			allReady = false
			break
		}
	}

	if allReady {
		log.Printf("All %d participants ready - deleting barrier", len(participants))

		// Delete barrier to trigger ring swap
		err := n.Connection.Delete(MigrationBarrierPath, -1)
		if err != nil && !errors.Is(err, zk.ErrNoNode) {
			log.Printf("Failed to delete barrier: %v", err)
			return
		}

		log.Printf("Barrier deleted - migration epoch %d ready to swap", n.MigrationEpoch)
	}
}

// watchMigrationBarrier watches for barrier deletion to trigger ring swap
func (n *Node) watchMigrationBarrier() {
	for {
		exists, _, events, err := n.Connection.ExistsW(MigrationBarrierPath)
		if err != nil {
			log.Printf("Failed to watch barrier: %v", err)
			return
		}

		if !exists {
			// Barrier already deleted - swap immediately
			log.Printf("Barrier deleted - swapping to new ring")
			n.swapRing()
			return
		}

		// Wait for barrier deletion
		event := <-events
		if event.Type == zk.EventNodeDeleted {
			log.Printf("Barrier deletion detected - swapping to new ring")
			n.swapRing()
			return
		}
	}
}

// swapRing atomically commits to the new ring topology
func (n *Node) swapRing() {
	log.Printf("Swapping to new ring for migration epoch %d", n.MigrationEpoch)

	// Commit the ring swap
	n.completeMigration()

	log.Printf("Ring swap complete - now serving with new topology")

	// Cleanup ZooKeeper migration state
	go n.cleanupMigration()

	// Start garbage collection after grace period
	go n.scheduleGarbageCollection()
}

// cleanupMigration removes migration znodes
func (n *Node) cleanupMigration() {
	// Remove our participant entry
	participantPath := MigrationParticipantsPath + "/" + n.Name
	err := n.Connection.Delete(participantPath, -1)
	if err != nil && !errors.Is(err, zk.ErrNoNode) {
		log.Printf("Failed to cleanup participant node: %v", err)
	}

	// Try to cleanup migration base path (will fail if others still present - that's ok)
	// Last node to cleanup will succeed
	time.Sleep(1 * time.Second)

	children, _, _ := n.Connection.Children(MigrationParticipantsPath)
	if len(children) == 0 {
		// We're the last one - create GC ready signal
		_, err := n.Connection.Create(MigrationGCReadyPath, []byte{}, zk.FlagEphemeral, zk.WorldACL(zk.PermAll))
		if err != nil && !errors.Is(err, zk.ErrNodeExists) {
			log.Printf("Failed to create GC ready signal: %v", err)
		} else {
			log.Printf("Created GC ready signal")
		}
	}
}

// scheduleGarbageCollection waits for grace period then GCs stale blocks
func (n *Node) scheduleGarbageCollection() {
	gracePeriod := 5 * time.Minute // Configurable

	log.Printf("Scheduling GC after %v grace period", gracePeriod)
	time.Sleep(gracePeriod)

	count := n.Store.GarbageCollect(context.Background(), gracePeriod)
	log.Printf("Garbage collection complete: removed %d stale blocks", count)
}
func (n *Node) Shutdown() {
	n.Connection.Close()
}
