package replication

import (
	"errors"
	"log"
	"strings"
	"time"

	"github.com/go-zookeeper/zk"
)

type Node struct {
	Name       string   `json:"name" validate:"required"`
	Address    string   `json:"address" validate:"required"`
	Port       string   `json:"port" validate:"required"`
	Replicas   []string `json:"replicas"`
	Connection *zk.Conn `json:"-"`
}

func New(name string, address string, port string) (*Node, error) {

	conn, _, err := zk.Connect([]string{address}, time.Second*5)

	if err != nil {
		return nil, err
	}
	// Connect to zookeeper. If the node is the first to join a cluster, it creates the base path
	// Future nodes fetch this path from zookeeper and use the node addresses to forward the operation.
	err = createPath(conn, "/nodes")
	if err != nil {
		return nil, err
	}
	node := &Node{
		Name:       name,
		Address:    address,
		Port:       port,
		Replicas:   []string{},
		Connection: conn,
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
	// Ensure that the node name exists
	if err := createPath(conn, nodePath); err != nil {
		return err
	}
	// registrationPath := nodePath + "/node-"
	data := []byte(node.Address + ":" + node.Port)
	// An ephemeral node is deleted when a connection terminates.
	createdPath, err := conn.CreateProtectedEphemeralSequential(nodePath, data, zk.WorldACL(zk.PermAll))
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
	n.Replicas = nodes
	if err != nil {
		return err
	}
	log.Printf("Found %v nodes from the ring: %v", len(nodes), nodes)
	go n.watchNodes(conn, basePath)

	return nil

}

func (n *Node) watchNodes(conn *zk.Conn, basePath string) {
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
			n.Replicas = nodes
		}
	}
}

func (n *Node) Shutdown() {
	n.Connection.Close()
}
