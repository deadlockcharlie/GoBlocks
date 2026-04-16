package storage

import (
	"blockstore/config"
	"testing"
)

func makeBlock(fill byte) [config.BlockSize]byte {
	var b [config.BlockSize]byte
	for i := range b {
		b[i] = fill
	}
	return b
}

// ----- Put / Get tests -----

func TestBlockStore_PutAndGet(t *testing.T) {
	store := NewStore()
	block := makeBlock(0xAB)

	if err := store.Put("block1", block); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	got, ok := store.Get("block1")
	if !ok {
		t.Fatal("expected block to exist")
	}
	if got != block {
		t.Error("Get returned wrong block data after Put")
	}
}

func TestBlockStore_GetNonExistent(t *testing.T) {
	store := NewStore()
	_, ok := store.Get("nonexistent")
	if ok {
		t.Error("Get of nonexistent block should return false")
	}
}

func TestBlockStore_OverwriteBlock(t *testing.T) {
	store := NewStore()
	block1 := makeBlock(0x01)
	block2 := makeBlock(0x02)

	if err := store.Put("block1", block1); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if err := store.Put("block1", block2); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	got, ok := store.Get("block1")
	if !ok {
		t.Fatal("expected block to exist")
	}
	if got != block2 {
		t.Error("Second Put should overwrite first")
	}
}

// ----- Delete tests -----

func TestBlockStore_Delete(t *testing.T) {
	store := NewStore()
	block := makeBlock(0xFF)
	if err := store.Put("block1", block); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	store.Delete("block1")

	_, ok := store.Get("block1")
	if ok {
		t.Error("Block should not exist after delete")
	}
}

func TestBlockStore_DeleteNonExistent(t *testing.T) {
	store := NewStore()
	store.Delete("doesnotexist")
}

func TestBlockStore_DeleteThenPut(t *testing.T) {
	store := NewStore()
	block := makeBlock(0xCC)
	if err := store.Put("id1", block); err != nil {
		t.Fatalf("first Put failed: %v", err)
	}
	store.Delete("id1")
	if err := store.Put("id1", block); err != nil {
		t.Fatalf("second Put failed: %v", err)
	}

	got, ok := store.Get("id1")
	if !ok {
		t.Fatal("expected block to exist after re-put")
	}
	if got != block {
		t.Error("Should be able to re-put after delete")
	}
}

func TestBlockStore_ConcurrentPut(t *testing.T) {
	store := NewStore()
	block := makeBlock(0x01)

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(n int) {
			store.Put("block", block) //nolint:errcheck
			done <- true
		}(i)
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestBlockStore_ConcurrentGetAndPut(t *testing.T) {
	store := NewStore()
	block := makeBlock(0x05)
	if err := store.Put("block1", block); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	done := make(chan bool, 20)
	for i := 0; i < 10; i++ {
		go func() {
			store.Get("block1") //nolint:errcheck
			done <- true
		}()
	}
	for i := 0; i < 10; i++ {
		go func() {
			store.Put("block1", block) //nolint:errcheck
			done <- true
		}()
	}
	for i := 0; i < 20; i++ {
		<-done
	}
}

func TestBlockStore_MultipleBlocks(t *testing.T) {
	store := NewStore()
	blocks := map[string][config.BlockSize]byte{
		"a": makeBlock(0x01),
		"b": makeBlock(0x02),
		"c": makeBlock(0x03),
	}

	for id, block := range blocks {
		if err := store.Put(id, block); err != nil {
			t.Fatalf("Put %s failed: %v", id, err)
		}
	}
	for id, expected := range blocks {
		got, _ := store.Get(id)
		if got != expected {
			t.Errorf("block %s has wrong data", id)
		}
	}
}
