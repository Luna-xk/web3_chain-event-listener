// Package chain 封装与链交互的能力,并提供链重组检测。
package chain

import (
	"context"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
)

// Client 抽象了 listener 需要的最小链上读能力。
// 真实实现基于 go-ethereum 的 ethclient;mock 实现用于离线 demo 与测试。
// 通过接口隔离,核心监听逻辑无需关心数据来源。
type Client interface {
	// BlockNumber 返回当前最新区块高度。
	BlockNumber(ctx context.Context) (uint64, error)
	// HeaderByNumber 返回指定高度的区块头(含 Hash / ParentHash)。
	HeaderByNumber(ctx context.Context, number uint64) (*types.Header, error)
	// FilterLogs 按过滤条件拉取日志。
	FilterLogs(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error)
	// Close 释放底层连接。
	Close()
}
