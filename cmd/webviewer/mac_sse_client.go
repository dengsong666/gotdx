package main

import (
	"fmt"
	"log"
	"sync"

	gotdx "github.com/bensema/gotdx"
	"github.com/bensema/gotdx/proto"
)

// macSSEClient 为异动 SSE 使用独立的短连接客户端，并在 MAC 主站之间故障转移。
// 单个主站出现读超时时，不让后续重试继续命中同一台异常主站。
type macSSEClient struct {
	hosts      []string
	timeoutSec int

	mu        sync.Mutex
	preferred int
}

func newMACSSEClient() *macSSEClient {
	return &macSSEClient{
		hosts:      gotdx.MACHostAddresses(),
		timeoutSec: 6,
	}
}

func (client *macSSEClient) MACMarketMonitor(market uint8, start uint32, count uint32) ([]proto.MACMarketMonitorItem, error) {
	if len(client.hosts) == 0 {
		return nil, fmt.Errorf("没有配置 MAC 行情主站")
	}

	preferred := client.preferredHost()
	var lastErr error
	for offset := range client.hosts {
		index := (preferred + offset) % len(client.hosts)
		host := client.hosts[index]
		macClient := gotdx.NewMAC(
			gotdx.WithMacTCPAddress(host),
			gotdx.WithMacTCPAddressPool(),
			gotdx.WithTimeoutSec(client.timeoutSec),
		)
		items, err := macClient.MACMarketMonitor(market, start, count)
		_ = macClient.Disconnect()
		if err == nil {
			client.setPreferredHost(index)
			return items, nil
		}
		lastErr = fmt.Errorf("host=%s: %w", host, err)
		log.Printf(
			"MAC SSE host failed market=%d start=%d count=%d host=%s: %v",
			market,
			start,
			count,
			host,
			err,
		)
	}

	return nil, lastErr
}

// Disconnect 保持与长期客户端相同的生命周期接口；SSE 客户端的每次查询已使用独立短连接。
func (client *macSSEClient) Disconnect() error { return nil }

func (client *macSSEClient) preferredHost() int {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.preferred % len(client.hosts)
}

func (client *macSSEClient) setPreferredHost(index int) {
	client.mu.Lock()
	client.preferred = index
	client.mu.Unlock()
}
