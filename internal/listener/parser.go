package listener

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/lunaxk/chain-event-listener/internal/model"
)

var transferTopic = crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))

// parseLog 将一条原始日志解析为领域事件。
//
// ERC20 与 ERC721 的 Transfer 共用同一个 topic0,通过 indexed 参数个数区分:
//   - 3 个 topic(签名 + from + to),value 在 data 中  -> ERC20
//   - 4 个 topic(签名 + from + to + tokenId),data 为空 -> ERC721
func parseLog(chain string, lg types.Log) (model.Event, bool) {
	if len(lg.Topics) < 3 || lg.Topics[0] != transferTopic {
		return model.Event{}, false
	}
	ev := model.Event{
		Chain:       chain,
		Contract:    lg.Address.Hex(),
		BlockNumber: lg.BlockNumber,
		BlockHash:   lg.BlockHash.Hex(),
		TxHash:      lg.TxHash.Hex(),
		LogIndex:    lg.Index,
		From:        common.BytesToAddress(lg.Topics[1].Bytes()).Hex(),
		To:          common.BytesToAddress(lg.Topics[2].Bytes()).Hex(),
		Timestamp:   time.Now().UTC(),
	}
	switch len(lg.Topics) {
	case 3: // ERC20
		ev.Type = model.EventERC20Transfer
		ev.Value = new(big.Int).SetBytes(lg.Data).String()
	case 4: // ERC721
		ev.Type = model.EventERC721Transfer
		ev.Value = lg.Topics[3].Big().String() // tokenId
	default:
		return model.Event{}, false
	}
	return ev, true
}
