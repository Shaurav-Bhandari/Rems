package handlers

import (
	"strconv"

	DTO "backend/DTO"
	services "backend/services"
	"backend/utils"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ============================================================================
// HANDLER
// ============================================================================

type AdminHandler struct {
	svc *services.AdminService
}

func NewAdminHandler(db *gorm.DB) *AdminHandler {
	return &AdminHandler{svc: services.NewAdminService(db)}
}

// ============================================================================
// ROUTES (register these in your router)
//
//   admin := app.Group("/api/v1/admins")
//   admin.Post("/",              h.Create)
//   admin.Get("/",               h.List)
//   admin.Get("/:id",            h.Get)
//   admin.Put("/:id",            h.Update)
//   admin.Delete("/:id",         h.Delete)
//   admin.Put("/:id/password",   h.ChangePassword)
//
// ============================================================================

// Create — POST /api/v1/admins
func (h *AdminHandler) Create(c fiber.Ctx) error {
	var req DTO.CreateAdminRequest
	if err := c.Bind().JSON(&req); err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid JSON", err.Error())
	}

	resp, err := h.svc.Create(&req)
	if err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, err.Error(), nil)
	}

	return utils.SendResponse(c, fiber.StatusCreated, "Admin created", resp)
}

// List — GET /api/v1/admins?page=1&page_size=20&search=foo
func (h *AdminHandler) List(c fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	pageSize, _ := strconv.Atoi(c.Query("page_size", "20"))
	search := c.Query("search")

	resp, err := h.svc.List(page, pageSize, search)
	if err != nil {
		return utils.SendResponse(c, fiber.StatusInternalServerError, "Failed to list admins", nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Admins retrieved", resp)
}

// Get — GET /api/v1/admins/:id
func (h *AdminHandler) Get(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid admin ID", nil)
	}

	resp, err := h.svc.GetByID(id)
	if err != nil {
		return utils.SendResponse(c, fiber.StatusNotFound, err.Error(), nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Admin retrieved", resp)
}

// Update — PUT /api/v1/admins/:id
func (h *AdminHandler) Update(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid admin ID", nil)
	}

	var req DTO.UpdateAdminRequest
	if err := c.Bind().JSON(&req); err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid JSON", err.Error())
	}

	resp, err := h.svc.Update(id, &req)
	if err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, err.Error(), nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Admin updated", resp)
}

// Delete — DELETE /api/v1/admins/:id
func (h *AdminHandler) Delete(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid admin ID", nil)
	}

	if err := h.svc.Delete(id); err != nil {
		return utils.SendResponse(c, fiber.StatusNotFound, err.Error(), nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Admin deleted", nil)
}

// ChangePassword — PUT /api/v1/admins/:id/password
func (h *AdminHandler) ChangePassword(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid admin ID", nil)
	}

	var req DTO.ChangeAdminPasswordRequest
	if err := c.Bind().JSON(&req); err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, "Invalid JSON", err.Error())
	}

	if err := h.svc.ChangePassword(id, &req); err != nil {
		return utils.SendResponse(c, fiber.StatusBadRequest, err.Error(), nil)
	}

	return utils.SendResponse(c, fiber.StatusOK, "Password changed", nil)
}