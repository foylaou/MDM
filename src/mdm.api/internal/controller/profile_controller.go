package controller

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/mdm-server/internal/domain"
	"github.com/anthropics/mdm-server/internal/middleware"
	"github.com/anthropics/mdm-server/internal/port"
)

type ProfileController struct {
	profileRepo port.ProfileRepository
	auth        *middleware.AuthHelper
}

func NewProfileController(profileRepo port.ProfileRepository, auth *middleware.AuthHelper) *ProfileController {
	return &ProfileController{profileRepo: profileRepo, auth: auth}
}

func (c *ProfileController) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/profiles", c.handleProfiles)
	mux.HandleFunc("/api/profiles/", c.handleProfileByID)
}

// handleProfiles godoc
// @Summary 描述檔列表 / 上傳描述檔
// @Tags Profile
// @Accept mpfd
// @Produce json
// @Security BearerAuth
// @Param file formData file false "mobileconfig 檔案（POST）"
// @Param name formData string false "描述檔名稱（POST）"
// @Success 200 {object} map[string][]swagProfileItem "GET: {profiles: [...]}, POST: {id, name, filename, size}"
// @Router /api/profiles [get]
// @Router /api/profiles [post]
func (c *ProfileController) handleProfiles(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	minLevel := "viewer"
	if r.Method == http.MethodPost {
		minLevel = "operator"
	}
	claims, err := c.auth.RequireModule(r, "mdm", minLevel)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"unauthorized"}`))
		return
	}

	switch r.Method {
	case http.MethodGet:
		profiles, err := c.profileRepo.List(r.Context())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"query failed"}`))
			return
		}
		type profileItem struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			Filename   string `json:"filename"`
			Size       int    `json:"size"`
			UploadedBy string `json:"uploaded_by"`
			CreatedAt  string `json:"created_at"`
		}
		items := make([]profileItem, 0, len(profiles))
		for _, p := range profiles {
			items = append(items, profileItem{
				ID: p.ID, Name: p.Name, Filename: p.Filename,
				Size: p.Size, UploadedBy: p.UploadedBy,
				CreatedAt: p.CreatedAt.Format(time.RFC3339),
			})
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"profiles": items})

	case http.MethodPost:
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"file too large or invalid form"}`))
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"file required"}`))
			return
		}
		defer file.Close()

		content, err := io.ReadAll(file)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"read failed"}`))
			return
		}

		name := r.FormValue("name")
		if name == "" {
			name = strings.TrimSuffix(header.Filename, ".mobileconfig")
		}

		profile := &domain.Profile{
			Name:       name,
			Filename:   header.Filename,
			Content:    content,
			Size:       len(content),
			UploadedBy: claims.Username,
		}
		id, err := c.profileRepo.Create(r.Context(), profile)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"insert failed"}`))
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": id, "name": name, "filename": header.Filename, "size": len(content),
		})
		log.Printf("profile uploaded: %s (%s) by %s", name, header.Filename, claims.Username)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// handleProfileByID godoc
// @Summary 取得描述檔內容（base64）/ 刪除描述檔
// @Tags Profile
// @Produce json
// @Security BearerAuth
// @Param id path string true "描述檔 ID"
// @Success 200 {object} swagProfileContentResp "GET 回傳 base64 內容"
// @Failure 404 {object} swagError
// @Router /api/profiles/{id} [get]
// @Router /api/profiles/{id} [delete]
func (c *ProfileController) handleProfileByID(w http.ResponseWriter, r *http.Request) {
	if _, err := c.auth.RequireModule(r, "mdm", "operator"); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"unauthorized"}`))
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/profiles/")
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		content, filename, err := c.profileRepo.GetContent(r.Context(), id)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"not found"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		encoded := base64.StdEncoding.EncodeToString(content)
		json.NewEncoder(w).Encode(map[string]interface{}{"content_base64": encoded, "filename": filename})

	case http.MethodDelete:
		if err := c.profileRepo.Delete(r.Context(), id); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"delete failed"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		writeOK(w)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
