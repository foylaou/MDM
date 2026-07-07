package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/anthropics/mdm-server/internal/adapter/postgres"
	"github.com/anthropics/mdm-server/internal/domain"
	"github.com/anthropics/mdm-server/internal/middleware"
	"github.com/anthropics/mdm-server/internal/port"
)

// DisposalController handles 資訊資產報廢申請 (asset disposal applications):
// applicant submits one or more assets with a dispose reason and a data-wipe
// checklist per line; 權責主管 (approver) approves — which requires every
// line's checklist to be confirmed — and the approval executes the actual
// disposal via AssetRepository.Dispose.
type DisposalController struct {
	disposalRepo port.DisposalRepository
	assetRepo    port.AssetRepository
	userRepo     port.UserRepository
	auth         *middleware.AuthHelper
}

func NewDisposalController(disposalRepo port.DisposalRepository, assetRepo port.AssetRepository, userRepo port.UserRepository, auth *middleware.AuthHelper) *DisposalController {
	return &DisposalController{disposalRepo: disposalRepo, assetRepo: assetRepo, userRepo: userRepo, auth: auth}
}

func (c *DisposalController) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/disposal-requests", c.handleList)
	mux.HandleFunc("/api/disposal-requests-export", c.handleExport)
	mux.HandleFunc("/api/disposal-requests/", c.handleByID)
}

func disposalToRow(d *domain.DisposalRequest) map[string]interface{} {
	items := make([]map[string]interface{}, 0, len(d.Items))
	for _, it := range d.Items {
		row := map[string]interface{}{
			"line_no": it.LineNo, "asset_id": it.AssetID,
			"asset_name": it.AssetName, "asset_number": it.AssetNumber,
			"dispose_reason": it.DisposeReason, "data_wipe_checked": it.DataWipeChecked,
		}
		if it.DisposeDate != nil {
			row["dispose_date"] = it.DisposeDate.Format("2006-01-02")
		} else {
			row["dispose_date"] = nil
		}
		items = append(items, row)
	}
	row := map[string]interface{}{
		"id": d.ID, "request_number": d.RequestNumber,
		"applicant_id": d.ApplicantID, "applicant_name": d.ApplicantName,
		"status": d.Status, "approver_id": d.ApproverID, "approver_name": d.ApproverName,
		"reject_reason": d.RejectReason, "is_archived": d.IsArchived,
		"created_at": d.CreatedAt.Format(time.RFC3339), "updated_at": d.UpdatedAt.Format(time.RFC3339),
		"items": items,
	}
	if d.ApprovedAt != nil {
		row["approved_at"] = d.ApprovedAt.Format(time.RFC3339)
	} else {
		row["approved_at"] = nil
	}
	return row
}

// handleList godoc
// @Summary 報廢申請列表 / 建立報廢申請
// @Tags Disposal
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param status query string false "狀態篩選" Enums(pending,approved,rejected)
// @Param show_archived query string false "顯示已歸檔" Enums(true,false)
// @Param body body swagDisposalReq false "建立報廢申請（POST）"
// @Success 200 {object} map[string]interface{} "GET: {requests: [...]}, POST: {id, request_number}"
// @Router /api/disposal-requests [get]
// @Router /api/disposal-requests [post]
func (c *DisposalController) handleList(w http.ResponseWriter, r *http.Request) {
	claims, err := c.auth.RequireModule(r, "disposal", "requester")
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		status := r.URL.Query().Get("status")
		showArchived := r.URL.Query().Get("show_archived") == "true"
		reqs, err := c.disposalRepo.List(r.Context(), status, showArchived)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		rows := make([]map[string]interface{}, 0, len(reqs))
		for _, d := range reqs {
			rows = append(rows, disposalToRow(d))
		}
		writeJSON(w, map[string]interface{}{"requests": rows})

	case http.MethodPost:
		var body struct {
			ApplicantID string `json:"applicant_id"`
			Items       []struct {
				AssetID         string  `json:"asset_id"`
				DisposeDate     *string `json:"dispose_date"`
				DisposeReason   string  `json:"dispose_reason"`
				DataWipeChecked bool    `json:"data_wipe_checked"`
			} `json:"items"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ApplicantID == "" || len(body.Items) == 0 {
			writeError(w, http.StatusBadRequest, "applicant_id and items required")
			return
		}
		for _, it := range body.Items {
			if it.AssetID == "" || it.DisposeReason == "" {
				writeError(w, http.StatusBadRequest, "asset_id and dispose_reason required for every item")
				return
			}
		}

		applicantName := ""
		if applicant, err := c.userRepo.GetByID(r.Context(), body.ApplicantID); err == nil {
			applicantName = applicant.DisplayName
			if applicantName == "" {
				applicantName = applicant.Username
			}
		}

		items := make([]domain.DisposalRequestItem, 0, len(body.Items))
		var unavailable []string
		for _, it := range body.Items {
			asset, err := c.assetRepo.GetByID(r.Context(), it.AssetID)
			if err != nil || asset == nil {
				unavailable = append(unavailable, it.AssetID+" (不存在)")
				continue
			}
			if asset.AssetStatus == "retired" {
				unavailable = append(unavailable, asset.Name+" (已報廢)")
				continue
			}
			items = append(items, domain.DisposalRequestItem{
				AssetID: it.AssetID, AssetName: asset.Name, AssetNumber: asset.AssetNumber,
				DisposeDate: parseDatePtr(it.DisposeDate), DisposeReason: it.DisposeReason,
				DataWipeChecked: it.DataWipeChecked,
			})
		}
		if len(unavailable) > 0 {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": "部分資產無法申請報廢", "assets": unavailable})
			return
		}

		requestNumber, _ := c.disposalRepo.NextRequestNumber(r.Context())
		id, err := c.disposalRepo.Create(r.Context(), &domain.DisposalRequest{
			RequestNumber: requestNumber, ApplicantID: body.ApplicantID, ApplicantName: applicantName, Items: items,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		_ = claims
		writeJSON(w, map[string]interface{}{"id": id, "request_number": requestNumber})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// handleByID godoc
// @Summary 報廢申請操作：approve / reject / delete
// @Tags Disposal
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "報廢申請 ID"
// @Param action path string false "操作" Enums(approve,reject)
// @Success 200 {object} swagOK
// @Failure 409 {object} swagError "資料清除檢核未全部確認"
// @Router /api/disposal-requests/{id}/{action} [post]
// @Router /api/disposal-requests/{id} [delete]
func (c *DisposalController) handleByID(w http.ResponseWriter, r *http.Request) {
	claims, err := c.auth.RequireModule(r, "disposal", "requester")
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/disposal-requests/"), "/")
	id := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	if r.Method == http.MethodPost && action != "" {
		req, err := c.disposalRepo.GetByID(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusNotFound, "disposal request not found")
			return
		}

		approverName := claims.Username
		if u, err := c.userRepo.GetByID(r.Context(), claims.UserID); err == nil && u.DisplayName != "" {
			approverName = u.DisplayName
		}

		switch action {
		case "approve":
			if _, err := c.auth.RequireModule(r, "disposal", "approver"); err != nil {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			if req.Status != "pending" {
				writeError(w, http.StatusBadRequest, "request is not pending")
				return
			}
			if err := c.disposalRepo.Approve(r.Context(), id, claims.UserID, approverName); err != nil {
				if errors.Is(err, postgres.ErrDataWipeNotChecked) {
					writeError(w, http.StatusConflict, "所有資產的資料清除檢核必須先確認才能核准報廢")
					return
				}
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			for _, it := range req.Items {
				if err := c.assetRepo.Dispose(r.Context(), it.AssetID, claims.UserID, it.DisposeReason); err != nil {
					log.Printf("[disposal] dispose asset %s failed: %v", it.AssetID, err)
				}
			}
			log.Printf("[disposal] approved: request_number=%d approver=%s items=%d", req.RequestNumber, approverName, len(req.Items))
			writeJSON(w, map[string]interface{}{"ok": true, "status": "approved"})

		case "reject":
			if _, err := c.auth.RequireModule(r, "disposal", "approver"); err != nil {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			if req.Status != "pending" {
				writeError(w, http.StatusBadRequest, "request is not pending")
				return
			}
			var body struct {
				Reason string `json:"reason"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			if err := c.disposalRepo.Reject(r.Context(), id, body.Reason); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, map[string]interface{}{"ok": true, "status": "rejected"})

		default:
			w.WriteHeader(http.StatusBadRequest)
		}
		return
	}

	if r.Method == http.MethodDelete {
		if _, err := c.auth.RequireModule(r, "disposal", "manager"); err != nil {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if err := c.disposalRepo.Delete(r.Context(), id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeOK(w)
		return
	}

	w.WriteHeader(http.StatusMethodNotAllowed)
}

// handleExport godoc
// @Summary 匯出報廢申請為 Excel
// @Tags Disposal
// @Produce application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Security BearerAuth
// @Success 200 {file} file "Excel 檔案"
// @Router /api/disposal-requests-export [get]
func (c *DisposalController) handleExport(w http.ResponseWriter, r *http.Request) {
	if _, err := c.auth.RequireModule(r, "disposal", "requester"); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	status := r.URL.Query().Get("status")
	reqs, err := c.disposalRepo.List(r.Context(), status, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	statusLabels := map[string]string{"pending": "待核准", "approved": "已報廢", "rejected": "已拒絕"}

	f := excelize.NewFile()
	sheet := "資訊資產報廢申請"
	f.SetSheetName("Sheet1", sheet)

	headers := []string{"申請單號", "NO", "申請人員", "資產名稱", "資產編號", "報廢日期", "報廢原因", "資料清除檢核", "狀態", "核准人"}
	for col, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(col+1, 1)
		f.SetCellValue(sheet, cell, h)
	}
	style, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	endCell, _ := excelize.CoordinatesToCellName(len(headers), 1)
	f.SetCellStyle(sheet, "A1", endCell, style)

	row := 2
	for _, d := range reqs {
		statusLabel := statusLabels[d.Status]
		if statusLabel == "" {
			statusLabel = d.Status
		}
		for _, it := range d.Items {
			disposeDate := ""
			if it.DisposeDate != nil {
				disposeDate = it.DisposeDate.Format("2006-01-02")
			}
			checked := ""
			if it.DataWipeChecked {
				checked = "V"
			}
			vals := []interface{}{
				d.RequestNumber, it.LineNo, d.ApplicantName, it.AssetName, it.AssetNumber,
				disposeDate, it.DisposeReason, checked, statusLabel, d.ApproverName,
			}
			for col, v := range vals {
				cell, _ := excelize.CoordinatesToCellName(col+1, row)
				f.SetCellValue(sheet, cell, v)
			}
			row++
		}
	}

	for col := range headers {
		colName, _ := excelize.ColumnNumberToName(col + 1)
		f.SetColWidth(sheet, colName, colName, 14)
	}

	now := time.Now().Format("2006-01-02")
	filename := fmt.Sprintf("資訊資產報廢申請_%s.xlsx", now)

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	if err := f.Write(w); err != nil {
		log.Printf("[disposal-export] write error: %v", err)
	}
	f.Close()
}
