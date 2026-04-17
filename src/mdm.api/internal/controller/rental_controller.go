package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/anthropics/mdm-server/internal/adapter/postgres"
	"github.com/anthropics/mdm-server/internal/domain"
	"github.com/anthropics/mdm-server/internal/middleware"
	"github.com/anthropics/mdm-server/internal/port"
	"github.com/anthropics/mdm-server/internal/service"
)

type RentalController struct {
	rentalRepo *postgres.RentalRepo
	assetRepo  *postgres.AssetRepo
	userRepo   port.UserRepository
	notifySvc  *service.NotifyService
	auth       *middleware.AuthHelper
}

func NewRentalController(rentalRepo *postgres.RentalRepo, assetRepo *postgres.AssetRepo, userRepo port.UserRepository, notifySvc *service.NotifyService, auth *middleware.AuthHelper) *RentalController {
	return &RentalController{rentalRepo: rentalRepo, assetRepo: assetRepo, userRepo: userRepo, notifySvc: notifySvc, auth: auth}
}

// buildNotifyData gathers device names and common fields for notification emails.
func (c *RentalController) buildNotifyData(ctx context.Context, rentalNumber int, borrowerName, approverName, purpose, notes string, expectedReturn *time.Time) service.RentalNotifyData {
	data := service.RentalNotifyData{
		RentalNumber: rentalNumber,
		BorrowerName: borrowerName,
		ApproverName: approverName,
		Purpose:      purpose,
		Notes:        notes,
	}
	if expectedReturn != nil {
		data.ExpectedReturn = expectedReturn.Format("2006-01-02")
	}
	// Gather device names from all rentals with this number
	rentals, _ := c.rentalRepo.List(ctx, "", "", false)
	for _, rl := range rentals {
		if rl.RentalNumber == rentalNumber {
			name := rl.DeviceName
			if name == "" {
				name = rl.DeviceSerial
			}
			if name == "" {
				name = rl.DeviceUdid
			}
			data.DeviceNames = append(data.DeviceNames, name)
		}
	}
	return data
}

func (c *RentalController) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/rentals", c.handleRentals)
	mux.HandleFunc("/api/rentals-export", c.handleExport)
	mux.HandleFunc("/api/rentals/", c.handleRentalByID)
	mux.HandleFunc("/api/rentals-archive", c.handleArchive)
}

// handleRentals godoc
// @Summary 借用單列表 / 建立借用單
// @Tags Rental
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param status query string false "狀態篩選" Enums(pending,approved,active,returned,rejected)
// @Param device_udid query string false "裝置 UDID"
// @Param show_archived query string false "顯示已歸檔" Enums(true,false)
// @Param body body swagRentalReq false "建立借用單（POST）"
// @Success 200 {object} map[string]interface{} "GET: {rentals: [...]}, POST: {ids, count, rental_number}"
// @Failure 409 {object} swagError "裝置不可用"
// @Router /api/rentals [get]
// @Router /api/rentals [post]
func (c *RentalController) handleRentals(w http.ResponseWriter, r *http.Request) {
	claims, err := c.auth.RequireModule(r, "rental", "requester")
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		status := r.URL.Query().Get("status")
		deviceUdid := r.URL.Query().Get("device_udid")
		showArchived := r.URL.Query().Get("show_archived") == "true"

		rentals, err := c.rentalRepo.List(r.Context(), status, deviceUdid, showArchived)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		rows := make([]map[string]interface{}, 0, len(rentals))
		for _, rl := range rentals {
			row := map[string]interface{}{
				"id": rl.ID, "device_udid": rl.DeviceUdid,
				"borrower_id": rl.BorrowerID, "borrower_name": rl.BorrowerName,
				"approver_id": rl.ApproverID, "approver_name": rl.ApproverName,
				"status": rl.Status, "purpose": rl.Purpose,
				"borrow_date": rl.BorrowDate.Format(time.RFC3339), "notes": rl.Notes,
				"created_at": rl.CreatedAt.Format(time.RFC3339), "updated_at": rl.UpdatedAt.Format(time.RFC3339),
				"device_name": rl.DeviceName, "device_serial": rl.DeviceSerial,
				"custodian_id": rl.CustodianID, "custodian_name": rl.CustodianName,
				"rental_number": rl.RentalNumber, "is_archived": rl.IsArchived,
				"return_checklist": rl.ReturnChecklist, "return_notes": rl.ReturnNotes,
			}
			if rl.ExpectedReturn != nil {
				row["expected_return"] = rl.ExpectedReturn.Format("2006-01-02")
			} else {
				row["expected_return"] = nil
			}
			if rl.ActualReturn != nil {
				row["actual_return"] = rl.ActualReturn.Format(time.RFC3339)
			} else {
				row["actual_return"] = nil
			}
			rows = append(rows, row)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"rentals": rows})

	case http.MethodPost:
		var body struct {
			DeviceUdids    []string `json:"device_udids"`
			BorrowerID     string   `json:"borrower_id"`
			Purpose        string   `json:"purpose"`
			ExpectedReturn *string  `json:"expected_return"`
			Notes          string   `json:"notes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.BorrowerID == "" || len(body.DeviceUdids) == 0 {
			writeError(w, http.StatusBadRequest, "device_udids and borrower_id required")
			return
		}

		// Get borrower name
		borrower, err := c.userRepo.GetByID(r.Context(), body.BorrowerID)
		borrowerName := ""
		if err == nil {
			borrowerName = borrower.DisplayName
			if borrowerName == "" {
				borrowerName = borrower.Username
			}
		}

		// Check availability
		var unavailable []string
		for _, udid := range body.DeviceUdids {
			assetStatus, isRented, isLostMode, _ := c.rentalRepo.CheckDeviceAvailability(r.Context(), udid)
			if isRented {
				unavailable = append(unavailable, udid+" (借出中)")
			} else if isLostMode {
				unavailable = append(unavailable, udid+" (遺失)")
			} else if assetStatus != "available" && assetStatus != "" {
				unavailable = append(unavailable, udid+" ("+assetStatus+")")
			}
		}
		if len(unavailable) > 0 {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":   "部分裝置目前無法借出",
				"devices": unavailable,
			})
			return
		}

		rentalNumber, _ := c.rentalRepo.NextRentalNumber(r.Context())

		var expectedReturn *time.Time
		if body.ExpectedReturn != nil && *body.ExpectedReturn != "" {
			t, err := time.Parse("2006-01-02", *body.ExpectedReturn)
			if err == nil {
				expectedReturn = &t
			}
		}

		var ids []string
		for _, udid := range body.DeviceUdids {
			rental := &domain.Rental{
				DeviceUdid:     udid,
				BorrowerID:     body.BorrowerID,
				BorrowerName:   borrowerName,
				Purpose:        body.Purpose,
				ExpectedReturn: expectedReturn,
				Notes:          body.Notes,
				RentalNumber:   rentalNumber,
			}
			id, err := c.rentalRepo.Create(r.Context(), rental)
			if err != nil {
				log.Printf("rental insert: %v", err)
				continue
			}
			ids = append(ids, id)
		}
		// Notify custodians/approvers about the new rental request
		go func() {
			bgCtx := context.Background()
			data := c.buildNotifyData(bgCtx, rentalNumber, borrowerName, "", body.Purpose, body.Notes, expectedReturn)
			// Collect unique custodian emails for the rented devices
			notified := map[string]bool{}
			for _, udid := range body.DeviceUdids {
				assets, _ := c.assetRepo.List(bgCtx, udid)
				for _, a := range assets {
					if a.CustodianID != nil && *a.CustodianID != "" {
						custodian, err := c.userRepo.GetByID(bgCtx, *a.CustodianID)
						if err == nil && custodian.Email != "" && !notified[custodian.Email] {
							c.notifySvc.SendRentalRequest(bgCtx, data, custodian.Email)
							notified[custodian.Email] = true
						}
					}
				}
			}
		}()
		_ = claims
		json.NewEncoder(w).Encode(map[string]interface{}{"ids": ids, "count": len(ids), "rental_number": rentalNumber})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// handleRentalByID godoc
// @Summary 借用單操作：approve / activate / return / reject / delete
// @Tags Rental
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "借用單 ID"
// @Param action path string false "操作" Enums(approve,activate,return,reject)
// @Param body body swagReturnReq false "歸還資訊（return 時使用）"
// @Success 200 {object} swagOK
// @Failure 400 {object} swagError
// @Failure 404 {object} swagError
// @Router /api/rentals/{id}/{action} [post]
// @Router /api/rentals/{id} [delete]
func (c *RentalController) handleRentalByID(w http.ResponseWriter, r *http.Request) {
	claims, err := c.auth.RequireModule(r, "rental", "requester")
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/rentals/"), "/")
	id := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	if r.Method == http.MethodPost && action != "" {
		// Get approver display name
		approver, _ := c.userRepo.GetByID(r.Context(), claims.UserID)
		approverDisplayName := claims.Username
		if approver != nil {
			if approver.DisplayName != "" {
				approverDisplayName = approver.DisplayName
			}
		}

		rental, err := c.rentalRepo.GetByID(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusNotFound, "rental not found")
			return
		}

		switch action {
		case "approve":
			if rental.Status != "pending" {
				writeError(w, http.StatusBadRequest, "rental is not pending")
				return
			}
			c.rentalRepo.UpdateStatusByNumber(r.Context(), rental.RentalNumber, "pending", "approved", &claims.UserID, approverDisplayName)
			// Notify borrower that rental is approved
			go func() {
				bgCtx := context.Background()
				borrowerID, borrowerName, _ := c.rentalRepo.GetBorrowerInfo(bgCtx, id)
				data := c.buildNotifyData(bgCtx, rental.RentalNumber, borrowerName, approverDisplayName, "", "", nil)
				borrower, err := c.userRepo.GetByID(bgCtx, borrowerID)
				if err == nil && borrower.Email != "" {
					c.notifySvc.SendRentalApproved(bgCtx, data, borrower.Email)
				}
			}()
			writeJSON(w, map[string]interface{}{"ok": true, "status": "approved"})

		case "activate":
			if rental.Status != "approved" {
				writeError(w, http.StatusBadRequest, "rental is not approved")
				return
			}
			c.rentalRepo.ActivateByNumber(r.Context(), rental.RentalNumber)
			borrowerID, borrowerName, _ := c.rentalRepo.GetBorrowerInfo(r.Context(), id)
			udids, _ := c.rentalRepo.ListDeviceUdidsByNumber(r.Context(), rental.RentalNumber)
			for _, udid := range udids {
				c.assetRepo.SetHolderByUdid(r.Context(), udid, borrowerID, borrowerName)
			}
			// Notify borrower that devices are handed out
			go func() {
				bgCtx := context.Background()
				data := c.buildNotifyData(bgCtx, rental.RentalNumber, borrowerName, approverDisplayName, "", "", nil)
				borrower, err := c.userRepo.GetByID(bgCtx, borrowerID)
				if err == nil && borrower.Email != "" {
					c.notifySvc.SendRentalActivated(bgCtx, data, borrower.Email)
				}
			}()
			log.Printf("[rental] batch activated: rental_number=%d borrower=%s", rental.RentalNumber, borrowerName)
			writeJSON(w, map[string]interface{}{"ok": true, "status": "active"})

		case "return":
			if rental.Status != "active" {
				writeError(w, http.StatusBadRequest, "rental is not active")
				return
			}
			var returnBody struct {
				Notes     string                 `json:"notes"`
				Checklist map[string]interface{} `json:"checklist"`
			}
			json.NewDecoder(r.Body).Decode(&returnBody)
			var checklistJSON []byte
			if returnBody.Checklist != nil {
				checklistJSON, _ = json.Marshal(returnBody.Checklist)
			}
			c.rentalRepo.ReturnByNumber(r.Context(), rental.RentalNumber, checklistJSON, returnBody.Notes)
			udids, _ := c.rentalRepo.ListDeviceUdidsByNumber(r.Context(), rental.RentalNumber)
			for _, udid := range udids {
				c.assetRepo.ClearHolderByUdid(r.Context(), udid)
			}
			// Notify custodian that devices are returned
			go func() {
				bgCtx := context.Background()
				data := c.buildNotifyData(bgCtx, rental.RentalNumber, "", approverDisplayName, "", returnBody.Notes, nil)
				data.ReturnNotes = returnBody.Notes
				// Notify each custodian
				notified := map[string]bool{}
				for _, udid := range udids {
					assets, _ := c.assetRepo.List(bgCtx, udid)
					for _, a := range assets {
						if a.CustodianID != nil && *a.CustodianID != "" {
							custodian, err := c.userRepo.GetByID(bgCtx, *a.CustodianID)
							if err == nil && custodian.Email != "" && !notified[custodian.Email] {
								c.notifySvc.SendRentalReturned(bgCtx, data, custodian.Email)
								notified[custodian.Email] = true
							}
						}
					}
				}
			}()
			log.Printf("[rental] batch returned: rental_number=%d holder cleared", rental.RentalNumber)
			writeJSON(w, map[string]interface{}{"ok": true, "status": "returned"})

		case "reject":
			if rental.Status != "pending" {
				writeError(w, http.StatusBadRequest, "rental is not pending")
				return
			}
			c.rentalRepo.UpdateStatusByNumber(r.Context(), rental.RentalNumber, "pending", "rejected", &claims.UserID, approverDisplayName)
			// Notify borrower that rental is rejected
			go func() {
				bgCtx := context.Background()
				borrowerID, borrowerName, _ := c.rentalRepo.GetBorrowerInfo(bgCtx, id)
				data := c.buildNotifyData(bgCtx, rental.RentalNumber, borrowerName, approverDisplayName, "", "", nil)
				borrower, err := c.userRepo.GetByID(bgCtx, borrowerID)
				if err == nil && borrower.Email != "" {
					c.notifySvc.SendRentalRejected(bgCtx, data, borrower.Email)
				}
			}()
			writeJSON(w, map[string]interface{}{"ok": true, "status": "rejected"})

		default:
			w.WriteHeader(http.StatusBadRequest)
		}
		return
	}

	if r.Method == http.MethodDelete {
		rental, err := c.rentalRepo.GetByID(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusNotFound, "rental not found")
			return
		}
		c.rentalRepo.DeleteByNumber(r.Context(), rental.RentalNumber)
		writeOK(w)
		return
	}

	w.WriteHeader(http.StatusMethodNotAllowed)
}

// handleExport godoc
// @Summary 匯出借用記錄為 Excel
// @Tags Rental
// @Produce application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Security BearerAuth
// @Param status query string false "狀態篩選"
// @Param ids query string false "僅匯出指定借用單 ID（逗號分隔）"
// @Success 200 {file} file "Excel 檔案"
// @Router /api/rentals-export [get]
func (c *RentalController) handleExport(w http.ResponseWriter, r *http.Request) {
	if _, err := c.auth.RequireModule(r, "rental", "requester"); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	status := r.URL.Query().Get("status")
	rentals, err := c.rentalRepo.List(r.Context(), status, "", false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// If specific IDs provided, filter
	if idsParam := r.URL.Query().Get("ids"); idsParam != "" {
		idSet := map[string]bool{}
		for _, id := range strings.Split(idsParam, ",") {
			idSet[strings.TrimSpace(id)] = true
		}
		filtered := make([]*domain.Rental, 0)
		for _, rl := range rentals {
			if idSet[rl.ID] {
				filtered = append(filtered, rl)
			}
		}
		rentals = filtered
	}

	// Group by rental_number
	type rentalGroup struct {
		Number    int
		Rentals   []*domain.Rental
		First     *domain.Rental
	}
	groupMap := map[int]*rentalGroup{}
	for _, rl := range rentals {
		g, ok := groupMap[rl.RentalNumber]
		if !ok {
			g = &rentalGroup{Number: rl.RentalNumber, First: rl}
			groupMap[rl.RentalNumber] = g
		}
		g.Rentals = append(g.Rentals, rl)
	}
	groups := make([]*rentalGroup, 0, len(groupMap))
	for _, g := range groupMap {
		groups = append(groups, g)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Number > groups[j].Number })

	statusLabels := map[string]string{
		"pending": "待核准", "approved": "已核准", "active": "借出中",
		"returned": "已歸還", "rejected": "已拒絕",
	}
	checklistLabels := [][2]string{
		{"deviceReceived", "已收到裝置"},
		{"screenOk", "螢幕正常"},
		{"bodyOk", "機身正常"},
		{"canPowerOn", "可開機"},
		{"accessoriesOk", "配件齊全"},
	}

	f := excelize.NewFile()
	sheet := "租借記錄"
	f.SetSheetName("Sheet1", sheet)

	// Headers
	headers := []string{
		"單號", "裝置數", "裝置名稱", "裝置序號", "借用人", "保管人",
		"用途", "狀態", "借出日期", "預計歸還", "實際歸還", "核准人", "備註",
	}
	for _, cl := range checklistLabels {
		headers = append(headers, "歸還清點-"+cl[1])
	}
	headers = append(headers, "歸還備註", "存查")

	for col, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(col+1, 1)
		f.SetCellValue(sheet, cell, h)
	}

	// Bold header style
	style, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	endCell, _ := excelize.CoordinatesToCellName(len(headers), 1)
	f.SetCellStyle(sheet, "A1", endCell, style)

	// Data rows
	for row, g := range groups {
		r := row + 2 // 1-indexed, skip header
		rl := g.First

		deviceNames := make([]string, 0, len(g.Rentals))
		deviceSerials := make([]string, 0, len(g.Rentals))
		for _, item := range g.Rentals {
			name := item.DeviceName
			if name == "" {
				name = item.DeviceSerial
			}
			deviceNames = append(deviceNames, name)
			deviceSerials = append(deviceSerials, item.DeviceSerial)
		}

		statusLabel := statusLabels[rl.Status]
		if statusLabel == "" {
			statusLabel = rl.Status
		}

		borrowDate := ""
		if !rl.BorrowDate.IsZero() {
			borrowDate = rl.BorrowDate.Format("2006-01-02")
		}
		expectedReturn := ""
		if rl.ExpectedReturn != nil {
			expectedReturn = rl.ExpectedReturn.Format("2006-01-02")
		}
		actualReturn := ""
		if rl.ActualReturn != nil {
			actualReturn = rl.ActualReturn.Format("2006-01-02")
		}

		vals := []interface{}{
			g.Number,
			len(g.Rentals),
			strings.Join(deviceNames, "、"),
			strings.Join(deviceSerials, "、"),
			rl.BorrowerName,
			rl.CustodianName,
			rl.Purpose,
			statusLabel,
			borrowDate,
			expectedReturn,
			actualReturn,
			rl.ApproverName,
			rl.Notes,
		}

		// Checklist columns
		cl := rl.ReturnChecklist
		for _, pair := range checklistLabels {
			v := ""
			if cl != nil {
				if b, ok := cl[pair[0]]; ok {
					if bv, ok := b.(bool); ok && bv {
						v = "V"
					}
				}
			}
			vals = append(vals, v)
		}

		// Return notes + archived
		vals = append(vals, rl.ReturnNotes)
		archived := ""
		if rl.IsArchived {
			archived = "是"
		}
		vals = append(vals, archived)

		for col, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(col+1, r)
			f.SetCellValue(sheet, cell, v)
		}
	}

	// Auto-fit column widths (approximate)
	for col := range headers {
		colName, _ := excelize.ColumnNumberToName(col + 1)
		f.SetColWidth(sheet, colName, colName, 14)
	}

	now := time.Now().Format("2006-01-02")
	filename := fmt.Sprintf("租借記錄_%s.xlsx", now)

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	if err := f.Write(w); err != nil {
		log.Printf("[rental-export] write error: %v", err)
	}
	f.Close()
}

// handleArchive godoc
// @Summary 歸檔借用單
// @Tags Rental
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body swagArchiveReq true "借用單 ID 列表"
// @Success 200 {object} swagOK
// @Failure 400 {object} swagError
// @Router /api/rentals-archive [post]
func (c *RentalController) handleArchive(w http.ResponseWriter, r *http.Request) {
	if _, err := c.auth.RequireModule(r, "rental", "manager"); err != nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "ids required")
		return
	}
	if err := c.rentalRepo.Archive(r.Context(), body.IDs); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w)
}
