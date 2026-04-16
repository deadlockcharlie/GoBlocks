package replication

import (
	"blockstore/config"
	"blockstore/storage"
	"errors"
	"log"
	"maps"
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
}

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
	nodePath := "/nodes/" + node.Name
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
	basePath := "/nodes"
	exists, _, err := conn.Exists(basePath)
	if err != nil {
		return err
	}

	if !exists {
		return nil
	}

	nodes, _, err := conn.Children(basePath)
	for _, node := range nodes {
		n.Replicas[node] = ReplicaInfo{}
	}
	if err != nil {
		return err
	}
	n.resolveReplicas(conn)
	log.Printf("Found %v nodes from the ring: %v", len(nodes), n.Replicas)
	go n.watchReplicas(conn, basePath)

	return nil

}

func (n *Node) watchReplicas(conn *zk.Conn, basePath string) {
	for {
		_, _, events, err := conn.ChildrenW(basePath)
		if err != nil {
			log.Printf("Failed to watch nodes: %s", err)
			return
		}
		event := <-events
		if event.Type == zk.EventNodeChildrenChanged {
			nodes, _, err := conn.Children("/nodes")

			if err != nil {
				log.Printf("Failed to fetch nodes on ring change: %s", err)
				continue
			}
			log.Printf("Found nodes on ring change: %v", nodes)
			for _, node := range nodes {
				n.Replicas[node] = ReplicaInfo{}
			}
			n.resolveReplicas(conn)
		}
	}
}

// The hashring is populated everytime we resolve replicas. This is the only function where the ring is modified.
func (n *Node) resolveReplicas(conn *zk.Conn) {
	//var resolvedReplicas []string
	for replica, _ := range n.Replicas {
		data, _, err := conn.Get("/nodes/" + replica)
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

func (n *Node) Shutdown() {
	n.Connection.Close()
}
