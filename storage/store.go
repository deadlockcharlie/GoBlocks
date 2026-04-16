package storage

import (
	"sync"

	"blockstore/config"
)

type BlockStore struct {
	lock   sync.RWMutex
	blocks map[string][config.BlockSize]byte
}

func NewStore() *BlockStore {
	return &BlockStore{
		blocks: make(map[string][config.BlockSize]byte),
	}
}

func (s *BlockStore) Put(id string, block [config.BlockSize]byte) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.blocks[id] = block
	return nil
}

func (s *BlockStore) Get(id string) ([config.BlockSize]byte, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	block := s.blocks[id]
	return block, nil
}

func (s *BlockStore) Delete(id string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	_, ok := s.blocks[id]
	if ok {
		delete(s.blocks, id)
	}
}
