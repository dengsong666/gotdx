package routes

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bensema/gotdx/proto"
	"github.com/bensema/gotdx/types"
)

type monitorResult struct {
	items []proto.MACMarketMonitorItem
	err   error
}

type sequenceMonitorClient struct {
	mu        sync.Mutex
	sequences map[uint8][]monitorResult
	calls     map[uint8]int
}

func (client *sequenceMonitorClient) MACMarketMonitor(market uint8, start uint32, count uint32) ([]proto.MACMarketMonitorItem, error) {
	if start != 0 || count != stockUnusualMonitorCount {
		return nil, fmt.Errorf("unexpected monitor range: start=%d count=%d", start, count)
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.calls == nil {
		client.calls = make(map[uint8]int)
	}
	index := client.calls[market]
	client.calls[market]++
	sequence := client.sequences[market]
	if len(sequence) == 0 {
		return nil, nil
	}
	if index >= len(sequence) {
		index = len(sequence) - 1
	}
	result := sequence[index]
	return append([]proto.MACMarketMonitorItem(nil), result.items...), result.err
}

func (client *sequenceMonitorClient) callCount(market uint8) int {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.calls[market]
}

type blockingMonitorClient struct {
	calls   chan uint8
	release chan struct{}
}

func (client *blockingMonitorClient) MACMarketMonitor(market uint8, start uint32, count uint32) ([]proto.MACMarketMonitorItem, error) {
	client.calls <- market
	<-client.release
	return nil, nil
}

func TestParseStockTargets(t *testing.T) {
	targets, err := parseStockTargets(" 000001,600000,920001,000001 ")
	if err != nil {
		t.Fatalf("parse targets failed: %v", err)
	}
	if len(targets) != 3 {
		t.Fatalf("unexpected target count: %d", len(targets))
	}
	for _, target := range []stockTarget{
		{market: types.MarketSZ.Uint8(), code: "000001"},
		{market: types.MarketSH.Uint8(), code: "600000"},
		{market: types.MarketBJ.Uint8(), code: "920001"},
	} {
		if _, ok := targets[target]; !ok {
			t.Fatalf("missing target: %+v", target)
		}
	}

	invalid := []string{
		"",
		"000001,,600000",
		"000001,ABCDEF",
		"12345",
		"123456",
		"510300",
	}
	for _, value := range invalid {
		t.Run(value, func(t *testing.T) {
			if _, err := parseStockTargets(value); err == nil {
				t.Fatalf("expected invalid stocks error for %q", value)
			}
		})
	}

	many := make([]string, 0, stockUnusualMaxStocks+1)
	for i := 0; i <= stockUnusualMaxStocks; i++ {
		many = append(many, fmt.Sprintf("00%04d", i))
	}
	if _, err := parseStockTargets(strings.Join(many, ",")); err == nil {
		t.Fatal("expected too many stocks error")
	}
}

func TestStockUnusualSSEOptions(t *testing.T) {
	config := stockUnusualSSEConfig{
		pollInterval:      defaultStockUnusualPollInterval,
		heartbeatInterval: defaultStockUnusualHeartbeatInterval,
	}
	WithPollInterval(3 * time.Second)(&config)
	WithHeartbeatInterval(2 * time.Minute)(&config)
	if config.pollInterval != 3*time.Second || config.heartbeatInterval != 2*time.Minute {
		t.Fatalf("unexpected options: %+v", config)
	}
	WithPollInterval(0)(&config)
	WithHeartbeatInterval(-time.Second)(&config)
	if config.pollInterval != 3*time.Second || config.heartbeatInterval != 2*time.Minute {
		t.Fatalf("invalid options should be ignored: %+v", config)
	}
}

func TestStockUnusualSSEHTTPValidation(t *testing.T) {
	mux := http.NewServeMux()
	RegisterStockUnusualSSE(mux, &sequenceMonitorClient{})

	request := httptest.NewRequest(http.MethodPost, StockUnusualSSEPath+"?stocks=000001", nil)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("unexpected method response: code=%d headers=%v", recorder.Code, recorder.Header())
	}

	request = httptest.NewRequest(http.MethodGet, StockUnusualSSEPath+"?stocks=bad", nil)
	recorder = httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "6 位数字") {
		t.Fatalf("unexpected validation response: code=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestStockUnusualSSEBuildsBaselineAndStreamsNewItems(t *testing.T) {
	oldItem := testUnusualItem(1, "09:30:00", "加速拉升")
	newItem1 := testUnusualItem(2, "09:31:00", "大单托盘")
	newItem2 := testUnusualItem(3, "09:32:00", "主力买入")
	client := &sequenceMonitorClient{
		sequences: map[uint8][]monitorResult{
			types.MarketSZ.Uint8(): {
				{items: []proto.MACMarketMonitorItem{oldItem}},
				{items: []proto.MACMarketMonitorItem{oldItem, newItem1, newItem2}},
			},
		},
	}
	handler := newStockUnusualSSEHandler(client, stockUnusualSSEConfig{
		pollInterval:      10 * time.Millisecond,
		heartbeatInterval: time.Hour,
	})
	mux := http.NewServeMux()
	mux.Handle(StockUnusualSSEPath, handler)
	server := httptest.NewServer(mux)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+StockUnusualSSEPath+"?stocks=000001", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("open SSE failed: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", response.StatusCode)
	}
	if got := response.Header.Get("Content-Type"); got != "text/event-stream; charset=utf-8" {
		t.Fatalf("unexpected content type: %s", got)
	}
	if got := response.Header.Get("X-Accel-Buffering"); got != "no" {
		t.Fatalf("unexpected buffering header: %s", got)
	}

	reader := bufio.NewReader(response.Body)
	first := readSSEEvent(t, reader, time.Second)
	second := readSSEEvent(t, reader, time.Second)
	if first.event != "unusual" || second.event != "unusual" {
		t.Fatalf("unexpected events: %+v %+v", first, second)
	}

	var firstPayload map[string]any
	if err := json.Unmarshal(first.data, &firstPayload); err != nil {
		t.Fatalf("decode first event failed: %v", err)
	}
	if firstPayload["code"] != "000001" || firstPayload["desc"] != newItem1.Desc || firstPayload["type"] != float64(newItem1.UnusualType) {
		t.Fatalf("unexpected first payload: %+v", firstPayload)
	}
	if _, exists := firstPayload["market"]; exists {
		t.Fatalf("market must not be exposed: %+v", firstPayload)
	}
	if _, exists := firstPayload["index"]; exists {
		t.Fatalf("index must not be exposed: %+v", firstPayload)
	}
	if _, exists := firstPayload["raw"]; !exists {
		t.Fatalf("raw values must be exposed: %+v", firstPayload)
	}

	var secondPayload unusualEventPayload
	if err := json.Unmarshal(second.data, &secondPayload); err != nil {
		t.Fatalf("decode second event failed: %v", err)
	}
	if secondPayload.Desc != newItem2.Desc {
		t.Fatalf("response order changed: %+v", secondPayload)
	}

	cancel()
	eventually(t, time.Second, func() bool {
		handler.mu.Lock()
		defer handler.mu.Unlock()
		return len(handler.subscribers) == 0 && !handler.running
	})
	if client.callCount(types.MarketSZ.Uint8()) < 2 {
		t.Fatalf("expected at least two polls, got %d", client.callCount(types.MarketSZ.Uint8()))
	}
}

func TestStockUnusualSSEConnectionsSharePolling(t *testing.T) {
	client := &blockingMonitorClient{
		calls:   make(chan uint8, 4),
		release: make(chan struct{}),
	}
	handler := newStockUnusualSSEHandler(client, stockUnusualSSEConfig{
		pollInterval:      time.Hour,
		heartbeatInterval: time.Hour,
	})
	mux := http.NewServeMux()
	mux.Handle(StockUnusualSSEPath, handler)
	server := httptest.NewServer(mux)
	defer server.Close()

	firstResponse, firstCancel := openSSE(t, server.URL+StockUnusualSSEPath+"?stocks=000001")
	defer firstResponse.Body.Close()
	select {
	case <-client.calls:
	case <-time.After(time.Second):
		t.Fatal("first shared poll did not start")
	}

	secondResponse, secondCancel := openSSE(t, server.URL+StockUnusualSSEPath+"?stocks=000002")
	defer secondResponse.Body.Close()
	select {
	case <-client.calls:
		t.Fatal("second subscriber started another concurrent poll")
	case <-time.After(50 * time.Millisecond):
	}

	firstCancel()
	secondCancel()
	close(client.release)
	eventually(t, time.Second, func() bool {
		handler.mu.Lock()
		defer handler.mu.Unlock()
		return len(handler.subscribers) == 0 && len(handler.states) == 0 && !handler.running
	})
}

func TestStockUnusualSSEErrorOnlyOnceAndRecoveryRebuildsBaseline(t *testing.T) {
	handler := newStockUnusualSSEHandler(&sequenceMonitorClient{}, testSSEConfig())
	subscriber := attachTestSubscriber(handler, map[stockTarget]struct{}{
		{market: types.MarketSZ.Uint8(), code: "000001"}: {},
	})

	handler.handlePollError(types.MarketSZ.Uint8())
	handler.handlePollError(types.MarketSZ.Uint8())
	if len(subscriber.messages) != 1 {
		t.Fatalf("same error should be sent once, got %d", len(subscriber.messages))
	}
	message := <-subscriber.messages
	if message.event != "error" || !json.Valid(message.data) {
		t.Fatalf("unexpected error event: %+v", message)
	}

	oldItem := testUnusualItem(1, "09:30:00", "旧异动")
	missedItem := testUnusualItem(2, "09:31:00", "错误期间异动")
	handler.handlePollSuccess(types.MarketSZ.Uint8(), []proto.MACMarketMonitorItem{oldItem, missedItem})
	if len(subscriber.messages) != 0 {
		t.Fatalf("recovery must rebuild baseline without replay, got %d messages", len(subscriber.messages))
	}

	handler.handlePollError(types.MarketSZ.Uint8())
	if len(subscriber.messages) != 1 {
		t.Fatalf("error after recovery should be sent again, got %d", len(subscriber.messages))
	}
	handler.unsubscribe(subscriber)
}

func TestStockUnusualSSEDailyResetAndDedup(t *testing.T) {
	handler := newStockUnusualSSEHandler(&sequenceMonitorClient{}, testSSEConfig())
	handler.now = func() time.Time {
		return time.Date(2026, 7, 21, 0, 0, 1, 0, shanghaiLocation)
	}
	subscriber := attachTestSubscriber(handler, map[stockTarget]struct{}{
		{market: types.MarketSZ.Uint8(), code: "000001"}: {},
	})
	oldItem := testUnusualItem(1, "09:30:00", "昨日同键异动")
	newItem := testUnusualItem(2, "09:31:00", "今日新增异动")

	handler.mu.Lock()
	state := handler.states[types.MarketSZ.Uint8()]
	state.date = "2026-07-20"
	state.initialized = true
	state.seen[makeUnusualEventKey(types.MarketSZ.Uint8(), oldItem)] = struct{}{}
	handler.mu.Unlock()

	handler.handlePollSuccess(types.MarketSZ.Uint8(), []proto.MACMarketMonitorItem{oldItem})
	if len(subscriber.messages) != 0 {
		t.Fatalf("first poll after date change must rebuild baseline")
	}
	handler.handlePollSuccess(types.MarketSZ.Uint8(), []proto.MACMarketMonitorItem{oldItem, newItem})
	if len(subscriber.messages) != 1 {
		t.Fatalf("expected one new event, got %d", len(subscriber.messages))
	}
	handler.handlePollSuccess(types.MarketSZ.Uint8(), []proto.MACMarketMonitorItem{oldItem, newItem})
	if len(subscriber.messages) != 1 {
		t.Fatalf("duplicate event was sent again, got %d", len(subscriber.messages))
	}
	handler.unsubscribe(subscriber)
}

func TestStockUnusualSSESlowSubscriberIsDisconnected(t *testing.T) {
	handler := newStockUnusualSSEHandler(&sequenceMonitorClient{}, testSSEConfig())
	subscriber := attachTestSubscriber(handler, map[stockTarget]struct{}{
		{market: types.MarketSZ.Uint8(), code: "000001"}: {},
	})
	handler.mu.Lock()
	state := handler.states[types.MarketSZ.Uint8()]
	state.date = handler.now().In(shanghaiLocation).Format(time.DateOnly)
	state.initialized = true
	handler.mu.Unlock()

	items := make([]proto.MACMarketMonitorItem, 0, stockUnusualSubscriberBuffer+1)
	for i := 0; i <= stockUnusualSubscriberBuffer; i++ {
		items = append(items, testUnusualItem(uint16(i+1), "09:30:00", fmt.Sprintf("异动%d", i)))
	}
	handler.handlePollSuccess(types.MarketSZ.Uint8(), items)

	select {
	case <-subscriber.done:
	default:
		t.Fatal("slow subscriber was not disconnected")
	}
	handler.mu.Lock()
	defer handler.mu.Unlock()
	if len(handler.subscribers) != 0 || len(handler.states) != 0 {
		t.Fatalf("slow subscriber resources were not released")
	}
}

func TestStockUnusualSSEHeartbeat(t *testing.T) {
	client := &sequenceMonitorClient{sequences: map[uint8][]monitorResult{
		types.MarketSZ.Uint8(): {{items: nil}},
	}}
	handler := newStockUnusualSSEHandler(client, stockUnusualSSEConfig{
		pollInterval:      time.Hour,
		heartbeatInterval: 10 * time.Millisecond,
	})
	mux := http.NewServeMux()
	mux.Handle(StockUnusualSSEPath, handler)
	server := httptest.NewServer(mux)
	defer server.Close()

	response, cancel := openSSE(t, server.URL+StockUnusualSSEPath+"?stocks=000001")
	defer cancel()
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	line := readLine(t, reader, time.Second)
	if line != ": ping\n" {
		t.Fatalf("unexpected heartbeat: %q", line)
	}
}

func testSSEConfig() stockUnusualSSEConfig {
	return stockUnusualSSEConfig{
		pollInterval:      time.Hour,
		heartbeatInterval: time.Hour,
	}
}

func testUnusualItem(index uint16, eventTime string, desc string) proto.MACMarketMonitorItem {
	return proto.MACMarketMonitorItem{
		Index:       index,
		Market:      uint16(types.MarketSZ),
		Code:        "000001",
		Name:        "平安银行",
		Time:        eventTime,
		Desc:        desc,
		Value:       "1.25%",
		UnusualType: 4,
		V1:          1,
		V2:          0.0125,
		V3:          2,
		V4:          3,
	}
}

func attachTestSubscriber(handler *stockUnusualSSEHandler, targets map[stockTarget]struct{}) *stockUnusualSubscriber {
	subscriber := &stockUnusualSubscriber{
		targets:       targets,
		markets:       make(map[uint8]struct{}),
		failedMarkets: make(map[uint8]struct{}),
		messages:      make(chan sseMessage, stockUnusualSubscriberBuffer),
		done:          make(chan struct{}),
	}
	for target := range targets {
		subscriber.markets[target.market] = struct{}{}
	}

	handler.mu.Lock()
	handler.subscribers[subscriber] = struct{}{}
	for market := range subscriber.markets {
		handler.marketRefs[market]++
		handler.states[market] = &stockUnusualMarketState{seen: make(map[unusualEventKey]struct{})}
	}
	handler.mu.Unlock()
	return subscriber
}

func openSSE(t *testing.T, url string) (*http.Response, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		cancel()
		t.Fatalf("open SSE failed: %v", err)
	}
	return response, cancel
}

type parsedSSEEvent struct {
	event string
	data  []byte
}

func readSSEEvent(t *testing.T, reader *bufio.Reader, timeout time.Duration) parsedSSEEvent {
	t.Helper()
	type result struct {
		event parsedSSEEvent
		err   error
	}
	resultCh := make(chan result, 1)
	go func() {
		var event parsedSSEEvent
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				resultCh <- result{err: err}
				return
			}
			line = strings.TrimSuffix(line, "\n")
			line = strings.TrimSuffix(line, "\r")
			if line == "" {
				if event.event != "" {
					resultCh <- result{event: event}
					return
				}
				continue
			}
			if strings.HasPrefix(line, "event: ") {
				event.event = strings.TrimPrefix(line, "event: ")
			}
			if strings.HasPrefix(line, "data: ") {
				event.data = []byte(strings.TrimPrefix(line, "data: "))
			}
		}
	}()

	select {
	case got := <-resultCh:
		if got.err != nil {
			t.Fatalf("read SSE event failed: %v", got.err)
		}
		return got.event
	case <-time.After(timeout):
		t.Fatal("timed out waiting for SSE event")
		return parsedSSEEvent{}
	}
}

func readLine(t *testing.T, reader *bufio.Reader, timeout time.Duration) string {
	t.Helper()
	type result struct {
		line string
		err  error
	}
	resultCh := make(chan result, 1)
	go func() {
		line, err := reader.ReadString('\n')
		resultCh <- result{line: line, err: err}
	}()
	select {
	case got := <-resultCh:
		if got.err != nil && !errors.Is(got.err, io.EOF) {
			t.Fatalf("read line failed: %v", got.err)
		}
		return got.line
	case <-time.After(timeout):
		t.Fatal("timed out waiting for SSE line")
		return ""
	}
}

func eventually(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}
