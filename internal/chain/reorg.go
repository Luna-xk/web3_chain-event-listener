package chain

import (
	"github.com/ethereum/go-ethereum/common"
)

// Tracker 维护最近若干区块的 (高度 -> 哈希) 映射,用于链重组检测。
//
// 思路:监听端每处理一个区块就把它的哈希记下来;下一轮拉取前,
// 用链上最新哈希与本地记录比对,即可发现重组并定位"公共祖先"。
type Tracker struct {
	depth  int
	blocks map[uint64]common.Hash
	latest uint64
	hasAny bool
}

// NewTracker 创建一个最多保留 depth 个区块哈希的跟踪器。
func NewTracker(depth int) *Tracker {
	if depth < 1 {
		depth = 1
	}
	return &Tracker{depth: depth, blocks: make(map[uint64]common.Hash)}
}

// Add 记录某高度的区块哈希,并按深度裁剪过旧的记录。
func (t *Tracker) Add(number uint64, hash common.Hash) {
	t.blocks[number] = hash
	if !t.hasAny || number > t.latest {
		t.latest = number
		t.hasAny = true
	}
	t.prune()
}

// Hash 返回某高度记录的哈希。
func (t *Tracker) Hash(number uint64) (common.Hash, bool) {
	h, ok := t.blocks[number]
	return h, ok
}

// Latest 返回当前记录的最高高度。
func (t *Tracker) Latest() (uint64, bool) { return t.latest, t.hasAny }

// Oldest 返回当前记录的最低高度(用于界定重组检测的回溯下界)。
func (t *Tracker) Oldest() uint64 {
	if !t.hasAny {
		return 0
	}
	if t.latest < uint64(t.depth) {
		return 0
	}
	return t.latest - uint64(t.depth) + 1
}

// RevertFrom 删除高度 >= from 的记录,并将 latest 回退到 from-1。
func (t *Tracker) RevertFrom(from uint64) {
	for n := range t.blocks {
		if n >= from {
			delete(t.blocks, n)
		}
	}
	if from == 0 {
		t.hasAny = false
		t.latest = 0
		return
	}
	t.latest = from - 1
	t.hasAny = len(t.blocks) > 0
}

func (t *Tracker) prune() {
	if uint64(len(t.blocks)) <= uint64(t.depth) {
		return
	}
	min := t.Oldest()
	for n := range t.blocks {
		if n < min {
			delete(t.blocks, n)
		}
	}
}
