package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"gateway/internal/store"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// ListChannels 返回全部链路。
func (s *Server) ListChannels(w http.ResponseWriter, r *http.Request) {
	var list []store.Channel
	if err := s.DB.Order("id asc").Find(&list).Error; err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	ok(w, list)
}

// SaveChannel 创建或更新链路，并回传下发配置 JSON。
// id==0 视为新建：交给 SQLite 自增主键分配，避免 GORM 把零值主键当作 upsert 造成重复行。
func (s *Server) SaveChannel(w http.ResponseWriter, r *http.Request) {
	var c store.Channel
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		fail(w, http.StatusBadRequest, "JSON 解析失败: "+err.Error())
		return
	}
	// 冲突检测：同一串口/CAN 端口或网络 IP+端口不能被多个链路共用。
	if key := channelResourceKey(c.Type, c.Config); key != "" {
		var others []store.Channel
		if err := s.DB.Where("id <> ?", c.ID).Find(&others).Error; err != nil {
			fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		for _, o := range others {
			if channelResourceKey(o.Type, o.Config) == key {
				fail(w, http.StatusConflict, "链路配置冲突：串口/CAN 端口或网络 IP+端口已被链路「"+o.Name+"」占用，不能被多个链路共用")
				return
			}
		}
	}
	if c.ID == 0 {
		// 明确新建：让 AUTOINCREMENT 分配主键
		if err := s.DB.Create(&c).Error; err != nil {
			fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		ok(w, c)
		return
	}
	// 更新：主键已存在才覆盖，避免伪造主键插入脏数据
	var exist store.Channel
	err := s.DB.First(&exist, c.ID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			fail(w, http.StatusNotFound, "链路不存在: id="+strconv.Itoa(c.ID))
			return
		}
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	c.CreatedAt = exist.CreatedAt // 保留原始创建时间，避免 Save 写入零值
	if err := s.DB.Save(&c).Error; err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	ok(w, c)
}

// DeleteChannel 删除指定链路。
func (s *Server) DeleteChannel(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		fail(w, http.StatusBadRequest, "无效的链路 ID")
		return
	}
	res := s.DB.Delete(&store.Channel{}, "id = ?", id)
	if res.Error != nil {
		fail(w, http.StatusInternalServerError, res.Error.Error())
		return
	}
	if res.RowsAffected == 0 {
		fail(w, http.StatusNotFound, "链路不存在: id="+strconv.Itoa(id))
		return
	}
	ok(w, map[string]int{"id": id})
}

// channelResourceKey 提取链路占用的硬件资源唯一键：串口/CAN 以端口名唯一，
// 网络以 IP+端口唯一。返回空串表示无可比较的资源占用，不参与冲突判断。
func channelResourceKey(typ string, config []byte) string {
	if len(config) == 0 {
		return ""
	}
	var cfg struct {
		SerialName string          `json:"serialName"`
		CanName    string          `json:"canName"`
		DeviceIP   string          `json:"deviceIp"`
		DevicePort json.RawMessage `json:"devicePort"`
	}
	if err := json.Unmarshal(config, &cfg); err != nil {
		return ""
	}
	switch typ {
	case "Serial":
		if s := strings.TrimSpace(cfg.SerialName); s != "" {
			return "Serial|" + strings.ToLower(s)
		}
	case "CAN":
		if s := strings.TrimSpace(cfg.CanName); s != "" {
			return "CAN|" + strings.ToLower(s)
		}
	case "Network":
		ip := strings.TrimSpace(cfg.DeviceIP)
		port := strings.TrimSpace(string(cfg.DevicePort))
		if ip != "" && port != "" && port != "null" {
			return "Network|" + strings.ToLower(ip) + ":" + port
		}
	}
	return ""
}
