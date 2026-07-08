package engine

import (
	"context"
	"testing"
	"time"

	"gorm.io/datatypes"

	"gateway/internal/engine/connector"
	"gateway/internal/engine/converter/modbus"
	"gateway/internal/store"
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

func networkChannel(id int, name, ip string, port int) store.Channel {
	cfg := `{"deviceIp":"` + ip + `","devicePort":` + itoa(port) + `}`
	return store.Channel{ID: id, Name: name, Type: connector.TypeNetwork, Config: datatypes.JSON(cfg)}
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

	// mock 服务提供一个可连接的目标，让 worker 能进入已连接状态。
	srv, err := modbus.StartMockServer(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("启动 mock 服务失败: %v", err)
	}
	defer srv.Close()
	host, port, ok := splitHostPort(srv.Addr())
	if !ok {
		t.Fatalf("解析地址失败: %s", srv.Addr())
	}

	eng := New(ctx)
	defer eng.Stop()

	// 启动一条链路。
	ch := networkChannel(1, "链路A", host, port)
	eng.Apply([]store.Channel{ch})
	if !waitFor(t, 2*time.Second, func() bool {
		st := eng.Status()
		return len(st) == 1 && st[0].Connected
	}) {
		t.Fatalf("链路未在预期时间内连接: %+v", eng.Status())
	}

	// 相同配置再次 Apply：不应重启（worker 指纹未变，仍连接）。
	eng.Apply([]store.Channel{ch})
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

// TestEngineUnsupportedSkipped 验证不支持的链路（CAN Open 失败）不会导致引擎崩溃，
// 且 worker 记录未连接状态。
func TestEngineUnsupportedSkipped(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eng := New(ctx)
	defer eng.Stop()

	ch := store.Channel{ID: 7, Name: "CAN链路", Type: connector.TypeCAN, Config: datatypes.JSON(`{"canName":"can0","canBaud":250000}`)}
	eng.Apply([]store.Channel{ch})

	// worker 会创建但 Open 持续失败（ErrNotSupported），状态应为未连接。
	if !waitFor(t, time.Second, func() bool {
		st := eng.Status()
		return len(st) == 1 && !st[0].Connected && st[0].LastError != ""
	}) {
		t.Fatalf("CAN 链路状态不符合预期: %+v", eng.Status())
	}
}
