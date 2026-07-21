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

// MaintenanceController handles 資通設備進出及維護申請單 (equipment maintenance
// dispatch requests): applicant submits, 承辦人員 (handler) signs, 權責主管
// (supervisor) approves — which flips the asset to "repairing" — then the
// asset is checked back in on return.
type MaintenanceController struct {
	maintRepo port.MaintenanceRepository
	assetRepo port.AssetRepository
	userRepo  port.UserRepository
	auditRepo port.AuditRepository
	auth      *middleware.AuthHelper
}

func NewMaintenanceController(maintRepo port.MaintenanceRepository, assetRepo port.AssetRepository, userRepo port.UserRepository, auditRepo port.AuditRepository, auth *middleware.AuthHelper) *MaintenanceController {
	return &MaintenanceController{maintRepo: maintRepo, assetRepo: assetRepo, userRepo: userRepo, auditRepo: auditRepo, auth: auth}
}

func (c *MaintenanceController) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/maintenance-requests", c.handleList)
	mux.HandleFunc("/api/maintenance-requests-export", c.handleExport)
	mux.HandleFunc("/api/maintenance-requests/", c.handleByID)
}

func maintenanceToRow(m *domain.MaintenanceRequest) map[string]interface{} {
	row := map[string]interface{}{
		"id": m.ID, "request_number": m.RequestNumber, "asset_id": m.AssetID,
		"asset_name": m.AssetName, "asset_number": m.AssetNumber,
		"applicant_id": m.ApplicantID, "applicant_name": m.ApplicantName,
		"reason": m.Reason, "vendor": m.Vendor, "technician": m.Technician,
		"process_notes": m.ProcessNotes, "status": m.Status,
		"handler_id": m.HandlerID, "handler_name": m.HandlerName,
		"supervisor_id": m.SupervisorID, "supervisor_name": m.SupervisorName,
		"reject_reason": m.RejectReason, "is_archived": m.IsArchived,
		"created_at": m.CreatedAt.Format(time.RFC3339), "updated_at": m.UpdatedAt.Format(time.RFC3339),
		"contains_sensitive_data": m.ContainsSensitiveData, "vendor_nda_ref": m.VendorNDARef,
		"data_wiped_before_checkout": m.DataWipedBeforeCheckout,
		"loaner_info":                m.LoanerInfo,
		"loaner_security_checked":    m.LoanerSecurityChecked,
	}
	if m.CheckoutDate != nil {
		row["checkout_date"] = m.CheckoutDate.Format("2006-01-02")
	} else {
		row["checkout_date"] = nil
	}
	if m.ReturnDate != nil {
		row["return_date"] = m.ReturnDate.Format("2006-01-02")
	} else {
		row["return_date"] = nil
	}
	if m.HandledAt != nil {
		row["handled_at"] = m.HandledAt.Format(time.RFC3339)
	} else {
		row["handled_at"] = nil
	}
	if m.ApprovedAt != nil {
		row["approved_at"] = m.ApprovedAt.Format(time.RFC3339)
	} else {
		row["approved_at"] = nil
	}
	if m.LoanerProvidedDate != nil {
		row["loaner_provided_date"] = m.LoanerProvidedDate.Format("2006-01-02")
	} else {
		row["loaner_provided_date"] = nil
	}
	if m.LoanerReturnedDate != nil {
		row["loaner_returned_date"] = m.LoanerReturnedDate.Format("2006-01-02")
	} else {
		row["loaner_returned_date"] = nil
	}
	return row
}

// handleList godoc
// @Summary 維修申請列表 / 建立維修申請
// @Tags Maintenance
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param status query string false "狀態篩選" Enums(pending,handler_signed,approved,returned,rejected)
// @Param show_archived query string false "顯示已歸檔" Enums(true,false)
// @Param body body swagMaintenanceReq false "建立維修申請（POST）"
// @Success 200 {object} map[string]interface{} "GET: {requests: [...]}, POST: {ids, count, request_number}"
// @Router /api/maintenance-requests [get]
// @Router /api/maintenance-requests [post]
func (c *MaintenanceController) handleList(w http.ResponseWriter, r *http.Request) {
	claims, err := c.auth.RequireModule(r, "maintenance", "requester")
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		status := r.URL.Query().Get("status")
		showArchived := r.URL.Query().Get("show_archived") == "true"
		reqs, err := c.maintRepo.List(r.Context(), status, showArchived)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		rows := make([]map[string]interface{}, 0, len(reqs))
		for _, m := range reqs {
			rows = append(rows, maintenanceToRow(m))
		}
		writeJSON(w, map[string]interface{}{"requests": rows})

	case http.MethodPost:
		var body struct {
			AssetIDs              []string `json:"asset_ids"`
			ApplicantID           string   `json:"applicant_id"`
			Reason                string   `json:"reason"`
			Vendor                string   `json:"vendor"`
			Technician            string   `json:"technician"`
			CheckoutDate          *string  `json:"checkout_date"`
			ReturnDate            *string  `json:"return_date"`
			ContainsSensitiveData bool     `json:"contains_sensitive_data"`
			VendorNDARef          string   `json:"vendor_nda_ref"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.AssetIDs) == 0 || body.ApplicantID == "" {
			writeError(w, http.StatusBadRequest, "asset_ids and applicant_id required")
			return
		}

		applicantName := ""
		if applicant, err := c.userRepo.GetByID(r.Context(), body.ApplicantID); err == nil {
			applicantName = applicant.DisplayName
			if applicantName == "" {
				applicantName = applicant.Username
			}
		}

		checkoutDate := parseDatePtr(body.CheckoutDate)
		returnDate := parseDatePtr(body.ReturnDate)

		requestNumber, _ := c.maintRepo.NextRequestNumber(r.Context())

		var ids []string
		for _, assetID := range body.AssetIDs {
			req := &domain.MaintenanceRequest{
				RequestNumber:         requestNumber,
				AssetID:               assetID,
				ApplicantID:           body.ApplicantID,
				ApplicantName:         applicantName,
				Reason:                body.Reason,
				Vendor:                body.Vendor,
				Technician:            body.Technician,
				CheckoutDate:          checkoutDate,
				ReturnDate:            returnDate,
				ContainsSensitiveData: body.ContainsSensitiveData,
				VendorNDARef:          body.VendorNDARef,
			}
			id, err := c.maintRepo.Create(r.Context(), req)
			if err != nil {
				log.Printf("maintenance insert: %v", err)
				continue
			}
			ids = append(ids, id)
		}
		_ = claims
		writeJSON(w, map[string]interface{}{"ids": ids, "count": len(ids), "request_number": requestNumber})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func parseDatePtr(s *string) *time.Time {
	if s == nil || *s == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", *s)
	if err != nil {
		return nil
	}
	return &t
}

// handleByID godoc
// @Summary 維修申請操作：sign / approve / return / reject
// @Tags Maintenance
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "維修申請 ID"
// @Param action path string false "操作" Enums(sign,approve,return,reject)
// @Param body body swagMaintenanceApproveReq false "核准時的資安檢核（approve 時使用）"
// @Param body body swagMaintenanceReturnReq false "歸還資訊（return 時使用）"
// @Success 200 {object} swagOK
// @Router /api/maintenance-requests/{id}/{action} [post]
// @Router /api/maintenance-requests/{id} [delete]
func (c *MaintenanceController) handleByID(w http.ResponseWriter, r *http.Request) {
	claims, err := c.auth.RequireModule(r, "maintenance", "requester")
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/maintenance-requests/"), "/")
	id := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	if r.Method == http.MethodPost && action != "" {
		req, err := c.maintRepo.GetByID(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusNotFound, "maintenance request not found")
			return
		}

		operatorName := claims.Username
		if u, err := c.userRepo.GetByID(r.Context(), claims.UserID); err == nil && u.DisplayName != "" {
			operatorName = u.DisplayName
		}

		switch action {
		case "sign":
			if _, err := c.auth.RequireModule(r, "maintenance", "operator"); err != nil {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			if req.Status != "pending" {
				writeError(w, http.StatusBadRequest, "request is not pending")
				return
			}
			if err := c.maintRepo.SignByHandler(r.Context(), req.RequestNumber, claims.UserID, operatorName); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			c.auditRepo.Create(r.Context(), &domain.AuditLog{
				UserID: claims.UserID, Username: claims.Username,
				Action: "maintenance_sign", Target: id,
				Detail: fmt.Sprintf("request_number=%d", req.RequestNumber), Module: "maintenance",
				IPAddress: clientIP(r), UserAgent: r.UserAgent(),
			})
			writeJSON(w, map[string]interface{}{"ok": true, "status": "handler_signed"})

		case "approve":
			if _, err := c.auth.RequireModule(r, "maintenance", "approver"); err != nil {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			if req.Status != "handler_signed" {
				writeError(w, http.StatusBadRequest, "request has not been signed by handler yet")
				return
			}
			var body struct {
				CheckoutDate            *string `json:"checkout_date"`
				DataWipedBeforeCheckout bool    `json:"data_wiped_before_checkout"`
				LoanerInfo              string  `json:"loaner_info"`
				LoanerProvidedDate      *string `json:"loaner_provided_date"`
				LoanerSecurityChecked   bool    `json:"loaner_security_checked"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			approveParams := domain.MaintenanceApproveParams{
				SupervisorID:            claims.UserID,
				SupervisorName:          operatorName,
				CheckoutDate:            parseDatePtr(body.CheckoutDate),
				DataWipedBeforeCheckout: body.DataWipedBeforeCheckout,
				LoanerInfo:              body.LoanerInfo,
				LoanerProvidedDate:      parseDatePtr(body.LoanerProvidedDate),
				LoanerSecurityChecked:   body.LoanerSecurityChecked,
			}
			if err := c.maintRepo.ApproveBySupervisor(r.Context(), req.RequestNumber, approveParams); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			assetIDs, _ := c.maintRepo.ListAssetIDsByNumber(r.Context(), req.RequestNumber)
			for _, aid := range assetIDs {
				c.assetRepo.Update(r.Context(), aid, map[string]interface{}{"asset_status": "repairing"})
			}
			auditDetail := fmt.Sprintf("request_number=%d data_wiped_before_checkout=%t", req.RequestNumber, body.DataWipedBeforeCheckout)
			if body.LoanerInfo != "" {
				auditDetail += fmt.Sprintf(" loaner_info=%s loaner_security_checked=%t", body.LoanerInfo, body.LoanerSecurityChecked)
			}
			c.auditRepo.Create(r.Context(), &domain.AuditLog{
				UserID: claims.UserID, Username: claims.Username,
				Action: "maintenance_approve", Target: id,
				Detail: auditDetail, Module: "maintenance",
				IPAddress: clientIP(r), UserAgent: r.UserAgent(),
			})
			log.Printf("[maintenance] approved: request_number=%d supervisor=%s", req.RequestNumber, operatorName)
			writeJSON(w, map[string]interface{}{"ok": true, "status": "approved"})

		case "return":
			if _, err := c.auth.RequireModule(r, "maintenance", "operator"); err != nil {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			if req.Status != "approved" {
				writeError(w, http.StatusBadRequest, "request is not checked out")
				return
			}
			var body struct {
				ReturnDate         *string `json:"return_date"`
				ProcessNotes       string  `json:"process_notes"`
				LoanerReturnedDate *string `json:"loaner_returned_date"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			returnParams := domain.MaintenanceReturnParams{
				ReturnDate:         parseDatePtr(body.ReturnDate),
				ProcessNotes:       body.ProcessNotes,
				LoanerReturnedDate: parseDatePtr(body.LoanerReturnedDate),
			}
			if err := c.maintRepo.Return(r.Context(), req.RequestNumber, returnParams); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			assetIDs, _ := c.maintRepo.ListAssetIDsByNumber(r.Context(), req.RequestNumber)
			for _, aid := range assetIDs {
				c.assetRepo.Update(r.Context(), aid, map[string]interface{}{"asset_status": "available"})
			}
			c.auditRepo.Create(r.Context(), &domain.AuditLog{
				UserID: claims.UserID, Username: claims.Username,
				Action: "maintenance_return", Target: id,
				Detail: fmt.Sprintf("request_number=%d", req.RequestNumber), Module: "maintenance",
				IPAddress: clientIP(r), UserAgent: r.UserAgent(),
			})
			writeJSON(w, map[string]interface{}{"ok": true, "status": "returned"})

		case "reject":
			if _, err := c.auth.RequireModule(r, "maintenance", "operator"); err != nil {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			if req.Status != "pending" && req.Status != "handler_signed" {
				writeError(w, http.StatusBadRequest, "request cannot be rejected in its current state")
				return
			}
			var body struct {
				Reason string `json:"reason"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			if err := c.maintRepo.Reject(r.Context(), req.RequestNumber, body.Reason); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			c.auditRepo.Create(r.Context(), &domain.AuditLog{
				UserID: claims.UserID, Username: claims.Username,
				Action: "maintenance_reject", Target: id,
				Detail: body.Reason, Module: "maintenance",
				IPAddress: clientIP(r), UserAgent: r.UserAgent(),
			})
			writeJSON(w, map[string]interface{}{"ok": true, "status": "rejected"})

		default:
			w.WriteHeader(http.StatusBadRequest)
		}
		return
	}

	if r.Method == http.MethodDelete {
		if _, err := c.auth.RequireModule(r, "maintenance", "manager"); err != nil {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		req, err := c.maintRepo.GetByID(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusNotFound, "maintenance request not found")
			return
		}
		c.maintRepo.DeleteByNumber(r.Context(), req.RequestNumber)
		writeOK(w)
		return
	}

	w.WriteHeader(http.StatusMethodNotAllowed)
}

// handleExport godoc
// @Summary 匯出維修申請為 Excel
// @Tags Maintenance
// @Produce application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Security BearerAuth
// @Success 200 {file} file "Excel 檔案"
// @Router /api/maintenance-requests-export [get]
func (c *MaintenanceController) handleExport(w http.ResponseWriter, r *http.Request) {
	if _, err := c.auth.RequireModule(r, "maintenance", "requester"); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	status := r.URL.Query().Get("status")
	reqs, err := c.maintRepo.List(r.Context(), status, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	statusLabels := map[string]string{
		"pending": "待承辦", "handler_signed": "待主管核准", "approved": "維修中",
		"returned": "已歸還", "rejected": "已拒絕",
	}

	f := excelize.NewFile()
	sheet := "設備維修申請"
	f.SetSheetName("Sheet1", sheet)

	headers := []string{
		"申請單號", "申請人員", "設備名稱", "設備編號", "申請原因", "維修廠商", "維修人員",
		"攜出日期", "歸還日期", "作業過程", "狀態", "承辦人員", "權責主管",
		"含機敏資料", "廠商保密協議", "送修前已清除資料",
		"替代機資訊", "替代機提供日期", "替代機已資安檢查", "替代機收回日期",
	}
	for col, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(col+1, 1)
		f.SetCellValue(sheet, cell, h)
	}
	style, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	endCell, _ := excelize.CoordinatesToCellName(len(headers), 1)
	f.SetCellStyle(sheet, "A1", endCell, style)

	for i, m := range reqs {
		row := i + 2
		checkoutDate := ""
		if m.CheckoutDate != nil {
			checkoutDate = m.CheckoutDate.Format("2006-01-02")
		}
		returnDate := ""
		if m.ReturnDate != nil {
			returnDate = m.ReturnDate.Format("2006-01-02")
		}
		loanerProvidedDate := ""
		if m.LoanerProvidedDate != nil {
			loanerProvidedDate = m.LoanerProvidedDate.Format("2006-01-02")
		}
		loanerReturnedDate := ""
		if m.LoanerReturnedDate != nil {
			loanerReturnedDate = m.LoanerReturnedDate.Format("2006-01-02")
		}
		statusLabel := statusLabels[m.Status]
		if statusLabel == "" {
			statusLabel = m.Status
		}
		yesNo := func(b bool) string {
			if b {
				return "是"
			}
			return "否"
		}
		vals := []interface{}{
			m.RequestNumber, m.ApplicantName, m.AssetName, m.AssetNumber, m.Reason, m.Vendor, m.Technician,
			checkoutDate, returnDate, m.ProcessNotes, statusLabel, m.HandlerName, m.SupervisorName,
			yesNo(m.ContainsSensitiveData), m.VendorNDARef, yesNo(m.DataWipedBeforeCheckout),
			m.LoanerInfo, loanerProvidedDate, yesNo(m.LoanerSecurityChecked), loanerReturnedDate,
		}
		for col, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(col+1, row)
			f.SetCellValue(sheet, cell, v)
		}
	}

	for col := range headers {
		colName, _ := excelize.ColumnNumberToName(col + 1)
		f.SetColWidth(sheet, colName, colName, 14)
	}

	now := time.Now().Format("2006-01-02")
	filename := fmt.Sprintf("設備維修申請_%s.xlsx", now)

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	if err := f.Write(w); err != nil {
		log.Printf("[maintenance-export] write error: %v", err)
	}
	f.Close()
}
