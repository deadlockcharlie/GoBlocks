package storage

import (
	"context"
	"fmt"
	"sync"
	"time"

	"blockstore/config"
	"blockstore/observability"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

type BlockStore struct {
	lock        sync.RWMutex
	blocks      map[string][config.BlockSize]byte
	hints       map[string][config.BlockSize]byte // TODO: Implement hinted handoff
	staleBlocks map[string]time.Time
}

func NewStore() *BlockStore {
	return &BlockStore{
		blocks:      make(map[string][config.BlockSize]byte),
		hints:       make(map[string][config.BlockSize]byte),
		staleBlocks: make(map[string]time.Time),
	}
}

func (s *BlockStore) Put(ctx context.Context, id string, block [config.BlockSize]byte) error {
	ctx, span := observability.Tracer.Start(ctx, "[Blockstore] PutBlock")
	defer span.End()
	span.SetAttributes(
		attribute.String("[BlockStore] put call for block", id),
	)
	s.lock.Lock()
	defer s.lock.Unlock()
	s.blocks[id] = block
	span.SetStatus(codes.Ok, "[BlockStore] put block successful for id "+id)
	return nil
}

func (s *BlockStore) Get(ctx context.Context, id string) ([config.BlockSize]byte, bool) {
	ctx, span := observability.Tracer.Start(ctx, "[Blockstore] Get Block")
	defer span.End()

	span.SetAttributes(
		attribute.String("[BlockStore] get call for block", id),
	)
	s.lock.RLock()
	defer s.lock.RUnlock()
	block, ok := s.blocks[id]
	return block, ok
}

func (s *BlockStore) Delete(ctx context.Context, id string) {
	ctx, span := observability.Tracer.Start(ctx, "[BlockStore] DeleteBlock")
	defer span.End()

	span.SetAttributes(attribute.String("block.id", id))

	s.lock.Lock()
	defer s.lock.Unlock()
	delete(s.blocks, id)

	span.SetStatus(codes.Ok, "block deleted successfully")
}

func (s *BlockStore) MarkStale(ctx context.Context, id string) {
	ctx, span := observability.Tracer.Start(ctx, "[BlockStore] MarkStale")
	defer span.End()

	span.SetAttributes(attribute.String("block.id", id))

	s.lock.Lock()
	defer s.lock.Unlock()
	if _, exists := s.blocks[id]; exists {
		s.staleBlocks[id] = time.Now()
		span.SetStatus(codes.Ok, "block marked as stale")
	} else {
		span.SetStatus(codes.Ok, "block not found, nothing to mark")
	}
}

func (s *BlockStore) GetStaleBlocks(gracePeriod time.Duration) []string {
	s.lock.RLock()
	defer s.lock.Unlock()

	var stale []string
	now := time.Now()
	for blockId, staleTime := range s.staleBlocks {
		if now.Sub(staleTime) > gracePeriod {
			stale = append(stale, blockId)
		}
	}
	return stale
}

func (s *BlockStore) GarbageCollect(ctx context.Context, gracePeriod time.Duration) int {
	ctx, span := observability.Tracer.Start(ctx, "[BlockStore] GarbageCollect")
	defer span.End()

	staleIDs := s.GetStaleBlocks(gracePeriod)

	span.SetAttributes(
		attribute.Int("stale.count", len(staleIDs)),
		attribute.Float64("grace_period.minutes", gracePeriod.Minutes()),
	)

	s.lock.Lock()
	defer s.lock.Unlock()
	for _, id := range staleIDs {
		delete(s.blocks, id)
		delete(s.staleBlocks, id)
	}

	span.SetStatus(codes.Ok, fmt.Sprintf("garbage collected %d blocks", len(staleIDs)))
	return len(staleIDs)
}

// GetAllBlocks returns all block IDs (for migration calculations)
func (s *BlockStore) GetAllBlocks() []string {
	s.lock.RLock()
	defer s.lock.RUnlock()

	blocks := make([]string, 0, len(s.blocks))
	for id := range s.blocks {
		blocks = append(blocks, id)
	}
	return blocks
}
