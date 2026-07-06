package api

import (
	"fmt"
	"math/rand"
	"net/http"
	"time"
)

// Realtime 返回设备属性的模拟实时数值（采集业务接入前的占位）。
func (s *Server) Realtime(w http.ResponseWriter, r *http.Request) {
	device := r.URL.Query().Get("device")
	ok(w, map[string]any{
		"device":    device,
		"timestamp": time.Now().Unix(),
		"note":      "mock 数据，待采集业务接入",
	})
}

// SetValue 接收对可写属性的设定值下发（占位）。
func (s *Server) SetValue(w http.ResponseWriter, r *http.Request) {
	ok(w, map[string]string{"status": "accepted"})
}

// Logs 返回模拟通讯日志与误码率（采集业务接入前的占位）。
func (s *Server) Logs(w http.ResponseWriter, r *http.Request) {
	lines := make([]string, 0, 8)
	for i := 0; i < 8; i++ {
		lines = append(lines, fmt.Sprintf("TX: %02X %02X %02X %02X", rand.Intn(256), rand.Intn(256), rand.Intn(256), rand.Intn(256)))
		lines = append(lines, fmt.Sprintf("RX: %02X %02X", rand.Intn(256), rand.Intn(256)))
	}
	ok(w, map[string]any{
		"errorRate": fmt.Sprintf("%.2f%%", rand.Float64()*0.5),
		"lines":     lines,
	})
}
