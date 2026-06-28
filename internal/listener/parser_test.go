package listener

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/Luna-xk/chain-event-listener/internal/model"
)

func addrTopic(a string) common.Hash {
	return common.BytesToHash(common.HexToAddress(a).Bytes())
}

func TestParseLog_ERC20(t *testing.T) {
	lg := types.Log{
		Address:     common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48"),
		BlockNumber: 100,
		TxHash:      common.HexToHash("0xabc"),
		Index:       2,
		Topics: []common.Hash{
			transferTopic,
			addrTopic("0x1111111111111111111111111111111111111111"),
			addrTopic("0x2222222222222222222222222222222222222222"),
		},
		Data: common.BigToHash(big.NewInt(123456)).Bytes(),
	}
	ev, ok := parseLog("ethereum", lg)
	if !ok {
		t.Fatal("expected ERC20 transfer to parse")
	}
	if ev.Type != model.EventERC20Transfer {
		t.Errorf("type = %s, want %s", ev.Type, model.EventERC20Transfer)
	}
	if ev.Value != "123456" {
		t.Errorf("value = %s, want 123456", ev.Value)
	}
	if ev.From != common.HexToAddress("0x1111111111111111111111111111111111111111").Hex() {
		t.Errorf("from mismatch: %s", ev.From)
	}
}

func TestParseLog_ERC721(t *testing.T) {
	lg := types.Log{
		Address: common.HexToAddress("0xBC4CA0EdA7647A8aB7C2061c2E118A18a936f13D"),
		Topics: []common.Hash{
			transferTopic,
			addrTopic("0x1111111111111111111111111111111111111111"),
			addrTopic("0x2222222222222222222222222222222222222222"),
			common.BigToHash(big.NewInt(8888)), // tokenId
		},
	}
	ev, ok := parseLog("ethereum", lg)
	if !ok {
		t.Fatal("expected ERC721 transfer to parse")
	}
	if ev.Type != model.EventERC721Transfer {
		t.Errorf("type = %s, want %s", ev.Type, model.EventERC721Transfer)
	}
	if ev.Value != "8888" {
		t.Errorf("tokenId = %s, want 8888", ev.Value)
	}
}

func TestParseLog_IgnoresNonTransfer(t *testing.T) {
	lg := types.Log{Topics: []common.Hash{common.HexToHash("0xdeadbeef")}}
	if _, ok := parseLog("ethereum", lg); ok {
		t.Fatal("non-transfer log should be ignored")
	}
}
