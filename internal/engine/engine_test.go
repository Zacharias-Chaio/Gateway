package engine

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"gateway/internal/engine/connector"
	"gateway/internal/engine/converter"
)

// waitFor 轮询直到 cond 为真或超时。
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}

func networkChannel(id int, name, ip string, port int) ChannelSpec {
	cfg := `{"deviceIp":"` + ip + `","devicePort":` + itoa(port) + `}`
	return ChannelSpec{ID: id, Name: name, Type: connector.TypeNetwork, Config: []byte(cfg)}
}

// toPlan 将 ChannelSpec 转换为最小可执行的 ChannelPlan（无设备，仅维持连接）。
func toPlan(ch ChannelSpec) ChannelPlan {
	return ChannelPlan{
		ChannelID:   ch.ID,
		ChannelName: ch.Name,
		ChannelType: ch.Type,
		Config:      ch.Config,
		PollMs:      0, // 默认 500ms
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// TestEngineApplyStartStop 验证 Apply 的启动、删除与配置变更重启逻辑。
func TestEngineApplyStartStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 本地 TCP 测试服务提供一个可连接的目标，让 worker 能进入已连接状态。
	srv, err := startEchoServer(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("启动 TCP 测试服务失败: %v", err)
	}
	defer srv.close()
	host, port, ok := splitHostPort(srv.addr())
	if !ok {
		t.Fatalf("解析地址失败: %s", srv.addr())
	}

	eng := New(ctx)
	defer eng.Stop()

	// 启动一条链路。
	ch := networkChannel(1, "链路A", host, port)
	eng.Apply([]ChannelPlan{toPlan(ch)})
	if !waitFor(t, 2*time.Second, func() bool {
		st := eng.Status()
		return len(st) == 1 && st[0].Connected
	}) {
		t.Fatalf("链路未在预期时间内连接: %+v", eng.Status())
	}

	// 相同配置再次 Apply：不应重启（worker 指纹未变，仍连接）。
	eng.Apply([]ChannelPlan{toPlan(ch)})
	if st := eng.Status(); len(st) != 1 {
		t.Fatalf("重复 Apply 后链路数错误: %d", len(st))
	}

	// 删除链路：worker 应被停止移除。
	eng.Apply(nil)
	if !waitFor(t, 2*time.Second, func() bool {
		return len(eng.Status()) == 0
	}) {
		t.Fatalf("删除后仍有链路: %+v", eng.Status())
	}
}

// TestEngineUnsupportedSkipped 验证不支持的链路类型（如历史 CAN 或未知类型）
// 在驱动创建阶段被跳过并记录警告，不会创建 worker，也不影响引擎运行。
func TestEngineUnsupportedSkipped(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eng := New(ctx)
	defer eng.Stop()

	ch := ChannelSpec{ID: 7, Name: "CAN链路", Type: "CAN", Config: []byte(`{"canName":"can0"}`)}
	eng.Apply([]ChannelPlan{toPlan(ch)})

	if st := eng.Status(); len(st) != 0 {
		t.Fatalf("不支持的链路类型应被跳过: %+v", st)
	}
}

func TestEngineStopInterruptsBlockedReceive(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, err := startSilentServer(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("启动 TCP 测试服务失败: %v", err)
	}
	defer srv.close()
	host, port, ok := splitHostPort(srv.addr())
	if !ok {
		t.Fatalf("解析地址失败: %s", srv.addr())
	}

	eng := New(ctx)
	plan := toPlan(networkChannel(1, "链路A", host, port))
	plan.Devices = []DevicePlan{{
		UnitID: 1, ModelName: "设备A", Conv: blockingFrameIO{},
		Groups: []converter.RegGroup{{ReadFC: 3, StartAddr: 0, Quantity: 1}},
	}}
	eng.Apply([]ChannelPlan{plan})
	if !waitFor(t, time.Second, func() bool {
		status := eng.Status()
		return len(status) == 1 && status[0].Connected
	}) {
		t.Fatalf("链路未在预期时间内连接: %+v", eng.Status())
	}

	started := time.Now()
	eng.Stop()
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("Stop 等待阻塞读取过久: %s", elapsed)
	}
}

type telemetrySink struct {
	mu     sync.Mutex
	events []TelemetryEvent
	writes []WriteResultEvent
}

func (s *telemetrySink) PublishTelemetry(event TelemetryEvent) {
	s.mu.Lock()
	s.events = append(s.events, event)
	s.mu.Unlock()
}

func (s *telemetrySink) PublishWriteResult(event WriteResultEvent) {
	s.mu.Lock()
	s.writes = append(s.writes, event)
	s.mu.Unlock()
}

func (s *telemetrySink) writeResults() []WriteResultEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]WriteResultEvent, len(s.writes))
	copy(out, s.writes)
	return out
}

// TestWorkerDrainPendingWritesPublishesFailure 验证 worker 退出前排空写队列：
// 带 RequestID 的命令（消息总线来源）必须收到失败回执，HTTP 来源命令静默丢弃。
func TestWorkerDrainPendingWritesPublishesFailure(t *testing.T) {
	sink := &telemetrySink{}
	worker := newWorker(1, "链路A", "", connector.Config{}, nil, ChannelPlan{}, sink)
	worker.writeCh <- WriteCommand{RequestID: "req-1", DeviceIndex: 0, PropName: "频率", RawValue: 50}
	worker.writeCh <- WriteCommand{RequestID: "req-2", DeviceIndex: 0, PropName: "电压", RawValue: 220}
	worker.writeCh <- WriteCommand{DeviceIndex: 0, PropName: "电流", RawValue: 5} // 无 RequestID

	worker.drainPendingWrites()

	results := sink.writeResults()
	if len(results) != 2 {
		t.Fatalf("应为两条带 RequestID 的命令发布失败回执: got %d", len(results))
	}
	seen := map[string]bool{}
	for _, r := range results {
		if r.OK || r.Error == "" || r.ChannelID != 1 {
			t.Fatalf("回执应为失败且携带原因: %+v", r)
		}
		seen[r.RequestID] = true
	}
	if !seen["req-1"] || !seen["req-2"] {
		t.Fatalf("回执 RequestID 不完整: %+v", results)
	}
}

func TestWorkerPublishAllOffline(t *testing.T) {
	sink := &telemetrySink{}
	worker := newWorker(1, "链路A", "", connector.Config{}, nil, ChannelPlan{Devices: []DevicePlan{
		{Name: "设备A", Props: []converter.PropMeta{{Name: OnlinePropName, PropID: "online"}}},
		{Name: "设备B", Props: []converter.PropMeta{{Name: OnlinePropName, PropID: "online"}}},
	}}, sink)

	worker.publishAllOffline()

	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.events) != 2 {
		t.Fatalf("离线遥测数量错误: got %d want 2", len(sink.events))
	}
	for _, event := range sink.events {
		status, ok := event.Properties["online"]
		if !ok || status.Value != int64(0) || len(event.Properties) != 1 {
			t.Fatalf("离线遥测应仅包含在线状态=0: %+v", event.Properties)
		}
	}
}

func TestWorkerWaitReconnectContinuouslyPublishesOffline(t *testing.T) {
	sink := &telemetrySink{}
	worker := newWorker(1, "链路A", "", connector.Config{PollInterval: 10}, nil, ChannelPlan{Devices: []DevicePlan{
		{Name: "设备A", Props: []converter.PropMeta{{Name: OnlinePropName, PropID: "online"}}},
	}}, sink)

	if !worker.waitReconnect(context.Background(), 50*time.Millisecond) {
		t.Fatal("重连等待不应被取消")
	}

	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.events) < 3 {
		t.Fatalf("重连等待期间应持续发布离线遥测: got %d want at least 3", len(sink.events))
	}
}

func TestWorkerPublishesOfflineAfterReconnectAttemptsExhausted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sink := &telemetrySink{}
	worker := newWorker(1, "链路A", "", connector.Config{PollInterval: 10}, nil, ChannelPlan{Devices: []DevicePlan{
		{Name: "设备A", Props: []converter.PropMeta{{Name: OnlinePropName, PropID: "online"}}},
	}}, sink)
	done := make(chan struct{})
	go func() {
		worker.publishOfflineUntilStopped(ctx)
		close(done)
	}()

	if !waitFor(t, 100*time.Millisecond, func() bool {
		sink.mu.Lock()
		defer sink.mu.Unlock()
		return len(sink.events) >= 3
	}) {
		t.Fatal("重连停止后应持续发布离线遥测")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("取消后离线扇出未停止")
	}
}

func TestBuildPlansPreservesFrontendPropertyID(t *testing.T) {
	properties := `[{"id":"voltage","name":"电压","dataType":"int","readFunctionCode":3,"registerBase":0,"registerOffset":0,"startBit":0,"endBit":15,"accessMode":"r"}]`
	model := ModelSpec{
		ID: "model-1", Name: "设备模型",
		Profile:    []byte(`{"protocolType":"Modbus TCP"}`),
		Properties: []byte(properties),
	}
	devices := []DeviceMount{{Index: 0, CommNo: 1, ModelID: model.ID}}
	deviceJSON, err := json.Marshal(devices)
	if err != nil {
		t.Fatalf("编码设备挂载: %v", err)
	}
	channel := networkChannel(1, "链路A", "127.0.0.1", 502)
	channel.Devices = deviceJSON

	plans, warnings := BuildPlans([]ChannelSpec{channel}, []ModelSpec{model})
	if len(warnings) != 0 || len(plans) != 1 || len(plans[0].Devices) != 1 {
		t.Fatalf("采集计划构建失败: plans=%+v warnings=%v", plans, warnings)
	}
	props := plans[0].Devices[0].Props
	if len(props) != 1 || props[0].PropID != "voltage" {
		t.Fatalf("前端属性 ID 未保留: %+v", props)
	}
	if plans[0].Devices[0].Index != 0 {
		t.Fatalf("设备链路内序号错误: %+v", plans[0].Devices[0])
	}
}
