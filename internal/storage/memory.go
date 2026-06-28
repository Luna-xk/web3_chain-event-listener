package storage

import (
	"context"
	"sort"
	"sync"

	"github.com/lunaxk/chain-event-listener/internal/model"
)

// Memory 是线程安全的内存实现,开箱即用、无需外部依赖,适合 demo 与单测。
type Memory struct {
	mu       sync.RWMutex
	events   map[string]model.Event // uniqueKey -> event,天然幂等去重
	lastBlk  uint64
	hasBlock bool
}

// NewMemory 构造一个空的内存存储。
func NewMemory() *Memory {
	return &Memory{events: make(map[string]model.Event)}
}

func (m *Memory) LastProcessedBlock(_ context.Context) (uint64, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastBlk, m.hasBlock, nil
}

func (m *Memory) SaveEvents(_ context.Context, blockNumber uint64, _ string, events []model.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range events {
		m.events[e.UniqueKey()] = e // 幂等:相同日志覆盖写,不会重复
	}
	if !m.hasBlock || blockNumber > m.lastBlk {
		m.lastBlk = blockNumber
		m.hasBlock = true
	}
	return nil
}

func (m *Memory) RevertFrom(_ context.Context, from uint64) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	reverted := 0
	for k, e := range m.events {
		if e.BlockNumber >= from {
			delete(m.events, k)
			reverted++
		}
	}
	if from > 0 {
		m.lastBlk = from - 1
	} else {
		m.lastBlk, m.hasBlock = 0, false
	}
	return reverted, nil
}

func (m *Memory) Events(_ context.Context) ([]model.Event, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]model.Event, 0, len(m.events))
	for _, e := range m.events {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].BlockNumber != out[j].BlockNumber {
			return out[i].BlockNumber < out[j].BlockNumber
		}
		return out[i].LogIndex < out[j].LogIndex
	})
	return out, nil
}

func (m *Memory) Stats(_ context.Context) Stats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return Stats{TotalEvents: len(m.events), LastProcessedBlock: m.lastBlk}
}
