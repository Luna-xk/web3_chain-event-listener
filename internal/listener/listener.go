// Package listener 实现核心监听循环:拉取区块、解析事件、检测并恢复链重组。
package listener

import (
	"context"
	"log/slog"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"

	"github.com/Luna-xk/chain-event-listener/internal/chain"
	"github.com/Luna-xk/chain-event-listener/internal/config"
	"github.com/Luna-xk/chain-event-listener/internal/model"
	"github.com/Luna-xk/chain-event-listener/internal/storage"
)

// Listener 把链客户端、存储与重组跟踪器组合为完整的索引服务。
type Listener struct {
	cfg       config.ListenerConfig
	chainName string
	client    chain.Client
	store     storage.Store
	tracker   *chain.Tracker
	contracts []common.Address
	log       *slog.Logger

	next uint64 // 下一个待处理区块高度
}

// New 构造一个 Listener。
func New(cfg *config.Config, client chain.Client, store storage.Store, log *slog.Logger) *Listener {
	contracts := make([]common.Address, 0, len(cfg.Contracts))
	for _, c := range cfg.Contracts {
		contracts = append(contracts, common.HexToAddress(c))
	}
	return &Listener{
		cfg:       cfg.Listener,
		chainName: cfg.Chain.Name,
		client:    client,
		store:     store,
		tracker:   chain.NewTracker(cfg.Listener.ReorgDepth),
		contracts: contracts,
		log:       log,
	}
}

// Run 启动监听循环,直到 ctx 被取消。
func (l *Listener) Run(ctx context.Context) error {
	if err := l.resume(ctx); err != nil {
		return err
	}
	l.log.Info("listener started",
		"chain", l.chainName, "from_block", l.next,
		"confirmations", l.cfg.Confirmations, "contracts", len(l.contracts))

	ticker := time.NewTicker(l.cfg.PollInterval)
	defer ticker.Stop()
	for {
		if err := l.tick(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			l.log.Error("tick failed", "err", err)
		}
		select {
		case <-ctx.Done():
			l.log.Info("listener stopped")
			return nil
		case <-ticker.C:
		}
	}
}

// resume 决定起始区块:优先续接存储中的进度,否则按配置或最新高度回看。
func (l *Listener) resume(ctx context.Context) error {
	if last, ok, err := l.store.LastProcessedBlock(ctx); err != nil {
		return err
	} else if ok {
		l.next = last + 1
		return nil
	}
	if l.cfg.StartBlock > 0 {
		l.next = l.cfg.StartBlock
		return nil
	}
	head, err := l.client.BlockNumber(ctx)
	if err != nil {
		return err
	}
	if head > l.cfg.LookbackBlocks {
		l.next = head - l.cfg.LookbackBlocks
	}
	return nil
}

// tick 执行一轮:重组检测 -> 处理新区块。
func (l *Listener) tick(ctx context.Context) error {
	head, err := l.client.BlockNumber(ctx)
	if err != nil {
		return err
	}
	if err := l.detectReorg(ctx); err != nil {
		return err
	}

	// 仅索引达到确认深度的"安全"区块,降低浅层重组造成的脏数据。
	if head < l.cfg.Confirmations {
		return nil
	}
	safe := head - l.cfg.Confirmations
	if l.next > safe {
		return nil
	}

	end := safe
	if max := l.next + l.cfg.BatchSize - 1; end > max {
		end = max
	}
	for n := l.next; n <= end; n++ {
		if err := l.processBlock(ctx, n); err != nil {
			return err
		}
		l.next = n + 1
	}
	return nil
}

// detectReorg 从跟踪器记录的最高块向下回溯,比对链上哈希,
// 找到第一个仍然一致的"公共祖先";其上的区块即为被重组掉的分叉,需回滚。
func (l *Listener) detectReorg(ctx context.Context) error {
	latest, ok := l.tracker.Latest()
	if !ok {
		return nil
	}
	oldest := l.tracker.Oldest()
	ancestor := latest
	mismatch := false
	for n := latest; ; n-- {
		stored, has := l.tracker.Hash(n)
		if !has {
			break
		}
		h, err := l.client.HeaderByNumber(ctx, n)
		if err != nil {
			return err
		}
		if h.Hash() == stored {
			ancestor = n
			break
		}
		mismatch = true
		if n == oldest || n == 0 {
			// 重组深度超过跟踪窗口:尽力从窗口底部回滚。
			ancestor = oldest - 1
			break
		}
	}
	if !mismatch {
		return nil
	}

	revertFrom := ancestor + 1
	reverted, err := l.store.RevertFrom(ctx, revertFrom)
	if err != nil {
		return err
	}
	l.tracker.RevertFrom(revertFrom)
	l.next = revertFrom
	l.log.Warn("reorg detected, rolled back",
		"common_ancestor", ancestor, "revert_from", revertFrom,
		"reverted_events", reverted, "resume_from", l.next)
	return nil
}

// processBlock 拉取并解析单个区块的事件,落库并登记哈希。
func (l *Listener) processBlock(ctx context.Context, number uint64) error {
	header, err := l.client.HeaderByNumber(ctx, number)
	if err != nil {
		return err
	}
	hash := header.Hash()

	// 链式校验:确保本块父哈希衔接上一个已处理块(纵深防御)。
	if number > 0 {
		if prev, ok := l.tracker.Hash(number - 1); ok && header.ParentHash != prev {
			l.log.Warn("parent hash mismatch, will resolve on next reorg check",
				"block", number, "parent", header.ParentHash.Hex(), "tracked", prev.Hex())
		}
	}

	logs, err := l.client.FilterLogs(ctx, ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(number),
		ToBlock:   new(big.Int).SetUint64(number),
		Addresses: l.contracts,
	})
	if err != nil {
		return err
	}

	events := make([]model.Event, 0, len(logs))
	for _, lg := range logs {
		if ev, ok := parseLog(l.chainName, lg); ok {
			events = append(events, ev)
		}
	}
	if err := l.store.SaveEvents(ctx, number, hash.Hex(), events); err != nil {
		return err
	}
	l.tracker.Add(number, hash)

	if len(events) > 0 {
		l.log.Info("block processed",
			"block", number, "hash", short(hash.Hex()), "events", len(events))
	}
	return nil
}

func short(h string) string {
	if len(h) <= 12 {
		return h
	}
	return h[:8] + ".." + h[len(h)-4:]
}
