package controller

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/anthropics/mdm-server/internal/domain"
	"github.com/anthropics/mdm-server/internal/middleware"
	"github.com/anthropics/mdm-server/internal/port"
)

type CategoryController struct {
	categoryRepo port.CategoryRepository
	auth         *middleware.AuthHelper
}

func NewCategoryController(categoryRepo port.CategoryRepository, auth *middleware.AuthHelper) *CategoryController {
	return &CategoryController{categoryRepo: categoryRepo, auth: auth}
}

func (c *CategoryController) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/categories", c.handleCategories)
	mux.HandleFunc("/api/categories/", c.handleCategoryByID)
}

// handleCategories godoc
// @Summary 分類列表（樹狀排序）/ 新增分類
// @Tags Category
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body swagCategoryReq false "新增分類（POST）"
// @Success 200 {object} map[string][]swagCategoryItem "GET: {categories: [...]}, POST: {id: ...}"
// @Router /api/categories [get]
// @Router /api/categories [post]
func (c *CategoryController) handleCategories(w http.ResponseWriter, r *http.Request) {
	minLevel := "viewer"
	if r.Method == http.MethodPost {
		minLevel = "operator"
	}
	if _, err := c.auth.RequireModule(r, "asset", minLevel); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		allCats, err := c.categoryRepo.List(r.Context())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Build tree-ordered flat list: parent then children recursively
		type catJSON struct {
			ID        string  `json:"id"`
			ParentID  *string `json:"parent_id"`
			Name      string  `json:"name"`
			Level     int     `json:"level"`
			SortOrder int     `json:"sort_order"`
		}
		var sorted []catJSON
		var walk func(parentID *string)
		walk = func(parentID *string) {
			for _, cat := range allCats {
				match := false
				if parentID == nil && cat.ParentID == nil {
					match = true
				} else if parentID == nil && cat.ParentID != nil && *cat.ParentID == "" {
					match = true
				} else if parentID != nil && cat.ParentID != nil && *cat.ParentID == *parentID {
					match = true
				}
				if match {
					sorted = append(sorted, catJSON{
						ID: cat.ID, ParentID: cat.ParentID,
						Name: cat.Name, Level: cat.Level, SortOrder: cat.SortOrder,
					})
					walk(&cat.ID)
				}
			}
		}
		walk(nil)
		if sorted == nil {
			sorted = []catJSON{}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"categories": sorted})

	case http.MethodPost:
		var body struct {
			ParentID *string `json:"parent_id"`
			Name     string  `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		level := 0
		if body.ParentID != nil && *body.ParentID != "" {
			if l, err := c.categoryRepo.GetLevel(r.Context(), *body.ParentID); err == nil {
				level = l + 1
			}
		}
		id, err := c.categoryRepo.Create(r.Context(), &domain.Category{
			ParentID: body.ParentID, Name: body.Name, Level: level,
		})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"id": id})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// handleCategoryByID godoc
// @Summary 更新 / 刪除分類
// @Tags Category
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "分類 ID"
// @Param body body swagCategoryUpdateReq false "更新名稱（PUT）"
// @Success 200 {object} swagOK
// @Router /api/categories/{id} [put]
// @Router /api/categories/{id} [delete]
func (c *CategoryController) handleCategoryByID(w http.ResponseWriter, r *http.Request) {
	if _, err := c.auth.RequireModule(r, "asset", "operator"); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/categories/")
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodPut:
		var body struct {
			Name string `json:"name"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		c.categoryRepo.Update(r.Context(), id, body.Name)
		writeOK(w)
	case http.MethodDelete:
		c.categoryRepo.Delete(r.Context(), id)
		writeOK(w)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
