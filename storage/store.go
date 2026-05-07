package storage

import (
	"sync"
	"time"

	"blockstore/config"
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

func (s *BlockStore) Put(id string, block [config.BlockSize]byte) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.blocks[id] = block
	return nil
}

func (s *BlockStore) Get(id string) ([config.BlockSize]byte, bool) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	block, ok := s.blocks[id]
	return block, ok
}

func (s *BlockStore) Delete(id string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	delete(s.blocks, id)
}

func (s *BlockStore) MarkStale(id string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if _, exists := s.blocks[id]; exists {
		s.staleBlocks[id] = time.Now()
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

func (s *BlockStore) GarbageCollect(gracePeriod time.Duration) int {
	staleIDs := s.GetStaleBlocks(gracePeriod)
	s.lock.Lock()
	defer s.lock.Unlock()
	for _, id := range staleIDs {
		delete(s.blocks, id)
		delete(s.staleBlocks, id)
	}
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
