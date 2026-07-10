package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"gateway/internal/engine"
)

// Realtime 返回设备属性的实时采集值。
// 查询参数 device 格式为 "channelID/deviceIndex"（如 "1/0"），返回该链路设备下的所有缓存属性。
func (s *Server) Realtime(w http.ResponseWriter, r *http.Request) {
	device := r.URL.Query().Get("device")
	if device == "" {
		ok(w, map[string]any{"device": "", "timestamp": time.Now().Unix(), "values": map[string]any{}})
		return
	}

	// 尝试从引擎获取实时缓存值
	if s.Engine != nil {
		channelID, devIdx := parseDeviceKey(device)
		if channelID >= 0 {
			all := s.Engine.Values(channelID)
			values := make(map[string]any)
			prefix := fmt.Sprintf("%d/", devIdx)
			for k, v := range all {
				if len(k) > len(prefix) && k[:len(prefix)] == prefix {
					propName := k[len(prefix):]
					values[propName] = map[string]any{
						"value": v.Value,
						"ts":    v.Timestamp.UnixMilli(),
					}
				}
			}
			ok(w, map[string]any{
				"device":    device,
				"timestamp": time.Now().Unix(),
				"values":    values,
			})
			return
		}
	}

	// 引擎未启用或无缓存数据，返回空
	ok(w, map[string]any{
		"device":    device,
		"timestamp": time.Now().Unix(),
		"values":    map[string]any{},
		"note":      "无采集数据（引擎未启动或设备未连接）",
	})
}

// SetValue 接收对可写属性的设定值，通过引擎下发写命令。
// 请求体 JSON: { "channelId": 1, "deviceIndex": 0, "propName": "频率", "value": 50.0 }
func (s *Server) SetValue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ChannelID   int     `json:"channelId"`
		DeviceIndex int     `json:"deviceIndex"`
		PropName    string  `json:"propName"`
		Value       float64 `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fail(w, http.StatusBadRequest, "JSON 解析失败: "+err.Error())
		return
	}
	if req.ChannelID <= 0 || req.PropName == "" {
		fail(w, http.StatusBadRequest, "缺少 channelId 或 propName")
		return
	}

	if s.Engine == nil {
		fail(w, http.StatusServiceUnavailable, "引擎未启动，无法下发写命令")
		return
	}

	cmd := engine.WriteCommand{
		DeviceIndex: req.DeviceIndex,
		PropName:    req.PropName,
		RawValue:    req.Value,
	}
	if !s.Engine.Submit(req.ChannelID, cmd) {
		fail(w, http.StatusServiceUnavailable, "写命令投递失败：链路不存在或队列已满")
		return
	}
	ok(w, map[string]string{"status": "accepted"})
}

// Logs 返回通讯日志（当前从 logx 系统日志出口获取，API 层做格式封装）。
func (s *Server) Logs(w http.ResponseWriter, r *http.Request) {
	// 通讯日志已由 logx 统一管理，此处返回基本状态。
	ok(w, map[string]any{
		"timestamp": time.Now().Unix(),
		"note":      "通讯日志请查看 /api/syslog 实时流",
	})
}

// parseDeviceKey 解析 "channelID/deviceIndex" 格式。
func parseDeviceKey(s string) (channelID, devIdx int) {
	for i, c := range s {
		if c == '/' {
			id, _ := strconv.Atoi(s[:i])
			idx, _ := strconv.Atoi(s[i+1:])
			return id, idx
		}
	}
	id, _ := strconv.Atoi(s)
	return id, 0
}
