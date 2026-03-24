package replication

import (
	"bytes"
	"fmt"
	"log"
	"net/http"

	"blockstore/config"
)

type Client struct {
	httpClient *http.Client
	replicas   []string
}

func NewClient(replicas []string) *Client {
	return &Client{
		httpClient: &http.Client{},
		replicas:   replicas,
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

	resp, err := c.httpClient.Do(req)
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

func (c *Client) ReplicateToAll(id string, block [config.BlockSize]byte) error {
	for _, replica := range c.replicas {
		err := c.ForwardToReplica(replica, id, block)
		if err != nil {
			return fmt.Errorf("replication to %s failed: %w", replica, err)
		}
	}
	return nil
}
