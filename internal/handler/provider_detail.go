package handler

import (
	"encoding/json"
	"net/http"

	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
)

// customModelsScope is the KV scope storing per-connection custom model lists.
const customModelsScope = "providerModels"

// customModel is a user-defined model attached to a specific connection.
type customModel struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// loadCustomModels reads the custom model list for a connection from the KV store.
func (s *Server) loadCustomModels(connID string) []customModel {
	raw, err := s.DB.KVGet(customModelsScope, connID)
	if err != nil || raw == "" {
		return nil
	}
	var models []customModel
	if err := json.Unmarshal([]byte(raw), &models); err != nil {
		return nil
	}
	return models
}

// saveCustomModels persists the custom model list for a connection.
func (s *Server) saveCustomModels(connID string, models []customModel) error {
	data, err := json.Marshal(models)
	if err != nil {
		return err
	}
	return s.DB.KVSet(customModelsScope, connID, string(data))
}

// ModelMetaOverride stores user-customized metadata for a model.
type ModelMetaOverride struct {
	DisplayName   string `json:"displayName,omitempty"`
	ContextLength int    `json:"contextLength,omitempty"`
	MaxOutput     int    `json:"maxOutputTokens,omitempty"`
}

// ProviderModelItem represents a model in the provider detail response with its enabled and free status.
type ProviderModelItem struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Enabled       bool   `json:"enabled"`
	IsFree        bool   `json:"isFree,omitempty"`
	ContextLength int    `json:"contextLength,omitempty"`
	MaxOutput     int    `json:"maxOutputTokens,omitempty"`
	CanEdit       bool   `json:"canEdit"`
	HasOverride   bool   `json:"hasOverride,omitempty"`
}

// handleGetProviderModels returns registry models + live synced models + custom models for a connection.
func (s *Server) handleGetProviderModels(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	conn, err := s.DB.GetConnection(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found"})
		return
	}

	// 1. 获取硬编码备用 / 默认 Registry 模型
	registryModels := provider.GetRegistryModels(conn.Provider)
	if registryModels == nil {
		registryModels = []provider.ModelRef{}
	}

	// 判断是否为免密未授权的 OpenCode 连接（仅允许免费模型）
	isUnauthOpenCode := conn.Provider == "opencode" && (conn.AuthType == "none" || conn.Data.APIKey == "")

	// 2. 获取实时同步抓取的 Live 缓存模型并合并
	seen := make(map[string]bool)
	var unifiedModels []provider.ModelRef

	if raw, err := s.DB.KVGet("providerModelCache", conn.Provider); err == nil && raw != "" {
		var cached model.CachedModels
		if err := json.Unmarshal([]byte(raw), &cached); err == nil && len(cached.Models) > 0 {
			for _, m := range cached.Models {
				// 若 OpenCode 未填 API Key，过滤只留下免费模型
				if isUnauthOpenCode && !provider.IsOpenCodeFreeModel(m.ID) {
					continue
				}
				seen[m.ID] = true
				name := m.DisplayName
				if name == "" {
					name = m.ID
				}
				unifiedModels = append(unifiedModels, provider.ModelRef{
					ID:   m.ID,
					Name: name,
				})
			}
		}
	}

	// 若无 Live 模型或补齐 Registry 中独特项
	for _, rm := range registryModels {
		if isUnauthOpenCode && !provider.IsOpenCodeFreeModel(rm.ID) {
			continue
		}
		if !seen[rm.ID] {
			seen[rm.ID] = true
			unifiedModels = append(unifiedModels, rm)
		}
	}

	// 如果处于 OpenCode 免密模式且尚未抓取或未命中任何免费模型，使用 OpenCode 标准免费兜底模型
	if isUnauthOpenCode && len(unifiedModels) == 0 {
		for _, fm := range provider.GetOpenCodeFreeModels() {
			if !seen[fm.ID] {
				seen[fm.ID] = true
				unifiedModels = append(unifiedModels, fm)
			}
		}
	}

	// 获取全局禁用模型映射
	disabledMap, _ := s.DB.KVList("disabledModels")
	isModelDisabled := func(modelID string) bool {
		if disabledMap == nil {
			return false
		}
		fullID := conn.Provider + "/" + modelID
		return disabledMap[fullID] != "" || disabledMap[modelID] != ""
	}
	cacheIndex := s.loadModelCacheIndex()
	overrides, _ := s.DB.KVList("modelMetaOverrides")

	buildModelItem := func(id, name string) ProviderModelItem {
		fullID := conn.Provider + "/" + id
		item := ProviderModelItem{
			ID:      id,
			Name:    name,
			Enabled: !isModelDisabled(id),
			IsFree:  conn.Provider == "opencode" && provider.IsOpenCodeFreeModel(id),
		}

		// 优先使用用户自定义设置的元数据覆盖
		if overrides != nil && overrides[fullID] != "" {
			var ov ModelMetaOverride
			if err := json.Unmarshal([]byte(overrides[fullID]), &ov); err == nil {
				if ov.DisplayName != "" {
					item.Name = ov.DisplayName
				}
				item.ContextLength = ov.ContextLength
				item.MaxOutput = ov.MaxOutput
				item.CanEdit = true
				item.HasOverride = true
				return item
			}
		}

		// 其次使用动态同步或内置模型元数据
		if cachedMeta, ok := cacheIndex[fullID]; ok && cachedMeta != nil {
			item.ContextLength = cachedMeta.ContextLength
			item.MaxOutput = cachedMeta.MaxOutput
			// 规则：若上游动态同步抓取已有确切元数据（如 ContextLength > 0），则锁定编辑，仅允许开关
			if cachedMeta.ContextLength > 0 {
				item.CanEdit = false
			} else {
				item.CanEdit = true
			}
		} else {
			// 未包含确切动态元数据的内置或自定义模型，允许用户自由编辑
			item.CanEdit = true
		}
		return item
	}

	registryItems := []ProviderModelItem{}
	for _, m := range unifiedModels {
		registryItems = append(registryItems, buildModelItem(m.ID, m.Name))
	}

	customModels := s.loadCustomModels(conn.ID)
	customItems := []ProviderModelItem{}
	for _, cm := range customModels {
		customItems = append(customItems, buildModelItem(cm.ID, cm.Name))
	}
	regInfo, _ := provider.GetProvider(conn.Provider)
	writeJSON(w, http.StatusOK, map[string]any{
		"provider":       conn.Provider,
		"authType":       conn.AuthType,
		"authModes":      regInfo.AuthModes,
		"defaultHeaders": regInfo.Headers,
		"hasApiKey":      conn.Data.APIKey != "",
		"isFreeMode":     isUnauthOpenCode,
		"registryModels": registryItems,
		"customModels":   customItems,
	})
}

// handleAddProviderModel adds a custom model to a connection.
func (s *Server) handleAddProviderModel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	conn, err := s.DB.GetConnection(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found"})
		return
	}

	var req customModel
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.ID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "model id is required"})
		return
	}
	if req.Name == "" {
		req.Name = req.ID
	}

	models := s.loadCustomModels(conn.ID)
	for _, m := range models {
		if m.ID == req.ID {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "model already exists"})
			return
		}
	}
	models = append(models, req)
	if err := s.saveCustomModels(conn.ID, models); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save model"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"customModels": models})
}

// handleDeleteProviderModel removes a custom model from a connection.
func (s *Server) handleDeleteProviderModel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	conn, err := s.DB.GetConnection(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found"})
		return
	}

	var req struct {
		ModelID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.ModelID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "model id is required"})
		return
	}

	models := s.loadCustomModels(conn.ID)
	filtered := models[:0]
	for _, m := range models {
		if m.ID != req.ModelID {
			filtered = append(filtered, m)
		}
	}
	if len(filtered) == len(models) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "model not found"})
		return
	}
	if err := s.saveCustomModels(conn.ID, filtered); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete model"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"customModels": filtered})
}
// handleSaveProviderModelMeta updates user-customized metadata for a model.
// POST /api/providers/{id}/models/meta
func (s *Server) handleSaveProviderModelMeta(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	conn, err := s.DB.GetConnection(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found"})
		return
	}

	var req struct {
		ID            string `json:"id"`
		DisplayName   string `json:"displayName"`
		ContextLength int    `json:"contextLength"`
		MaxOutput     int    `json:"maxOutputTokens"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "model id is required"})
		return
	}

	fullID := conn.Provider + "/" + req.ID
	ov := ModelMetaOverride{
		DisplayName:   req.DisplayName,
		ContextLength: req.ContextLength,
		MaxOutput:     req.MaxOutput,
	}
	data, _ := json.Marshal(ov)
	if err := s.DB.KVSet("modelMetaOverrides", fullID, string(data)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save model metadata"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleResetProviderModelMeta clears user-customized metadata for a model.
// DELETE /api/providers/{id}/models/meta
func (s *Server) handleResetProviderModelMeta(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	conn, err := s.DB.GetConnection(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found"})
		return
	}

	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "model id is required"})
		return
	}

	fullID := conn.Provider + "/" + req.ID
	s.DB.KVDelete("modelMetaOverrides", fullID)

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
