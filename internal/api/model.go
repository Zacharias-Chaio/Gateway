package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"gateway/internal/store"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

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
	s.reloadEngine()
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
	s.reloadEngine()
	ok(w, map[string]string{"id": id})
}
