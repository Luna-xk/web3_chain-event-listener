// Command listener 是链上事件监听器的入口。
//
// 用法:
//
//	listener -config config.yaml
//
// 支持两种模式(见 config.yaml 的 chain.mode):
//   - mock:本地模拟出块与事件(含一次链重组),无需任何外部依赖即可演示。
//   - rpc :连接真实链节点(go-ethereum ethclient)。
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/Luna-xk/chain-event-listener/internal/chain"
	"github.com/Luna-xk/chain-event-listener/internal/config"
	"github.com/Luna-xk/chain-event-listener/internal/listener"
	"github.com/Luna-xk/chain-event-listener/internal/storage"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Error("load config", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	client, err := buildClient(ctx, cfg)
	if err != nil {
		log.Error("build chain client", "err", err)
		os.Exit(1)
	}
	defer client.Close()

	store := storage.NewMemory()
	l := listener.New(cfg, client, store, log)

	go reportStats(ctx, store, log)

	if err := l.Run(ctx); err != nil {
		log.Error("listener exited with error", "err", err)
		os.Exit(1)
	}

	s := store.Stats(context.Background())
	log.Info("final stats", "total_events", s.TotalEvents, "last_block", s.LastProcessedBlock)
}

func buildClient(ctx context.Context, cfg *config.Config) (chain.Client, error) {
	switch cfg.Chain.Mode {
	case config.ModeRPC:
		return chain.NewRPCClient(ctx, cfg.Chain.RPCURL)
	default:
		contracts := make([]common.Address, 0, len(cfg.Contracts))
		for _, c := range cfg.Contracts {
			contracts = append(contracts, common.HexToAddress(c))
		}
		genesis := cfg.Listener.StartBlock
		if genesis == 0 {
			genesis = 1_000_000 // 模拟链的起始高度
		}
		return chain.NewMockClient(genesis, contracts), nil
	}
}

func reportStats(ctx context.Context, store storage.Store, log *slog.Logger) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s := store.Stats(ctx)
			log.Info("stats", "total_events", s.TotalEvents, "last_block", s.LastProcessedBlock)
		}
	}
}
