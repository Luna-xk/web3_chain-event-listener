// Package model defines the core domain entities persisted by the listener.
package model

import "time"

// EventType 标识被解析出的链上事件类型。
type EventType string

const (
	EventERC20Transfer  EventType = "ERC20_Transfer"
	EventERC721Transfer EventType = "ERC721_Transfer"
)

// Event 是一条经过解析、可落库的链上事件记录。
//
// 设计要点:
//   - 同时保存 BlockNumber 与 BlockHash,链重组时可按区块精确回滚。
//   - (TxHash, LogIndex) 唯一确定一条日志,作为幂等去重的业务主键。
type Event struct {
	Type        EventType `json:"type"`
	Chain       string    `json:"chain"`
	Contract    string    `json:"contract"`
	BlockNumber uint64    `json:"block_number"`
	BlockHash   string    `json:"block_hash"`
	TxHash      string    `json:"tx_hash"`
	LogIndex    uint      `json:"log_index"`
	From        string    `json:"from"`
	To          string    `json:"to"`
	// Value 对 ERC20 为转账金额(wei),对 ERC721 为 tokenId。
	Value     string    `json:"value"`
	Timestamp time.Time `json:"timestamp"`
}

// UniqueKey 返回事件的幂等去重键。
func (e Event) UniqueKey() string {
	return e.TxHash + ":" + itoa(uint64(e.LogIndex))
}

func itoa(n uint64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
