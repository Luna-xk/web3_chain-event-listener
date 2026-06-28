package chain

import (
	"context"
	"math/big"
	"math/rand"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// transferTopic 是 ERC20/ERC721 共用的 Transfer 事件签名 topic。
var transferTopic = crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))

type mockBlock struct {
	header *types.Header
	hash   common.Hash // 缓存 header.Hash(),作为该区块的 canonical 哈希
	logs   []types.Log
}

// MockClient 在本地模拟一条链:按墙钟时间出块、生成随机 Transfer 事件,
// 并在到达指定高度时制造一次链重组,用于离线演示监听与重组恢复全流程。
//
// 关键点:区块哈希一律使用 go-ethereum 的 header.Hash() 计算,且
// ParentHash 正确指向父块哈希,因此监听端的 parentHash 链式校验与
// 重组检测逻辑与真实链表现一致。
type MockClient struct {
	mu        sync.Mutex
	contracts []common.Address
	start     time.Time // 创世区块对应的逻辑时间(置于过去,用于预填历史)
	created   time.Time // 实际构造时间,用于触发重组计时
	genesis   uint64
	blockTime time.Duration
	blocks    map[uint64]*mockBlock
	head      uint64
	rnd       *rand.Rand

	reorgAfter time.Duration // 启动多久后触发一次重组
	reorgDepth uint64        // 重组影响的区块数
	reorgDone  bool
}

const mockPreSeed = 20 // 构造时预填的历史区块数,保证起始回看有数据

// NewMockClient 创建模拟客户端,从 genesis 高度开始出块,并预填一段历史。
func NewMockClient(genesis uint64, contracts []common.Address) *MockClient {
	now := time.Now()
	blockTime := 800 * time.Millisecond
	m := &MockClient{
		contracts:  contracts,
		start:      now.Add(-time.Duration(mockPreSeed) * blockTime),
		created:    now,
		genesis:    genesis,
		blockTime:  blockTime,
		blocks:     make(map[uint64]*mockBlock),
		head:       genesis,
		rnd:        rand.New(rand.NewSource(now.UnixNano())),
		reorgAfter: 7 * time.Second,
		// 真实链重组通常仅 1~2 块;这里特意用较深的重组,确保覆盖到监听端
		// "已索引"的区块,从而能直观演示数据回滚与重放。
		reorgDepth: 8,
	}
	m.blocks[genesis] = m.buildBlock(genesis, common.Hash{})
	m.advance() // 预填至 genesis+mockPreSeed
	return m
}

func (m *MockClient) randBytes(n int) []byte {
	b := make([]byte, n)
	m.rnd.Read(b)
	return b
}

// buildBlock 构造一个区块:Extra 放随机 nonce 保证哈希唯一,ParentHash 指向父哈希。
func (m *MockClient) buildBlock(number uint64, parent common.Hash) *mockBlock {
	h := &types.Header{
		Number:     new(big.Int).SetUint64(number),
		ParentHash: parent,
		Time:       uint64(m.start.Add(time.Duration(number-m.genesis) * m.blockTime).Unix()),
		Extra:      m.randBytes(16),
	}
	hash := h.Hash()
	return &mockBlock{header: h, hash: hash, logs: m.genLogs(number, hash)}
}

// advance 根据已流逝时间补齐区块,并在条件满足时执行一次重组。
func (m *MockClient) advance() {
	target := m.genesis + uint64(time.Since(m.start)/m.blockTime)
	for n := m.head + 1; n <= target; n++ {
		m.blocks[n] = m.buildBlock(n, m.blocks[n-1].hash)
		m.head = n
	}
	if !m.reorgDone && time.Since(m.created) >= m.reorgAfter {
		m.applyReorg()
	}
}

// applyReorg 重写最近 reorgDepth 个区块,模拟主链分叉切换为更长的新链。
func (m *MockClient) applyReorg() {
	fork := m.head - m.reorgDepth
	for n := fork + 1; n <= m.head; n++ {
		m.blocks[n] = m.buildBlock(n, m.blocks[n-1].hash)
	}
	m.reorgDone = true
}

func (m *MockClient) genLogs(block uint64, blockHash common.Hash) []types.Log {
	if len(m.contracts) == 0 {
		return nil
	}
	n := m.rnd.Intn(3) // 每个区块 0-2 条事件
	logs := make([]types.Log, 0, n)
	for i := 0; i < n; i++ {
		contract := m.contracts[m.rnd.Intn(len(m.contracts))]
		fromAddr := common.BytesToAddress(m.randBytes(20))
		toAddr := common.BytesToAddress(m.randBytes(20))
		isNFT := m.rnd.Intn(2) == 0
		lg := types.Log{
			Address:     contract,
			BlockNumber: block,
			BlockHash:   blockHash,
			TxHash:      common.BytesToHash(m.randBytes(32)),
			Index:       uint(i),
			Topics: []common.Hash{
				transferTopic,
				common.BytesToHash(fromAddr.Bytes()),
				common.BytesToHash(toAddr.Bytes()),
			},
		}
		val := new(big.Int).SetUint64(uint64(m.rnd.Intn(1_000_000) + 1))
		if isNFT {
			// ERC721: tokenId 作为第四个 indexed topic,data 为空。
			lg.Topics = append(lg.Topics, common.BigToHash(val))
		} else {
			// ERC20: value 放在 data 中。
			lg.Data = common.BigToHash(val).Bytes()
		}
		logs = append(logs, lg)
	}
	return logs
}

func (m *MockClient) BlockNumber(_ context.Context) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.advance()
	return m.head, nil
}

func (m *MockClient) HeaderByNumber(_ context.Context, number uint64) (*types.Header, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.advance()
	b, ok := m.blocks[number]
	if !ok {
		return nil, ethereum.NotFound
	}
	cp := *b.header
	return &cp, nil
}

func (m *MockClient) FilterLogs(_ context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.advance()
	from, to := q.FromBlock.Uint64(), q.ToBlock.Uint64()
	want := make(map[common.Address]bool, len(q.Addresses))
	for _, a := range q.Addresses {
		want[a] = true
	}
	var out []types.Log
	for n := from; n <= to; n++ {
		b, ok := m.blocks[n]
		if !ok {
			continue
		}
		for _, lg := range b.logs {
			if len(want) == 0 || want[lg.Address] {
				out = append(out, lg)
			}
		}
	}
	return out, nil
}

func (m *MockClient) Close() {}
