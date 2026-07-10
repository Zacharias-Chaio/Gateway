package connector

import (
	"testing"

	"gorm.io/datatypes"

	"gateway/internal/store"

	"go.bug.st/serial"
)

// TestParseConfigSerial 验证串口链路 JSON 解析与字段映射。
func TestParseConfigSerial(t *testing.T) {
	ch := store.Channel{
		ID:   1,
		Name: "串口A",
		Type: TypeSerial,
		Config: datatypes.JSON(`{
			"serialName":"/dev/ttyS1","baudRate":9600,"dataBits":8,
			"parity":"Even","stopBits":1,"frameInterval":50,
			"reconnectRetries":3,"resendRetries":2,"pollInterval":1000
		}`),
	}
	cfg, err := ParseConfig(ch)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if cfg.SerialName != "/dev/ttyS1" || cfg.BaudRate != 9600 || cfg.DataBits != 8 {
		t.Fatalf("串口基本参数错误: %+v", cfg)
	}
	if cfg.Parity != "Even" || cfg.StopBits != "1" {
		t.Fatalf("校验/停止位解析错误: parity=%q stopBits=%q", cfg.Parity, cfg.StopBits)
	}
	if cfg.FrameInterval != 50 || cfg.ReconnectRetries != 3 || cfg.ResendRetries != 2 {
		t.Fatalf("重试/节流参数错误: %+v", cfg)
	}
	if cfg.PollInterval != 1000 {
		t.Fatalf("PollInterval 应为 1000, got %d", cfg.PollInterval)
	}
	if cfg.Target() != "/dev/ttyS1" {
		t.Fatalf("Target 错误: %q", cfg.Target())
	}
}

// TestParseConfigNetworkTarget 验证网络链路目标地址拼装。
func TestParseConfigNetworkTarget(t *testing.T) {
	ch := store.Channel{
		Type:   TypeNetwork,
		Config: datatypes.JSON(`{"deviceIp":"192.168.1.10","devicePort":502}`),
	}
	cfg, err := ParseConfig(ch)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if cfg.Target() != "192.168.1.10:502" {
		t.Fatalf("网络 Target 错误: %q", cfg.Target())
	}
}

// TestToParityStopBits 验证串口枚举映射与非法值报错。
func TestToParityStopBits(t *testing.T) {
	cases := []struct {
		in   string
		want serial.Parity
	}{
		{"", serial.NoParity},
		{"None", serial.NoParity},
		{"Even", serial.EvenParity},
		{"Odd", serial.OddParity},
	}
	for _, c := range cases {
		got, err := toParity(c.in)
		if err != nil || got != c.want {
			t.Fatalf("toParity(%q)=%v,%v want %v", c.in, got, err, c.want)
		}
	}
	if _, err := toParity("bogus"); err == nil {
		t.Fatalf("非法校验位应报错")
	}

	sb := []struct {
		in   string
		want serial.StopBits
	}{
		{"", serial.OneStopBit},
		{"1", serial.OneStopBit},
		{"1.5", serial.OnePointFiveStopBits},
		{"2", serial.TwoStopBits},
	}
	for _, c := range sb {
		got, err := toStopBits(c.in)
		if err != nil || got != c.want {
			t.Fatalf("toStopBits(%q)=%v,%v want %v", c.in, got, err, c.want)
		}
	}
	if _, err := toStopBits("3"); err == nil {
		t.Fatalf("非法停止位应报错")
	}
}

// TestNewDriverUnsupportedType 验证未知链路类型返回 ErrNotSupported。
func TestNewDriverUnsupportedType(t *testing.T) {
	if _, err := NewDriver(Config{Type: "Bluetooth"}); err != ErrNotSupported {
		t.Fatalf("未知类型应返回 ErrNotSupported, got %v", err)
	}
}
