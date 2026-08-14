package api

import (
	"net/http"

	"gateway/internal/config"
)

// GetHardware 返回硬件接口配置：按类别分组的「丝印标签 -> 设备节点」映射。
// 前端链路配置据此把名称输入框渲染为下拉框，导出 JSON 时用 value（真实设备节点）填充。
func (s *Server) GetHardware(w http.ResponseWriter, r *http.Request) {
	settings, err := config.LoadSettings(s.DB)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取硬件配置失败: "+err.Error())
		return
	}
	ok(w, settings.Hardware)
}
