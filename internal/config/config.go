// Package config 负责加载与校验运行配置。
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Mode 决定数据来源:真实 RPC 或本地模拟。
type Mode string

const (
	ModeRPC  Mode = "rpc"  // 连接真实链节点
	ModeMock Mode = "mock" // 本地模拟出块与事件,用于离线 demo
)

type Config struct {
	Chain     ChainConfig    `yaml:"chain"`
	Listener  ListenerConfig `yaml:"listener"`
	Contracts []string       `yaml:"contracts"`
}

type ChainConfig struct {
	Name   string `yaml:"name"`
	RPCURL string `yaml:"rpc_url"`
	Mode   Mode   `yaml:"mode"`
}

type ListenerConfig struct {
	// StartBlock 为 0 时表示从 (latest - LookbackBlocks) 开始。
	StartBlock     uint64        `yaml:"start_block"`
	LookbackBlocks uint64        `yaml:"lookback_blocks"`
	Confirmations  uint64        `yaml:"confirmations"`
	PollInterval   time.Duration `yaml:"poll_interval"`
	ReorgDepth     int           `yaml:"reorg_depth"`
	BatchSize      uint64        `yaml:"batch_size"`
}

// Load 从 YAML 文件读取配置并补齐默认值。
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Chain.Mode == "" {
		c.Chain.Mode = ModeMock
	}
	if c.Chain.Name == "" {
		c.Chain.Name = "ethereum"
	}
	if c.Listener.Confirmations == 0 {
		c.Listener.Confirmations = 6
	}
	if c.Listener.PollInterval == 0 {
		c.Listener.PollInterval = 3 * time.Second
	}
	if c.Listener.ReorgDepth == 0 {
		c.Listener.ReorgDepth = 64
	}
	if c.Listener.BatchSize == 0 {
		c.Listener.BatchSize = 20
	}
	if c.Listener.LookbackBlocks == 0 {
		c.Listener.LookbackBlocks = 10
	}
}

func (c *Config) validate() error {
	if c.Chain.Mode == ModeRPC && c.Chain.RPCURL == "" {
		return fmt.Errorf("chain.rpc_url is required when mode=rpc")
	}
	if c.Chain.Mode != ModeRPC && c.Chain.Mode != ModeMock {
		return fmt.Errorf("invalid chain.mode: %q (want rpc|mock)", c.Chain.Mode)
	}
	return nil
}
