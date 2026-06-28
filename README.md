# Chain Event Listener · 链上事件监听器

> 基于 **Go + go-ethereum** 的多链事件索引服务:实时监听 ERC20 / ERC721 `Transfer` 事件,**处理链重组(reorg)**、保证数据一致性与幂等入库。

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)](https://go.dev)
[![go-ethereum](https://img.shields.io/badge/go--ethereum-1.14-3C3C3D?logo=ethereum)](https://geth.ethereum.org)

这是一个生产级链上索引器的**精简实现**,聚焦后端工程师在 Web3 场景最常被考察的能力:从链节点稳定拉取数据、解析合约事件、应对链重组带来的数据回滚、并以幂等方式落库。

---

## ✨ 特性

- **事件监听**:基于区块轮询(兼容 HTTP RPC,无需 WebSocket),按合约地址 + topic 过滤 `Transfer` 日志。
- **ERC20 / ERC721 自动识别**:二者共用 `Transfer` topic,通过 indexed 参数个数区分(3 个 topic → ERC20,4 个 → ERC721)。
- **链重组处理(核心)**:维护最近 N 个区块哈希,每轮与链上比对,自动定位公共祖先、回滚失效区块、从分叉点重放。
- **确认数机制**:可配置确认深度,降低浅层重组带来的脏数据。
- **幂等入库**:以 `(txHash, logIndex)` 为业务主键去重,重放不会产生重复记录。
- **断点续传**:重启后从存储中已处理的最高区块继续。
- **可插拔架构**:`chain.Client` 与 `storage.Store` 均为接口,真实 RPC / mock、内存 / MySQL 可自由替换。
- **开箱即用的离线 Demo**:内置 `mock` 模式模拟出块并**主动制造一次链重组**,断网即可观察完整恢复流程。

---

## 🏗️ 架构

```
                 ┌──────────────────────────────────────────────┐
                 │                  Listener                     │
                 │                                                │
  chain.Client   │   ┌──────────┐   ┌───────────┐   ┌─────────┐  │   storage.Store
 ┌────────────┐  │   │  poll    │→  │  reorg     │→  │ parse & │  │  ┌────────────┐
 │ RPCClient  │──┼──▶│  blocks  │   │  detect    │   │  save   │──┼─▶│  Memory    │
 │ (geth)     │  │   └──────────┘   └─────┬─────┘   └─────────┘  │  │  (or MySQL)│
 ├────────────┤  │                        │ rollback             │  └────────────┘
 │ MockClient │  │                  ┌─────▼─────┐                 │
 │ (offline)  │  │                  │  Tracker  │ (recent hashes) │
 └────────────┘  │                  └───────────┘                 │
                 └──────────────────────────────────────────────┘
```

目录结构:

```
cmd/listener/        程序入口、配置装配、优雅退出
internal/
  config/            YAML 配置加载与默认值
  model/             领域模型(Event)
  storage/           存储接口 + 内存实现(幂等去重 / 回滚)
  chain/             链客户端接口、go-ethereum 实现、mock 实现、reorg Tracker
  listener/          核心监听循环 + 日志解析(ERC20/721)
```

---

## 🚀 快速开始

### 1. 离线 Demo(推荐,无需任何外部依赖)

```bash
make demo
```

或手动:

```bash
cp config.example.yaml config.yaml   # 默认 mode: mock
go run ./cmd/listener -config config.yaml
```

运行约 7 秒后会看到 **`reorg detected, rolled back`** —— 模拟链重组被自动检测并回滚,随后相同高度的区块以**新哈希重新落库**:

```
level=INFO msg="listener started" chain=ethereum from_block=1000010 confirmations=2 contracts=2
level=INFO msg="block processed" block=1000022 hash=0xeb5df8..1844 events=1
level=INFO msg="block processed" block=1000023 hash=0x931c03..b732 events=2
level=INFO msg="block processed" block=1000024 hash=0xc97147..11a1 events=2
...
level=WARN msg="reorg detected, rolled back" common_ancestor=1000022 revert_from=1000023 reverted_events=6 resume_from=1000023
level=INFO msg="block processed" block=1000023 hash=0x7acfa7..df9c events=1   # 同高度,新哈希
level=INFO msg="block processed" block=1000024 hash=0xbf20af..a220 events=1
level=INFO msg=stats total_events=20 last_block=1000028
```

> 演示中重组深度(8)被特意设得比 `confirmations`(2)更深,以便覆盖到"已索引"的区块、直观展示回滚;真实链重组通常仅 1~2 块,`confirmations` 即可拦截。

`Ctrl+C` 退出,会打印最终统计。

### 2. 连接真实链(RPC 模式)

编辑 `config.yaml`:

```yaml
chain:
  mode: "rpc"
  rpc_url: "https://eth.llamarpc.com"   # 或你自己的节点 / Alchemy / Infura
```

再次运行即可监听主网真实的 USDC / BAYC `Transfer` 事件。

---

## ⚙️ 配置说明

| 字段 | 说明 |
|---|---|
| `chain.mode` | `mock`(离线模拟)或 `rpc`(真实节点) |
| `chain.rpc_url` | RPC 端点,`rpc` 模式必填 |
| `listener.confirmations` | 确认数,事件达到该深度后视为最终确认 |
| `listener.poll_interval` | 区块轮询间隔 |
| `listener.reorg_depth` | 链重组检测的最大回溯深度 |
| `listener.batch_size` | 单轮最多处理的区块数,控制追块速度 |
| `contracts` | 监听的合约地址列表,留空表示监听全网 |

---

## 🔁 链重组处理逻辑

1. 每处理一个区块,将其 `header.Hash()` 记入 `Tracker`(只保留最近 `reorg_depth` 个)。
2. 每轮拉取前,从记录的最高块向下回溯,用**链上当前哈希**比对本地记录,找到第一个仍一致的高度 —— 即**公共祖先**。
3. 若发现不一致(说明发生重组),删除存储中区块号 ≥ 祖先+1 的全部事件,跟踪器同步回退。
4. 监听指针重置到分叉点,从公共祖先之后**重新拉取并落库**,保证最终一致。

> 入库以 `(txHash, logIndex)` 幂等去重,因此即便重放同一区块也不会产生重复数据。

---

## 🧪 测试

```bash
make test
```

覆盖了事件解析(ERC20 vs ERC721 区分)与重组跟踪器(回滚 / 裁剪)的核心逻辑。

---

## 🛣️ 后续可扩展方向

- 接入 MySQL / Postgres 持久化(实现 `storage.Store` 接口即可)。
- 多链并行监听(每条链一个 `Listener` 实例 + 统一调度)。
- 暴露 Prometheus 指标与 HTTP 查询接口。
- 用 WebSocket 订阅替代轮询以降低延迟。

---

## License

MIT
