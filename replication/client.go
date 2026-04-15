package replication

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"strings"

	"blockstore/config"
)

type Client struct {
	HttpClient *http.Client
	Node       *Node
}

func NewClient(n *Node) *Client {
	return &Client{
		HttpClient: &http.Client{},
		Node:       n,
	}
}

func (c *Client) ForwardToReplica(addr string, id string, block [config.BlockSize]byte) error {
	url := fmt.Sprintf("http://%s/internal/block/%s", addr, id)
	log.Printf("Forwarding to: %s", url)

	req, err := http.NewRequest("PUT", url, bytes.NewReader(block[:]))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("replica %s returned %d", addr, resp.StatusCode)
	}

	log.Printf("Replica returned: %d", resp.StatusCode)
	return nil
}

func (c *Client) ReplicateToAll(id string, source string, block [config.BlockSize]byte) error {
	log.Printf("Replicating from: %s", source)
	for key, replica := range c.Node.Replicas {
		if replica == "" || strings.Contains(key, source) {
			continue
		}
		err := c.ForwardToReplica(replica, id, block)
		if err != nil {
			return fmt.Errorf("replication failed: %w", err)
		}
	}
	return nil
}
