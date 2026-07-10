package engine

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"gateway/internal/engine/connector"
)

// TestTCPDriverRoundTrip 通过 mock 服务验证 TCP 驱动的 Open/Send/Receive/Close。
func TestTCPDriverRoundTrip(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, err := startEchoServer(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("启动 TCP 测试服务失败: %v", err)
	}
	defer srv.close()

	host, portStr, ok := splitHostPort(srv.addr())
	if !ok {
		t.Fatalf("解析测试服务地址失败: %s", srv.addr())
	}

	drv, err := connector.NewDriver(connector.Config{Type: connector.TypeNetwork, DeviceIP: host, DevicePort: portStr})
	if err != nil {
		t.Fatalf("创建 TCP 驱动失败: %v", err)
	}
	if err := drv.Open(ctx); err != nil {
		t.Fatalf("打开链路失败: %v", err)
	}
	defer drv.Close()

	req := []byte{0x01, 0x03, 0x00, 0x00, 0x00, 0x02}
	if _, err := drv.Send(req); err != nil {
		t.Fatalf("发送失败: %v", err)
	}

	buf := make([]byte, 256)
	n, err := drv.Receive(buf, 2*time.Second)
	if err != nil {
		t.Fatalf("接收失败: %v", err)
	}
	resp := string(buf[:n])
	if resp != "ok" {
		t.Fatalf("响应错误: got %q want ok", resp)
	}

	if err := drv.Refresh(); err != nil {
		t.Fatalf("刷新失败: %v", err)
	}
}

type tcpEchoServer struct{ ln net.Listener }

func startEchoServer(ctx context.Context, addr string) (*tcpEchoServer, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	s := &tcpEchoServer{ln: ln}
	go func() {
		<-ctx.Done()
		s.close()
	}()
	go s.accept()
	return s, nil
}

func (s *tcpEchoServer) addr() string { return s.ln.Addr().String() }

func (s *tcpEchoServer) close() { _ = s.ln.Close() }

func (s *tcpEchoServer) accept() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go func() {
			defer conn.Close()
			buf := make([]byte, 256)
			if _, err := conn.Read(buf); err == nil || err == io.EOF {
				_, _ = conn.Write([]byte("ok"))
			}
		}()
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
