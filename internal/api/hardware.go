package api

import (
	"net/http"
	"os"

	"gopkg.in/yaml.v3"
)

// GetHardware 返回硬件接口配置：按类别分组的「丝印标签 -> 设备节点」映射。
// 前端链路配置据此把名称输入框渲染为下拉框，导出 JSON 时用 value（真实设备节点）填充。
func (s *Server) GetHardware(w http.ResponseWriter, r *http.Request) {
	path := s.HardwarePath
	if path == "" {
		path = "hardware.yaml"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		// 配置文件缺失时回退到内置默认，保证前端可用。
		ok(w, defaultHardware())
		return
	}
	hw := parseHardwareYAML(data)
	if len(hw) == 0 {
		hw = defaultHardware()
	}
	ok(w, hw)
}

// parseHardwareYAML 解析「类别 -> {丝印标签: 设备节点}」两级映射：
//
//	Category:
//	  LABEL: node
//
// 解析失败（格式非法或结构不符）时返回 nil，交由调用方回退默认配置。
func parseHardwareYAML(data []byte) map[string]map[string]string {
	out := map[string]map[string]string{}
	if err := yaml.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

// defaultHardware 内置默认硬件接口，作为配置文件缺失时的回退。
func defaultHardware() map[string]map[string]string {
	return map[string]map[string]string{
		"Serial":   {"COM1": "/dev/ttyS1", "COM2": "/dev/ttyS2"},
		"Ethernet": {"ETH1": "eth0", "ETH2": "eth2"},
		"CAN":      {"CAN1": "can0", "CAN2": "can1"},
	}
}
