package replication

import (
	"blockstore/config"
	"blockstore/observability"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"slices"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
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

func (c *Client) PutInReplica(ctx context.Context, replica ReplicaInfo, id string, block [config.BlockSize]byte) error {
	ctx, span := observability.Tracer.Start(ctx, "PutInReplica")
	defer span.End()

	addr := replica.Address + ":" + replica.Port
	url := fmt.Sprintf("http://%s/internal/block/%s", addr, id)

	span.SetAttributes(
		attribute.String("block.id", id),
		attribute.String("target.replica", replica.Name),
		attribute.String("target.address", addr),
	)

	log.Printf("Forwarding to: %s", url)

	req, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewReader(block[:]))
	if err != nil {
		span.RecordError(err)
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	// Inject trace context into HTTP headers
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

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

func (c *Client) DeleteInReplica(ctx context.Context, replica ReplicaInfo, id string) error {
	ctx, span := observability.Tracer.Start(ctx, "DeleteInReplica")
	defer span.End()

	addr := replica.Address + ":" + replica.Port
	url := fmt.Sprintf("http://%s/internal/block/%s", addr, id)

	span.SetAttributes(
		attribute.String("block.id", id),
		attribute.String("target.replica", replica.Name),
		attribute.String("target.address", addr),
	)

	log.Printf("Forwarding to: %s", url)

	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		span.RecordError(err)
		return err
	}

	// Inject trace context into HTTP headers
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "HTTP request failed")
		return err
	}
	defer resp.Body.Close()

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("replica %s returned %d", addr, resp.StatusCode)
		span.RecordError(err)
		span.SetStatus(codes.Error, "deletion failed")
		return err
	}

	span.SetStatus(codes.Ok, "deletion successful")
	log.Printf("Replica returned: %d", resp.StatusCode)
	return nil
}

func (c *Client) PutBlock(ctx context.Context, blockID string, block [config.BlockSize]byte) error {
	ctx, span := observability.Tracer.Start(ctx, "ReplicationClient")
	defer span.End()

	nodes := c.Node.HashRing.GetNodesForBlock(blockID, c.Node.ReplicationFactor)

	replicationTargets := make([]string, 0, len(nodes))
	for _, replica := range nodes {
		replicationTargets = append(replicationTargets, replica.Name)
	}
	span.SetAttributes(
		attribute.String("block.id", blockID),
		attribute.StringSlice("replication.targets", replicationTargets),
	)

	// log.Printf("Replicating block %s to nodes: %v", blockID, nodes)
	for _, replica := range nodes {
		if replica.Name == c.Node.Name {
			// Local node is responsible: store locally, no HTTP call needed
			err := c.Node.Store.Put(ctx, blockID, block)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, "local storage failed")
				return err
			}
			continue
		}
		err := c.PutInReplica(ctx, replica, blockID, block)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "replication failed")
			return fmt.Errorf("replication failed: %w", err)
		}
	}

	span.SetStatus(codes.Ok, "replication successful")
	return nil
}

func (c *Client) DeleteBlock(ctx context.Context, blockID string) error {
	ctx, span := observability.Tracer.Start(ctx, "DeleteBlock")
	defer span.End()

	nodes := c.Node.HashRing.GetNodesForBlock(blockID, c.Node.ReplicationFactor)

	nodeNames := make([]string, len(nodes))
	for i, node := range nodes {
		nodeNames[i] = node.Name
	}

	span.SetAttributes(
		attribute.String("block.id", blockID),
		attribute.StringSlice("deletion.targets", nodeNames),
		attribute.Int("deletion.target_count", len(nodes)),
	)

	log.Printf("Deleting block %s from nodes: %v", blockID, nodes)

	for _, replica := range nodes {
		if replica.Name == c.Node.Name {
			c.Node.Store.Delete(ctx, blockID)
			continue
		}
		err := c.DeleteInReplica(ctx, replica, blockID)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "deletion failed")
			return fmt.Errorf("deletion failed: %w", err)
		}
	}

	span.SetStatus(codes.Ok, "deletion successful")
	return nil
}

func (c *Client) GetBlock(ctx context.Context, blockID string) ([config.BlockSize]byte, error) {
	ctx, span := observability.Tracer.Start(ctx, "GetBlock")
	defer span.End()

	span.SetAttributes(attribute.String("block.id", blockID))

	nodes := c.Node.HashRing.GetNodesForBlock(blockID, c.Node.ReplicationFactor)
	if len(nodes) == 0 {
		err := fmt.Errorf("no responsible replica nodes found")
		span.RecordError(err)
		span.SetStatus(codes.Error, "no replicas found")
		return [config.BlockSize]byte{}, err
	}

	nodeNames := make([]string, len(nodes))
	for i, node := range nodes {
		nodeNames[i] = node.Name
	}
	span.SetAttributes(attribute.StringSlice("responsible.replicas", nodeNames))

	// if the local node is responsible, lookup the local store.
	if slices.ContainsFunc(nodes, func(replica ReplicaInfo) bool { return c.Node.Name == replica.Name }) {
		span.SetAttributes(attribute.Bool("local.read", true))
		block, ok := c.Node.Store.Get(ctx, blockID)
		if !ok {
			err := fmt.Errorf("block %s not found", blockID)
			span.RecordError(err)
			span.SetStatus(codes.Error, "block not found")
			return [config.BlockSize]byte{}, err
		}
		span.SetStatus(codes.Ok, "block retrieved locally")
		return block, nil
	}

	// if the local node is not responsible for this block, fwd the request to the first node in nodes.
	span.SetAttributes(
		attribute.Bool("local.read", false),
		attribute.String("forward.to", nodes[0].Name),
	)

	url := nodes[0].Address + ":" + nodes[0].Port
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://%s/block/%s", url, blockID), nil)
	if err != nil {
		span.RecordError(err)
		return [config.BlockSize]byte{}, err
	}

	// Inject trace context
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "forward request failed")
		return [config.BlockSize]byte{}, err
	}
	defer resp.Body.Close()

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("replica %s returned %d", url, resp.StatusCode)
		span.RecordError(err)
		span.SetStatus(codes.Error, "forward failed")
		return [config.BlockSize]byte{}, err
	}

	block := [config.BlockSize]byte{}
	blockBytes, err := io.ReadAll(resp.Body)
	if err != nil || len(blockBytes) != config.BlockSize {
		if err != nil {
			span.RecordError(err)
		}
		span.SetStatus(codes.Error, "invalid block size")
		return [config.BlockSize]byte{}, err
	}

	block = [4096]byte(blockBytes)
	span.SetStatus(codes.Ok, "block retrieved via forward")
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
