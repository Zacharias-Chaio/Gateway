package engine

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"gateway/internal/engine/connector"
	"gateway/internal/engine/converter/modbus"
)

// TestTCPDriverRoundTrip 通过 mock 服务验证 TCP 驱动的 Open/Send/Receive/Close。
func TestTCPDriverRoundTrip(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, err := modbus.StartMockServer(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("启动 mock 服务失败: %v", err)
	}
	defer srv.Close()

	host, portStr, ok := splitHostPort(srv.Addr())
	if !ok {
		t.Fatalf("解析 mock 地址失败: %s", srv.Addr())
	}

	drv, err := connector.NewDriver(connector.Config{Type: connector.TypeNetwork, DeviceIP: host, DevicePort: portStr})
	if err != nil {
		t.Fatalf("创建 TCP 驱动失败: %v", err)
	}
	if err := drv.Open(ctx); err != nil {
		t.Fatalf("打开链路失败: %v", err)
	}
	defer drv.Close()

	req := modbus.BuildTCPRequest(1, 1, 0, 2)
	if _, err := drv.Send(req); err != nil {
		t.Fatalf("发送失败: %v", err)
	}

	buf := make([]byte, 256)
	n, err := drv.Receive(buf, 2*time.Second)
	if err != nil {
		t.Fatalf("接收失败: %v", err)
	}
	resp := buf[:n]
	// 期望 MBAP 事务号回填为 1，功能码 0x03，字节数 4，寄存器值 10、20。
	if n < 9 {
		t.Fatalf("响应过短: %X", resp)
	}
	if txID := binary.BigEndian.Uint16(resp[0:2]); txID != 1 {
		t.Fatalf("事务号错误: got %d want 1", txID)
	}
	if resp[7] != 0x03 {
		t.Fatalf("功能码错误: got %02X want 03", resp[7])
	}
	if resp[8] != 4 {
		t.Fatalf("字节数错误: got %d want 4", resp[8])
	}
	reg0 := binary.BigEndian.Uint16(resp[9:11])
	reg1 := binary.BigEndian.Uint16(resp[11:13])
	if reg0 != 10 || reg1 != 20 {
		t.Fatalf("寄存器值错误: got %d,%d want 10,20", reg0, reg1)
	}

	if err := drv.Refresh(); err != nil {
		t.Fatalf("刷新失败: %v", err)
	}
}

// splitHostPort 从 "host:port" 解析出 host 与整型 port。
func splitHostPort(addr string) (string, int, bool) {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			host := addr[:i]
			port := 0
			for _, c := range addr[i+1:] {
				if c < '0' || c > '9' {
					return "", 0, false
				}
				port = port*10 + int(c-'0')
			}
			return host, port, true
		}
	}
	return "", 0, false
}
