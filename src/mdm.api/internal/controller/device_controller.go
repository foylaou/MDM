package controller

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anthropics/mdm-server/internal/adapter/postgres"
	"github.com/anthropics/mdm-server/internal/middleware"
	"github.com/anthropics/mdm-server/internal/port"
)

type DeviceController struct {
	deviceRepo   *postgres.DeviceRepo
	mdmClient    port.MicroMDMClient
	auth         *middleware.AuthHelper
	depScheduler port.DEPSchedulerRunner // nil when DEP_AUTO_ASSIGN=false
	depRepo      port.DEPAssignmentRepo  // nil when DEP_AUTO_ASSIGN=false
	templateDir  string                  // path holding mac.json / ipad.json etc.
}

func NewDeviceController(deviceRepo *postgres.DeviceRepo, mdmClient port.MicroMDMClient, auth *middleware.AuthHelper, depScheduler port.DEPSchedulerRunner, depRepo port.DEPAssignmentRepo, templateDir string) *DeviceController {
	return &DeviceController{deviceRepo: deviceRepo, mdmClient: mdmClient, auth: auth, depScheduler: depScheduler, depRepo: depRepo, templateDir: templateDir}
}

func (c *DeviceController) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/devices/", c.handleDeviceByID)
	mux.HandleFunc("/api/devices-list", c.handleDevicesList)
	mux.HandleFunc("/api/devices-available", c.handleDevicesAvailable)
	mux.HandleFunc("/api/sync-device-info", c.handleSyncDeviceInfo)
	mux.HandleFunc("/api/dep/apply-now", c.handleDEPApplyNow)
	mux.HandleFunc("/api/dep/retry", c.handleDEPRetry)
	mux.HandleFunc("/api/dep/templates/", c.handleDEPTemplate)
}

// handleDEPApplyNow runs the DEP scheduler's RunOnce synchronously so an admin
// can apply profiles to brand-new ABM devices without waiting for the next
// polling tick. Requires DEP_AUTO_ASSIGN=true at boot (so the scheduler exists).
//
// @Summary 立即套用 DEP profile（觸發排程器跑一次）
// @Tags Device
// @Produce json
// @Security BearerAuth
// @Success 200 {object} swagOK
// @Failure 503 {object} swagError "DEP 排程器未啟用"
// @Router /api/dep/apply-now [post]
func (c *DeviceController) handleDEPApplyNow(w http.ResponseWriter, r *http.Request) {
	if _, err := c.auth.RequireSysAdmin(r); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if c.depScheduler == nil {
		writeError(w, http.StatusServiceUnavailable, "DEP 自動派發未啟用（DEP_AUTO_ASSIGN=false 或 ABM 設定不完整）")
		return
	}
	res := c.depScheduler.RunOnce(r.Context())
	log.Printf("[dep-apply-now] triggered by admin: applied=%d skipped=%d errors=%d already_known=%d abm_total=%d",
		res.Applied, res.Skipped, res.Errors, res.AlreadyKnown, res.ABMTotal)
	writeJSON(w, map[string]interface{}{
		"ok":            true,
		"applied":       res.Applied,
		"skipped":       res.Skipped,
		"errors":        res.Errors,
		"already_known": res.AlreadyKnown,
		"abm_total":     res.ABMTotal,
	})
}

// handleDEPRetry clears a serial from dep_assignments so the scheduler will
// re-apply the profile on the next cycle (or the next apply-now call).
func (c *DeviceController) handleDEPRetry(w http.ResponseWriter, r *http.Request) {
	if _, err := c.auth.RequireSysAdmin(r); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if c.depRepo == nil {
		writeError(w, http.StatusServiceUnavailable, "DEP 未啟用")
		return
	}
	var body struct {
		Serial string `json:"serial"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Serial == "" {
		writeError(w, http.StatusBadRequest, "serial required")
		return
	}
	if err := c.depRepo.Delete(r.Context(), body.Serial); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	log.Printf("[dep-retry] cleared serial=%s from dep_assignments by admin", body.Serial)
	// Run a cycle immediately so the device gets processed right now.
	if c.depScheduler != nil {
		res := c.depScheduler.RunOnce(r.Context())
		writeJSON(w, map[string]interface{}{
			"ok":            true,
			"serial":        body.Serial,
			"applied":       res.Applied,
			"skipped":       res.Skipped,
			"errors":        res.Errors,
			"already_known": res.AlreadyKnown,
		})
		return
	}
	writeOK(w)
}

// handleDEPTemplate GET/PUT /api/dep/templates/:family
// family = mac | ipad | iphone | appletv
// Returns the raw JSON content of the template file, or writes new content.
// Requires sys_admin. templateDir="" → 503.
func (c *DeviceController) handleDEPTemplate(w http.ResponseWriter, r *http.Request) {
	if _, err := c.auth.RequireSysAdmin(r); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if c.templateDir == "" {
		writeError(w, http.StatusServiceUnavailable, "DEP 模板目錄未設定（DEP_TEMPLATE_DIR）")
		return
	}

	family := strings.TrimPrefix(r.URL.Path, "/api/dep/templates/")
	family = strings.ToLower(strings.TrimSuffix(family, ".json"))
	switch family {
	case "mac", "ipad", "iphone", "appletv":
	default:
		writeError(w, http.StatusBadRequest, fmt.Sprintf("未知裝置類型 %q，應為 mac / ipad / iphone / appletv", family))
		return
	}

	path := filepath.Join(c.templateDir, family+".json")

	switch r.Method {
	case http.MethodGet:
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			// Return empty template so the UI can prefill.
			writeJSON(w, map[string]interface{}{
				"exists":  false,
				"content": defaultDEPTemplate(family),
			})
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		// Validate it's valid JSON before echoing.
		var check map[string]interface{}
		if err := json.Unmarshal(data, &check); err != nil {
			writeError(w, http.StatusInternalServerError, "模板檔案 JSON 解析失敗："+err.Error())
			return
		}
		writeJSON(w, map[string]interface{}{"exists": true, "content": string(data)})

	case http.MethodPut:
		var body struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		// Validate JSON before saving.
		var check map[string]interface{}
		if err := json.Unmarshal([]byte(body.Content), &check); err != nil {
			writeError(w, http.StatusBadRequest, "JSON 格式錯誤："+err.Error())
			return
		}
		if err := os.MkdirAll(c.templateDir, 0o755); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := os.WriteFile(path, []byte(body.Content), 0o644); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		log.Printf("[dep-template] saved %s", path)
		writeOK(w)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// defaultDEPTemplate returns a minimal DEP profile skeleton for each device family.
func defaultDEPTemplate(_ string) string {
	return `{
  "url": "https://YOUR_MDM_SERVER/mdm",
  "allow_pairing": true,
  "is_supervised": true,
  "is_mandatory": true,
  "await_device_configured": false,
  "department": "",
  "skip_setup_items": [
    "AppleID",
    "Biometric",
    "Diagnostics",
    "DisplayTone",
    "Location",
    "Passcode",
    "Payment",
    "Privacy",
    "Restore",
    "ScreenTime",
    "Siri",
    "TOS",
    "iMessageAndFaceTime"
  ]
}`
}

// handleDeviceByID godoc
// @Summary 依 UDID 取得裝置詳細資訊
// @Tags Device
// @Produce json
// @Security BearerAuth
// @Param udid path string true "裝置 UDID"
// @Success 200 {object} swagDevice
// @Failure 404 {object} swagError
// @Router /api/devices/{udid} [get]
func (c *DeviceController) handleDeviceByID(w http.ResponseWriter, r *http.Request) {
	if _, err := c.auth.RequireModule(r, "mdm", "viewer"); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	udid := strings.TrimPrefix(r.URL.Path, "/api/devices/")
	if udid == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	d, err := c.deviceRepo.GetByUDID(r.Context(), udid)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, map[string]interface{}{
		"udid":              d.UDID,
		"serial_number":     d.SerialNumber,
		"device_name":       d.DeviceName,
		"model":             d.Model,
		"os_version":        d.OSVersion,
		"last_seen":         d.LastSeen.Format(time.RFC3339),
		"enrollment_status": d.EnrollmentStatus,
		"is_supervised":     d.IsSupervised,
		"is_lost_mode":      d.IsLostMode,
		"battery_level":     d.BatteryLevel,
		"details":           d.Details,
	})
}

// handleDevicesList godoc
// @Summary 裝置列表（含資產與借用狀態）
// @Tags Device
// @Produce json
// @Security BearerAuth
// @Param filter query string false "關鍵字篩選"
// @Param category_id query string false "分類 ID"
// @Param custodian_id query string false "保管人 ID"
// @Param rental_status query string false "借用狀態"
// @Success 200 {object} swagDeviceListResp
// @Router /api/devices-list [get]
func (c *DeviceController) handleDevicesList(w http.ResponseWriter, r *http.Request) {
	claims, err := c.auth.RequireModule(r, "mdm", "viewer")
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	filter := r.URL.Query().Get("filter")
	category := r.URL.Query().Get("category_id")
	custodian := r.URL.Query().Get("custodian_id")
	rentalStatus := r.URL.Query().Get("rental_status")

	var viewerUserID string
	isSysAdmin := claims.SystemRole == "sys_admin" || claims.Role == "admin"
	if claims.Role == "viewer" && !isSysAdmin {
		viewerUserID = claims.UserID
	}

	devices, err := c.deviceRepo.ListWithAssets(r.Context(), filter, category, custodian, rentalStatus, viewerUserID)
	if err != nil {
		log.Printf("devices-list: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	type deviceRow struct {
		UDID              string  `json:"udid"`
		SerialNumber      string  `json:"serial_number"`
		DeviceName        string  `json:"device_name"`
		AssetNumber       string  `json:"asset_number"`
		Model             string  `json:"model"`
		OSVersion         string  `json:"os_version"`
		LastSeen          string  `json:"last_seen"`
		EnrollmentStatus  string  `json:"enrollment_status"`
		IsSupervised      bool    `json:"is_supervised"`
		IsLostMode        bool    `json:"is_lost_mode"`
		BatteryLevel      float64 `json:"battery_level"`
		CustodianName     string  `json:"custodian_name"`
		CurrentHolderName string  `json:"current_holder_name"`
		CategoryName      string  `json:"category_name"`
		CategoryID        *string `json:"category_id"`
		CustodianID       *string `json:"custodian_id"`
		AssetStatus       string  `json:"asset_status"`
	}

	rows := make([]deviceRow, 0, len(devices))
	for _, d := range devices {
		rows = append(rows, deviceRow{
			UDID: d.UDID, SerialNumber: d.SerialNumber, DeviceName: d.DeviceName,
			AssetNumber: d.AssetNumber,
			Model:       d.Model, OSVersion: d.OSVersion, LastSeen: d.LastSeen.Format(time.RFC3339),
			EnrollmentStatus: d.EnrollmentStatus, IsSupervised: d.IsSupervised,
			IsLostMode: d.IsLostMode, BatteryLevel: d.BatteryLevel,
			CustodianName: d.CustodianName, CurrentHolderName: d.CurrentHolderName,
			CategoryName: d.CategoryName,
			CategoryID:   d.CategoryID, CustodianID: d.CustodianID, AssetStatus: d.AssetStatus,
		})
	}
	writeJSON(w, map[string]interface{}{"devices": rows, "total": len(rows)})
}

// handleDevicesAvailable godoc
// @Summary 可借用裝置列表
// @Tags Device
// @Produce json
// @Security BearerAuth
// @Success 200 {object} swagDeviceAvailResp
// @Router /api/devices-available [get]
func (c *DeviceController) handleDevicesAvailable(w http.ResponseWriter, r *http.Request) {
	if _, err := c.auth.RequireAuth(r); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	devices, err := c.deviceRepo.ListAvailable(r.Context())
	if err != nil {
		log.Printf("devices-available: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	type deviceRow struct {
		UDID             string  `json:"udid"`
		SerialNumber     string  `json:"serial_number"`
		DeviceName       string  `json:"device_name"`
		Model            string  `json:"model"`
		OSVersion        string  `json:"os_version"`
		EnrollmentStatus string  `json:"enrollment_status"`
		AssetStatus      string  `json:"asset_status"`
		CategoryID       *string `json:"category_id"`
		CategoryName     string  `json:"category_name"`
	}
	rows := make([]deviceRow, 0, len(devices))
	for _, d := range devices {
		rows = append(rows, deviceRow{
			UDID: d.UDID, SerialNumber: d.SerialNumber, DeviceName: d.DeviceName,
			Model: d.Model, OSVersion: d.OSVersion,
			EnrollmentStatus: d.EnrollmentStatus, AssetStatus: d.AssetStatus,
			CategoryID: d.CategoryID, CategoryName: d.CategoryName,
		})
	}
	writeJSON(w, map[string]interface{}{"devices": rows})
}

// handleSyncDeviceInfo godoc
// @Summary 向所有裝置發送 DeviceInformation 查詢
// @Tags Device
// @Produce json
// @Security BearerAuth
// @Success 200 {object} swagSyncCountResp
// @Failure 401 {string} string "Unauthorized"
// @Router /api/sync-device-info [post]
func (c *DeviceController) handleSyncDeviceInfo(w http.ResponseWriter, r *http.Request) {
	if _, err := c.auth.RequireModule(r, "mdm", "manager"); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	devices, _, err := c.deviceRepo.List(r.Context(), "", 500, 0)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	queries := []string{
		"UDID", "DeviceName", "OSVersion", "BuildVersion",
		"ModelName", "Model", "ProductName", "SerialNumber",
		"DeviceCapacity", "AvailableDeviceCapacity", "BatteryLevel",
		"IsSupervised", "IsActivationLockEnabled", "IsMDMLostModeEnabled",
		"WiFiMAC", "BluetoothMAC",
	}
	count := 0
	for _, d := range devices {
		payload := map[string]interface{}{
			"udid": d.UDID, "request_type": "DeviceInformation", "queries": queries,
		}
		if _, err := c.mdmClient.SendCommand(r.Context(), payload); err != nil {
			continue
		}
		_ = c.mdmClient.SendPush(r.Context(), d.UDID)
		count++
	}
	writeJSON(w, map[string]interface{}{"count": count})
	log.Printf("[sync-info] sent DeviceInformation to %d devices", count)
}
