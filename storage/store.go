package storage

import (
	"io"
	"log"
	"sync"

	"blockstore/config"
)

type BlockStore struct {
	lock   sync.RWMutex
	blocks map[string][config.BlockSize]byte
}

func New() *BlockStore {
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

func (s *BlockStore) Get(id string) ([config.BlockSize]byte, bool) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	block, ok := s.blocks[id]
	return block, ok
}

func (s *BlockStore) Delete(id string) bool {
	s.lock.Lock()
	defer s.lock.Unlock()
	_, ok := s.blocks[id]
	if ok {
		delete(s.blocks, id)
	}
	return ok
}

func ReadBlock(r io.Reader) ([config.BlockSize]byte, error) {
	var block [config.BlockSize]byte
	n, err := io.ReadFull(r, block[:])
	log.Print("read block size: ", n)
	if err != nil {
		return block, err
	}
	if n != config.BlockSize {
		return block, io.ErrShortBuffer
	}
	return block, nil
}
