package chain

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func h(b byte) common.Hash { return common.BytesToHash([]byte{b}) }

func TestTracker_AddAndHash(t *testing.T) {
	tr := NewTracker(8)
	tr.Add(10, h(1))
	tr.Add(11, h(2))

	if got, ok := tr.Hash(11); !ok || got != h(2) {
		t.Fatalf("Hash(11) = %v, %v", got, ok)
	}
	if latest, ok := tr.Latest(); !ok || latest != 11 {
		t.Fatalf("Latest = %d, %v", latest, ok)
	}
}

func TestTracker_RevertFrom(t *testing.T) {
	tr := NewTracker(8)
	for i := uint64(10); i <= 15; i++ {
		tr.Add(i, h(byte(i)))
	}
	tr.RevertFrom(13)

	if _, ok := tr.Hash(13); ok {
		t.Error("block 13 should be reverted")
	}
	if _, ok := tr.Hash(14); ok {
		t.Error("block 14 should be reverted")
	}
	if latest, _ := tr.Latest(); latest != 12 {
		t.Errorf("latest after revert = %d, want 12", latest)
	}
	if _, ok := tr.Hash(12); !ok {
		t.Error("block 12 should remain")
	}
}

func TestTracker_PrunesBeyondDepth(t *testing.T) {
	tr := NewTracker(3)
	for i := uint64(1); i <= 10; i++ {
		tr.Add(i, h(byte(i)))
	}
	// 仅保留最近 3 个(8,9,10)。
	if _, ok := tr.Hash(7); ok {
		t.Error("block 7 should be pruned")
	}
	for i := uint64(8); i <= 10; i++ {
		if _, ok := tr.Hash(i); !ok {
			t.Errorf("block %d should be kept", i)
		}
	}
}
