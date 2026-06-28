package chain

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// RPCClient 是基于 go-ethereum ethclient 的真实链客户端。
type RPCClient struct {
	ec *ethclient.Client
}

// NewRPCClient 通过 HTTP/WS RPC URL 建立连接。
func NewRPCClient(ctx context.Context, rpcURL string) (*RPCClient, error) {
	ec, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		return nil, fmt.Errorf("dial rpc %q: %w", rpcURL, err)
	}
	return &RPCClient{ec: ec}, nil
}

func (c *RPCClient) BlockNumber(ctx context.Context) (uint64, error) {
	return c.ec.BlockNumber(ctx)
}

func (c *RPCClient) HeaderByNumber(ctx context.Context, number uint64) (*types.Header, error) {
	return c.ec.HeaderByNumber(ctx, new(big.Int).SetUint64(number))
}

func (c *RPCClient) FilterLogs(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
	return c.ec.FilterLogs(ctx, q)
}

func (c *RPCClient) Close() { c.ec.Close() }
