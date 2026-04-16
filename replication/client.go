package replication

import (
	"blockstore/config"
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"slices"
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

func (c *Client) PutInReplica(replica ReplicaInfo, id string, block [config.BlockSize]byte) error {
	addr := replica.Address + ":" + replica.Port
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

func (c *Client) DeleteInReplica(replica ReplicaInfo, id string) error {
	addr := replica.Address + ":" + replica.Port
	url := fmt.Sprintf("http://%s/internal/block/%s", addr, id)
	log.Printf("Forwarding to: %s", url)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}

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

func (c *Client) PutBlock(blockID string, block [config.BlockSize]byte) error {
	nodes := c.Node.HashRing.GetNodesForBlock(blockID, c.Node.ReplicationFactor)
	log.Printf("Replicating block %s to nodes: %v", blockID, nodes)
	for _, replica := range nodes {
		if replica.Name == c.Node.Name {
			// Local node is one of the nodes responsible for this blockID. So, store it locally
			err := c.Node.Store.Put(blockID, block)
			if err != nil {
				return err
			}
		}
		err := c.PutInReplica(replica, blockID, block)
		if err != nil {
			return fmt.Errorf("replication failed: %w", err)
		}
	}
	return nil
}

func (c *Client) DeleteBlock(blockID string) error {
	nodes := c.Node.HashRing.GetNodesForBlock(blockID, c.Node.ReplicationFactor)
	log.Printf("Replicating block %s to nodes: %v", blockID, nodes)
	for _, replica := range nodes {
		if replica.Name == c.Node.Name {
			// Local node is one of the nodes responsible for this blockID. So, store it locally
			c.Node.Store.Delete(blockID)
		}
		err := c.DeleteInReplica(replica, blockID)
		if err != nil {
			return fmt.Errorf("replication failed: %w", err)
		}
	}
	return nil
}

func (c *Client) GetBlock(blockID string) ([config.BlockSize]byte, error) {
	nodes := c.Node.HashRing.GetNodesForBlock(blockID, c.Node.ReplicationFactor)
	if len(nodes) == 0 {
		return [config.BlockSize]byte{}, fmt.Errorf("no responsible replica nodes found")
	}
	// if the local node is responsible, lookup the local store.
	if slices.ContainsFunc(nodes, func(replica ReplicaInfo) bool { return c.Node.Name == replica.Name }) {
		block, _ := c.Node.Store.Get(blockID)
		return block, nil
	}

	// if the local node is not responsible for this block, fwd the request to the first node in nodes.
	url := nodes[0].Address + ":" + nodes[0].Port
	req, err := http.NewRequest("GET", fmt.Sprintf("http://%s/block/%s", url, blockID), nil)
	if err != nil {
		return [config.BlockSize]byte{}, err
	}
	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return [config.BlockSize]byte{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return [config.BlockSize]byte{}, fmt.Errorf("replica %s returned %d", url, resp.StatusCode)
	}
	block := [config.BlockSize]byte{}
	blockBytes, err := io.ReadAll(resp.Body)
	if err != nil || len(blockBytes) != config.BlockSize {
		return [config.BlockSize]byte{}, err
	}
	block = [4096]byte(blockBytes)
	return block, nil
}

//
//func (c *Client) ReplicateToAll(id string, source string, block [config.BlockSize]byte) error {
//	log.Printf("Replicating from: %s", source)
//	for _, replica := range c.Node.Replicas {
//		if replica.Name == source {
//			continue
//		}
//		err := c.ForwardToReplica(replica, id, block)
//		if err != nil {
//			return fmt.Errorf("replication failed: %w", err)
//		}
//	}
//	return nil
//}
