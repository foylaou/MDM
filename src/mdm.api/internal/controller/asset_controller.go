package controller

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/mdm-server/internal/domain"
	"github.com/anthropics/mdm-server/internal/middleware"
	"github.com/anthropics/mdm-server/internal/port"
)

type AssetController struct {
	assetRepo port.AssetRepository
	auth      *middleware.AuthHelper
}

func NewAssetController(assetRepo port.AssetRepository, auth *middleware.AuthHelper) *AssetController {
	return &AssetController{assetRepo: assetRepo, auth: auth}
}

func (c *AssetController) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/assets", c.handleAssets)
	mux.HandleFunc("/api/assets/", c.handleAssetByID)
	mux.HandleFunc("/api/device-status", c.handleDeviceStatus)
}

// handleAssets godoc
// @Summary 資產列表 / 新增資產
// @Tags Asset
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param device_udid query string false "依裝置 UDID 篩選" Example(devices-list)
// @Param body body swagAssetReq false "新增資產（POST）"
// @Success 200 {object} map[string]interface{} "GET: {assets: [...]}, POST: {id: ...}"
// @Failure 400 {object} swagError
// @Router /api/assets [get]
// @Router /api/assets [post]
func (c *AssetController) handleAssets(w http.ResponseWriter, r *http.Request) {
	if _, err := c.auth.RequireModule(r, "asset", "viewer"); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		deviceUdid := r.URL.Query().Get("device_udid")
		assets, err := c.assetRepo.List(r.Context(), deviceUdid)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		rows := make([]map[string]interface{}, 0, len(assets))
		for _, a := range assets {
			row := map[string]interface{}{
				"id": a.ID, "device_udid": a.DeviceUdid, "asset_number": a.AssetNumber,
				"name": a.Name, "spec": a.Spec, "quantity": a.Quantity, "unit": a.Unit,
				"unit_price": a.UnitPrice, "purpose": a.Purpose,
				"custodian_id": a.CustodianID, "custodian_name": a.CustodianName,
				"location": a.Location, "asset_category": a.AssetCategory, "notes": a.Notes,
				"created_at": a.CreatedAt.Format(time.RFC3339), "updated_at": a.UpdatedAt.Format(time.RFC3339),
				"device_name": a.DeviceName, "device_serial": a.DeviceSerial,
				"category_id": a.CategoryID, "category_name": a.CategoryName, "asset_status": a.AssetStatus,
			}
			if a.AcquiredDate != nil {
				s := a.AcquiredDate.Format("2006-01-02")
				row["acquired_date"] = s
			} else {
				row["acquired_date"] = nil
			}
			if a.BorrowDate != nil {
				s := a.BorrowDate.Format("2006-01-02")
				row["borrow_date"] = s
			} else {
				row["borrow_date"] = nil
			}
			rows = append(rows, row)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"assets": rows})

	case http.MethodPost:
		var body struct {
			DeviceUdid    *string  `json:"device_udid"`
			AssetNumber   string   `json:"asset_number"`
			Name          string   `json:"name"`
			Spec          string   `json:"spec"`
			Quantity      int      `json:"quantity"`
			Unit          string   `json:"unit"`
			AcquiredDate  *string  `json:"acquired_date"`
			UnitPrice     float64  `json:"unit_price"`
			Purpose       string   `json:"purpose"`
			BorrowDate    *string  `json:"borrow_date"`
			CustodianID   *string  `json:"custodian_id"`
			CustodianName string   `json:"custodian_name"`
			Location      string   `json:"location"`
			AssetCategory string   `json:"asset_category"`
			Notes         string   `json:"notes"`
			CategoryID    *string  `json:"category_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		asset := &domain.Asset{
			DeviceUdid: body.DeviceUdid, AssetNumber: body.AssetNumber,
			Name: body.Name, Spec: body.Spec, Quantity: body.Quantity, Unit: body.Unit,
			UnitPrice: body.UnitPrice, Purpose: body.Purpose,
			CustodianID: body.CustodianID, CustodianName: body.CustodianName,
			Location: body.Location, AssetCategory: body.AssetCategory, Notes: body.Notes,
			CategoryID: body.CategoryID,
		}
		// Parse dates - pass as string pointers for the repo to handle
		id, err := c.assetRepo.Create(r.Context(), asset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"id": id})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// handleAssetByID godoc
// @Summary 更新 / 刪除資產
// @Tags Asset
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "資產 ID"
// @Param body body map[string]interface{} false "更新欄位（PUT）"
// @Success 200 {object} swagOK
// @Router /api/assets/{id} [put]
// @Router /api/assets/{id} [delete]
func (c *AssetController) handleAssetByID(w http.ResponseWriter, r *http.Request) {
	if _, err := c.auth.RequireModule(r, "asset", "operator"); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/assets/")
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodPut:
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if err := c.assetRepo.Update(r.Context(), id, body); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeOK(w)

	case http.MethodDelete:
		if err := c.assetRepo.Delete(r.Context(), id); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		writeOK(w)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// handleDeviceStatus godoc
// @Summary 更新裝置資產狀態
// @Tags Asset
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body swagDeviceStatusReq true "UDID + 狀態"
// @Success 200 {object} swagOK
// @Failure 400 {object} swagError
// @Router /api/device-status [put]
func (c *AssetController) handleDeviceStatus(w http.ResponseWriter, r *http.Request) {
	if _, err := c.auth.RequireModule(r, "asset", "operator"); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if !requireMethod(w, r, http.MethodPut) {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	var body struct {
		UDID   string `json:"udid"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.UDID == "" || body.Status == "" {
		writeError(w, http.StatusBadRequest, "udid and status required")
		return
	}
	valid := map[string]bool{"available": true, "faulty": true, "repairing": true, "retired": true}
	if !valid[body.Status] {
		writeError(w, http.StatusBadRequest, "invalid status")
		return
	}
	if err := c.assetRepo.UpdateStatus(r.Context(), body.UDID, body.Status); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w)
}
