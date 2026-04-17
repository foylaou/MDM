package controller

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/anthropics/mdm-server/internal/domain"
	"github.com/anthropics/mdm-server/internal/middleware"
	"github.com/anthropics/mdm-server/internal/port"
)

type InventoryController struct {
	inventoryRepo port.InventoryRepository
	auditRepo     port.AuditRepository
	auth          *middleware.AuthHelper
}

func NewInventoryController(inventoryRepo port.InventoryRepository, auditRepo port.AuditRepository, auth *middleware.AuthHelper) *InventoryController {
	return &InventoryController{inventoryRepo: inventoryRepo, auditRepo: auditRepo, auth: auth}
}

func (c *InventoryController) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/inventory-sessions", c.handleSessions)
	mux.HandleFunc("/api/inventory-sessions/", c.handleSessionByID)
	mux.HandleFunc("/api/inventory-items/", c.handleItemByID)
	mux.HandleFunc("/api/inventory-export/", c.handleExport)
}

func sessionToJSON(s *domain.InventorySession) map[string]interface{} {
	row := map[string]interface{}{
		"id": s.ID, "name": s.Name, "description": s.Description,
		"status": s.Status, "created_by": s.CreatedBy, "creator_name": s.CreatorName,
		"created_at": s.CreatedAt.Format(time.RFC3339), "notes": s.Notes,
		"total_count": s.TotalCount, "checked_count": s.CheckedCount,
		"matched_count": s.MatchedCount, "missing_count": s.MissingCount,
	}
	if s.StartedAt != nil {
		row["started_at"] = s.StartedAt.Format(time.RFC3339)
	} else {
		row["started_at"] = nil
	}
	if s.CompletedAt != nil {
		row["completed_at"] = s.CompletedAt.Format(time.RFC3339)
	} else {
		row["completed_at"] = nil
	}
	return row
}

func itemToJSON(item *domain.InventoryItem) map[string]interface{} {
	row := map[string]interface{}{
		"id": item.ID, "session_id": item.SessionID, "asset_id": item.AssetID,
		"device_udid": item.DeviceUdid, "asset_number": item.AssetNumber, "asset_name": item.AssetName,
		"found": item.Found, "condition": item.Condition,
		"checked_by": item.CheckedBy, "checker_name": item.CheckerName, "notes": item.Notes,
	}
	if item.CheckedAt != nil {
		row["checked_at"] = item.CheckedAt.Format(time.RFC3339)
	} else {
		row["checked_at"] = nil
	}
	return row
}

// handleSessions — GET: list sessions, POST: create session
func (c *InventoryController) handleSessions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		if _, err := c.auth.RequireModule(r, "asset", "viewer"); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		sessions, err := c.inventoryRepo.ListSessions(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		rows := make([]map[string]interface{}, 0, len(sessions))
		for _, s := range sessions {
			rows = append(rows, sessionToJSON(s))
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"sessions": rows})

	case http.MethodPost:
		claims, err := c.auth.RequireModule(r, "asset", "manager")
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Notes       string `json:"notes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
			writeError(w, http.StatusBadRequest, "name required")
			return
		}
		session := &domain.InventorySession{
			Name:        body.Name,
			Description: body.Description,
			CreatedBy:   claims.UserID,
			CreatorName: claims.Username,
			Notes:       body.Notes,
		}
		id, err := c.inventoryRepo.CreateSession(r.Context(), session)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		// Auto-generate items from current assets
		count, _ := c.inventoryRepo.GenerateItems(r.Context(), id)

		c.auditRepo.Create(r.Context(), &domain.AuditLog{
			UserID: claims.UserID, Username: claims.Username,
			Action: "inventory_create", Target: id,
			Detail: fmt.Sprintf("name=%s, items=%d", body.Name, count), Module: "asset",
			IPAddress: clientIP(r), UserAgent: r.UserAgent(),
		})
		json.NewEncoder(w).Encode(map[string]interface{}{"id": id, "item_count": count})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// handleSessionByID — GET: session detail + items, PUT: update status/notes, DELETE: delete session
// Also handles: POST /api/inventory-sessions/{id}/start, /complete, /generate
func (c *InventoryController) handleSessionByID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	path := strings.TrimPrefix(r.URL.Path, "/api/inventory-sessions/")
	parts := strings.SplitN(path, "/", 2)
	id := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		if _, err := c.auth.RequireModule(r, "asset", "viewer"); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		session, err := c.inventoryRepo.GetSession(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusNotFound, "session not found")
			return
		}
		items, _ := c.inventoryRepo.ListItems(r.Context(), id)
		itemRows := make([]map[string]interface{}, 0, len(items))
		for _, item := range items {
			itemRows = append(itemRows, itemToJSON(item))
		}
		result := sessionToJSON(session)
		result["items"] = itemRows
		json.NewEncoder(w).Encode(result)

	case http.MethodPost:
		claims, err := c.auth.RequireModule(r, "asset", "manager")
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch action {
		case "start":
			if err := c.inventoryRepo.UpdateSessionStatus(r.Context(), id, "in_progress"); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			c.auditRepo.Create(r.Context(), &domain.AuditLog{
				UserID: claims.UserID, Username: claims.Username,
				Action: "inventory_start", Target: id, Module: "asset",
				IPAddress: clientIP(r), UserAgent: r.UserAgent(),
			})
			writeOK(w)

		case "complete":
			if err := c.inventoryRepo.UpdateSessionStatus(r.Context(), id, "completed"); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			c.auditRepo.Create(r.Context(), &domain.AuditLog{
				UserID: claims.UserID, Username: claims.Username,
				Action: "inventory_complete", Target: id, Module: "asset",
				IPAddress: clientIP(r), UserAgent: r.UserAgent(),
			})
			writeOK(w)

		case "generate":
			count, err := c.inventoryRepo.GenerateItems(r.Context(), id)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "item_count": count})

		default:
			writeError(w, http.StatusBadRequest, "unknown action: "+action)
		}

	case http.MethodPut:
		if _, err := c.auth.RequireModule(r, "asset", "manager"); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body struct {
			Notes string `json:"notes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if err := c.inventoryRepo.UpdateSessionNotes(r.Context(), id, body.Notes); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeOK(w)

	case http.MethodDelete:
		if _, err := c.auth.RequireModule(r, "asset", "manager"); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if err := c.inventoryRepo.DeleteSession(r.Context(), id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeOK(w)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// handleItemByID — PUT: check an inventory item (found/missing + condition)
func (c *InventoryController) handleItemByID(w http.ResponseWriter, r *http.Request) {
	claims, err := c.auth.RequireModule(r, "asset", "operator")
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if !requireMethod(w, r, http.MethodPut) {
		return
	}
	w.Header().Set("Content-Type", "application/json")

	id := strings.TrimPrefix(r.URL.Path, "/api/inventory-items/")
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var body struct {
		Found     bool   `json:"found"`
		Condition string `json:"condition"`
		Notes     string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := c.inventoryRepo.CheckItem(r.Context(), id, body.Found, body.Condition, claims.UserID, claims.Username, body.Notes); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w)
}

// handleExport exports an inventory session report as Excel.
func (c *InventoryController) handleExport(w http.ResponseWriter, r *http.Request) {
	if _, err := c.auth.RequireModule(r, "asset", "viewer"); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	sessionID := strings.TrimPrefix(r.URL.Path, "/api/inventory-export/")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session id required")
		return
	}

	session, err := c.inventoryRepo.GetSession(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	items, err := c.inventoryRepo.ListItems(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	statusLabels := map[string]string{
		"draft": "草稿", "in_progress": "盤點中", "completed": "已完成",
	}

	f := excelize.NewFile()
	sheet := "盤點報告"
	f.SetSheetName("Sheet1", sheet)

	// Header info
	f.SetCellValue(sheet, "A1", "盤點名稱")
	f.SetCellValue(sheet, "B1", session.Name)
	f.SetCellValue(sheet, "A2", "狀態")
	sl := statusLabels[session.Status]
	if sl == "" {
		sl = session.Status
	}
	f.SetCellValue(sheet, "B2", sl)
	f.SetCellValue(sheet, "A3", "建立者")
	f.SetCellValue(sheet, "B3", session.CreatorName)
	f.SetCellValue(sheet, "A4", "建立時間")
	f.SetCellValue(sheet, "B4", session.CreatedAt.Format("2006-01-02 15:04"))
	f.SetCellValue(sheet, "C1", "應盤")
	f.SetCellValue(sheet, "D1", session.TotalCount)
	f.SetCellValue(sheet, "C2", "已盤")
	f.SetCellValue(sheet, "D2", session.CheckedCount)
	f.SetCellValue(sheet, "C3", "吻合")
	f.SetCellValue(sheet, "D3", session.MatchedCount)
	f.SetCellValue(sheet, "C4", "短缺")
	f.SetCellValue(sheet, "D4", session.MissingCount)

	boldStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	f.SetCellStyle(sheet, "A1", "A4", boldStyle)
	f.SetCellStyle(sheet, "C1", "C4", boldStyle)

	// Item table
	tableHeaders := []string{"財產編號", "名稱", "裝置 UDID", "盤點結果", "狀況", "盤點人", "盤點時間", "備註"}
	startRow := 6
	for col, h := range tableHeaders {
		cell, _ := excelize.CoordinatesToCellName(col+1, startRow)
		f.SetCellValue(sheet, cell, h)
	}
	endCell, _ := excelize.CoordinatesToCellName(len(tableHeaders), startRow)
	f.SetCellStyle(sheet, "A6", endCell, boldStyle)

	for i, item := range items {
		row := startRow + 1 + i
		foundStr := "未盤"
		if item.Found != nil {
			if *item.Found {
				foundStr = "在庫"
			} else {
				foundStr = "短缺"
			}
		}
		checkedAt := ""
		if item.CheckedAt != nil {
			checkedAt = item.CheckedAt.Format("2006-01-02 15:04")
		}
		vals := []interface{}{
			item.AssetNumber, item.AssetName, item.DeviceUdid,
			foundStr, item.Condition, item.CheckerName, checkedAt, item.Notes,
		}
		for col, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(col+1, row)
			f.SetCellValue(sheet, cell, v)
		}
	}

	for col := range tableHeaders {
		colName, _ := excelize.ColumnNumberToName(col + 1)
		f.SetColWidth(sheet, colName, colName, 16)
	}

	now := time.Now().Format("2006-01-02")
	filename := fmt.Sprintf("盤點報告_%s_%s.xlsx", session.Name, now)

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	if err := f.Write(w); err != nil {
		log.Printf("[inventory-export] write error: %v", err)
	}
	f.Close()
}
