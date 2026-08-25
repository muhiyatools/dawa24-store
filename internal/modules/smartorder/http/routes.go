package http

import "github.com/go-chi/chi/v5"

// RegisterRoutes mounts the smart ordering API on the customer surface.
//
// Mounted alongside the existing purchase-request routes rather than as a new
// top-level section: smart ordering is an additional way to start a طلب الشراء,
// not a replacement for the ones already there (FR-000, FR-058).
func RegisterRoutes(r chi.Router, h *Handler, v *Reviewer) {
	r.Get("/api/v1/smart-order", h.History)
	r.Post("/api/v1/smart-order", h.Start)
	r.Get("/api/v1/smart-order/{id}", h.Get)
	r.Post("/api/v1/smart-order/{id}/mapping", h.ConfirmMapping)
	r.Get("/api/v1/smart-order/{id}/results", h.Results)
	r.Get("/api/v1/smart-order/{id}/events", h.Events)

	r.Get("/api/v1/smart-order/{id}/lines/{lineID}/candidates", h.Candidates)
	r.Post("/api/v1/smart-order/{id}/lines/{lineID}/match", v.CorrectMatch)
	r.Post("/api/v1/smart-order/{id}/lines/{lineID}/quantity", v.SetQuantity)
	r.Post("/api/v1/smart-order/{id}/lines/{lineID}/supplier", v.ChooseSupplier)
	r.Post("/api/v1/smart-order/{id}/lines/{lineID}/remove", v.RemoveLine)

	r.Post("/api/v1/smart-order/{id}/finalize", v.Finalize)
}
