// Package storage 定义事件持久化抽象,方便在内存 / MySQL / 其它后端之间切换。
package storage

import (
	"context"

	"github.com/Luna-xk/chain-event-listener/internal/model"
)

// Store 是事件存储接口。生产环境可换成 MySQL/Postgres 实现,
// 只要保证 SaveEvents 的幂等性与 RevertFrom 的原子性即可。
type Store interface {
	// LastProcessedBlock 返回已成功处理的最高区块;无记录时返回 (0,false)。
	LastProcessedBlock(ctx context.Context) (uint64, bool, error)

	// SaveEvents 落库某个区块的全部事件,并推进 last processed 指针。
	// 必须按 (tx_hash, log_index) 幂等,避免重放导致重复入库。
	SaveEvents(ctx context.Context, blockNumber uint64, blockHash string, events []model.Event) error

	// RevertFrom 删除区块号 >= from 的全部事件(链重组回滚)。
	RevertFrom(ctx context.Context, from uint64) (reverted int, err error)

	// Events 返回当前已落库的全部事件(仅用于 demo 展示)。
	Events(ctx context.Context) ([]model.Event, error)

	// Stats 返回汇总指标。
	Stats(ctx context.Context) Stats
}

type Stats struct {
	TotalEvents        int
	LastProcessedBlock uint64
}
