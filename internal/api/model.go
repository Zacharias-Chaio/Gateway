package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"gateway/internal/engine/converter"
	"gateway/internal/store"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// validateModelProfile 落实业务收窄：协议只允许 Modbus RTU / TCP，
// 接口类型只允许 Serial / Network（留空视为未指定，向后兼容）。
// 非法值在保存阶段拦截，而不是等引擎构建采集计划时静默跳过。
func validateModelProfile(m *store.DeviceModel) error {
	var profile struct {
		InterfaceType string `json:"interfaceType"`
		ProtocolType  string `json:"protocolType"`
	}
	if len(m.Profile) > 0 {
		if err := json.Unmarshal(m.Profile, &profile); err != nil {
			return fmt.Errorf("模型 Profile 解析失败: %w", err)
		}
	}
	switch profile.InterfaceType {
	case "", "Serial", "Network":
	default:
		return fmt.Errorf("接口类型只支持 Serial / Network，当前为 %q", profile.InterfaceType)
	}
	switch profile.ProtocolType {
	case string(converter.ModbusRTU), string(converter.ModbusTCP):
	default:
		return fmt.Errorf("协议只支持 Modbus RTU / Modbus TCP，当前为 %q", profile.ProtocolType)
	}
	return nil
}

// ListModels 返回全部设备模型。
func (s *Server) ListModels(w http.ResponseWriter, r *http.Request) {
	var list []store.DeviceModel
	if err := s.DB.Order("profile_index asc").Find(&list).Error; err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	ok(w, list)
}

// SaveModel 创建或更新设备模型（按 ID upsert）。
func (s *Server) SaveModel(w http.ResponseWriter, r *http.Request) {
	var m store.DeviceModel
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		fail(w, http.StatusBadRequest, "JSON 解析失败: "+err.Error())
		return
	}
	if m.ID == "" {
		fail(w, http.StatusBadRequest, "缺少设备模型 ID")
		return
	}
	if err := validateModelProfile(&m); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	// 若已存在则保留原始创建时间，避免 Save 全字段更新把 CreatedAt 写成零值。
	var exist store.DeviceModel
	if err := s.DB.First(&exist, "id = ?", m.ID).Error; err == nil {
		m.CreatedAt = exist.CreatedAt
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.DB.Save(&m).Error; err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.notifyConfigChanged()
	ok(w, m)
}

// DeleteModel 删除指定设备模型。
func (s *Server) DeleteModel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		fail(w, http.StatusBadRequest, "无效的设备模型 ID")
		return
	}
	res := s.DB.Delete(&store.DeviceModel{}, "id = ?", id)
	if res.Error != nil {
		fail(w, http.StatusInternalServerError, res.Error.Error())
		return
	}
	if res.RowsAffected == 0 {
		fail(w, http.StatusNotFound, "设备模型不存在: id="+id)
		return
	}
	s.notifyConfigChanged()
	ok(w, map[string]string{"id": id})
}
