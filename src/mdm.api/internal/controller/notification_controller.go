package controller

import (
	"net/http"
	"strconv"
	"time"

	"github.com/anthropics/mdm-server/internal/domain"
	"github.com/anthropics/mdm-server/internal/middleware"
	"github.com/anthropics/mdm-server/internal/port"
)

type NotificationController struct {
	notifRepo port.NotificationRepository
	auth      *middleware.AuthHelper
}

func NewNotificationController(notifRepo port.NotificationRepository, auth *middleware.AuthHelper) *NotificationController {
	return &NotificationController{notifRepo: notifRepo, auth: auth}
}

func (c *NotificationController) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/notifications", c.handleNotifications)
}

// notificationToRow converts the domain struct (PascalCase Go fields) into
// the snake_case JSON shape the frontend expects — matches assetToRow /
// maintenanceToRow / disposalToRow elsewhere in this package. Encoding the
// struct directly (as this endpoint used to do) sends keys like "CreatedAt"
// instead of "created_at", so every field the UI reads comes back undefined.
func notificationToRow(n *domain.Notification) map[string]interface{} {
	row := map[string]interface{}{
		"id": n.ID, "type": n.Type, "event": n.Event,
		"recipient": n.Recipient, "subject": n.Subject, "body": n.Body,
		"status": n.Status, "error_message": n.ErrorMessage, "reference_id": n.ReferenceID,
		"created_at": n.CreatedAt.Format(time.RFC3339),
	}
	if n.SentAt != nil {
		row["sent_at"] = n.SentAt.Format(time.RFC3339)
	} else {
		row["sent_at"] = nil
	}
	return row
}

// handleNotifications godoc
// @Summary 通知列表
// @Tags Notification
// @Produce json
// @Security BearerAuth
// @Param event query string false "事件類型篩選"
// @Param reference_id query string false "關聯 ID"
// @Param limit query int false "數量上限" default(50)
// @Success 200 {object} map[string][]swagNotificationItem "{notifications: [...]}"
// @Router /api/notifications [get]
func (c *NotificationController) handleNotifications(w http.ResponseWriter, r *http.Request) {
	if _, err := c.auth.RequireModule(r, "rental", "approver"); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	event := r.URL.Query().Get("event")
	referenceID := r.URL.Query().Get("reference_id")
	limit := 50
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 && l <= 200 {
		limit = l
	}
	notifs, err := c.notifRepo.List(r.Context(), event, referenceID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	rows := make([]map[string]interface{}, 0, len(notifs))
	for _, n := range notifs {
		rows = append(rows, notificationToRow(n))
	}
	writeJSON(w, map[string]interface{}{"notifications": rows})
}
