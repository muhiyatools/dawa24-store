package http

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Handler handles catalog HTTP requests.
type Handler struct {
	service *catalog.Service
	log     *slog.Logger
}

// NewHandler creates a catalog HTTP handler.
func NewHandler(service *catalog.Service, log *slog.Logger) *Handler {
	return &Handler{service: service, log: log}
}

// RegisterRoutes registers catalog endpoints on a Chi router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	// Feeds the cascading category -> brand selector in every product form.
	r.Get("/api/v1/catalog/categories/{id}/brands", h.ListBrandsByCategory)
	r.Get("/api/v1/catalog/search", h.Search)
	r.Get("/api/v1/catalog/products/{id}", h.GetProduct)
	r.Put("/api/v1/catalog/products/{id}", h.UpdateProduct)
	r.Delete("/api/v1/catalog/products/{id}", h.DeleteProduct)
	r.Post("/api/v1/catalog/products", h.CreateProduct)
	r.Get("/api/v1/catalog/products", h.ListProducts)
	r.Post("/api/v1/catalog/products/bulk-status", h.SetProductsStatus)

	r.Post("/api/v1/catalog/products/{id}/variants", h.CreateVariant)
	r.Get("/api/v1/catalog/products/{id}/variants/{variantId}", h.GetVariant)
	r.Put("/api/v1/catalog/products/{id}/variants/{variantId}", h.UpdateVariant)
	r.Delete("/api/v1/catalog/products/{id}/variants/{variantId}", h.DeleteVariant)

	r.Get("/api/v1/catalog/categories/{id}", h.GetCategory)
	r.Put("/api/v1/catalog/categories/{id}", h.UpdateCategory)
	r.Delete("/api/v1/catalog/categories/{id}", h.DeleteCategory)

	r.Get("/api/v1/catalog/brands/{id}", h.GetBrand)
	r.Put("/api/v1/catalog/brands/{id}", h.UpdateBrand)
	r.Delete("/api/v1/catalog/brands/{id}", h.DeleteBrand)

	r.Get("/api/v1/catalog/categories", h.ListCategories)
	r.Post("/api/v1/catalog/categories", h.CreateCategory)
	r.Get("/api/v1/catalog/brands", h.ListBrands)
	r.Post("/api/v1/catalog/brands", h.CreateBrand)

	r.Post("/api/v1/catalog/pricing/customer", h.SetCustomerPricing)
	r.Get("/api/v1/catalog/pricing/customer", h.GetCustomerPricing)

	r.Post("/api/v1/catalog/alerts", h.CreateProductAlert)
	r.Get("/api/v1/catalog/alerts", h.ListProductAlerts)

	h.RegisterAdminRoutes(r)
}

// Search handles product search queries.
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	var categoryID *int64
	if catStr := r.URL.Query().Get("category_id"); catStr != "" {
		if val, err := strconv.ParseInt(catStr, 10, 64); err == nil {
			categoryID = &val
		}
	}

	var brandID *int64
	if brandStr := r.URL.Query().Get("brand_id"); brandStr != "" {
		if val, err := strconv.ParseInt(brandStr, 10, 64); err == nil {
			brandID = &val
		}
	}

	products, err := h.service.Search(r.Context(), catalog.SearchParams{
		Query:      query,
		CategoryID: categoryID,
		BrandID:    brandID,
		Limit:      limit,
		Offset:     offset,
	})
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"products": products,
		"count":    len(products),
	})
}

// GetProduct retrieves a product and its variants.
func (h *Handler) GetProduct(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid product ID", nil))
		return
	}

	product, variants, err := h.service.GetProduct(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"product":  product,
		"variants": variants,
	})
}

// CreateProduct creates a new product.
func (h *Handler) CreateProduct(w http.ResponseWriter, r *http.Request) {
	var p catalog.Product
	if err := httpx.DecodeJSON(w, r, &p); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	created, err := h.service.CreateProduct(r.Context(), &p)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusCreated, created)
}

// CreateVariant creates a product variant.
func (h *Handler) CreateVariant(w http.ResponseWriter, r *http.Request) {
	productIDStr := chi.URLParam(r, "id")
	productID, err := strconv.ParseInt(productIDStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid product ID", nil))
		return
	}

	var v catalog.ProductVariant
	if err := httpx.DecodeJSON(w, r, &v); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	v.ProductID = productID

	created, err := h.service.CreateVariant(r.Context(), &v)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusCreated, created)
}

// ListCategories lists product categories.
func (h *Handler) ListCategories(w http.ResponseWriter, r *http.Request) {
	categories, err := h.service.ListCategories(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"categories": categories,
	})
}

// ListBrands lists product brands.
func (h *Handler) ListBrands(w http.ResponseWriter, r *http.Request) {
	brands, err := h.service.ListBrands(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"brands": brands,
	})
}

// ListBrandsByCategory returns the manufacturers that operate in one category,
// for the product form's brand selector.
func (h *Handler) ListBrandsByCategory(w http.ResponseWriter, r *http.Request) {
	categoryID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || categoryID <= 0 {
		httpx.Error(w, r, h.log, apperr.Validation("catalog.category_invalid",
			"Category id is invalid.", map[string]string{"id": "معرف التصنيف غير صالح"}))
		return
	}
	brands, err := h.service.ListBrandsByCategory(r.Context(), categoryID)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"brands": brands})
}
