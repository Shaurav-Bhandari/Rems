package services

import (
	"errors"
	"fmt"
	"strings"

	"backend/DTO"
	"backend/models"
	core "backend/services/core"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AdminService struct {
	db *gorm.DB
}

func NewAdminService(db *gorm.DB) *AdminService {
	return &AdminService{db: db}
}

func (s *AdminService) Create(req *DTO.CreateAdminRequest) (*DTO.AdminResponse, error) {
	var count int64
	s.db.Model(&models.Admin{}).Where("email = ? OR user_name = ?", req.Email, req.UserName).Count(&count)
	if count > 0 {
		return nil, errors.New("admin with this email or username already exists")
	}

	hashedPassword, err := core.HashPwd(req.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	admin := req.ToModel(hashedPassword)
	if err := s.db.Create(admin).Error; err != nil {
		return nil, err
	}

	return DTO.FromAdmin(admin), nil
}

func (s *AdminService) List(page, pageSize int, search string) (*DTO.ListAdminsResponse, error) {
	var admins []models.Admin
	var total int64

	query := s.db.Model(&models.Admin{}).Where("is_deleted = ?", false)

	if search != "" {
		searchTerm := "%" + strings.ToLower(search) + "%"
		query = query.Where("LOWER(user_name) LIKE ? OR LOWER(full_name) LIKE ? OR LOWER(email) LIKE ?", searchTerm, searchTerm, searchTerm, searchTerm)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	offset := (page - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Find(&admins).Error; err != nil {
		return nil, err
	}

	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))

	return &DTO.ListAdminsResponse{
		Admins:     DTO.FromAdmins(admins),
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

func (s *AdminService) GetByID(id uuid.UUID) (*DTO.AdminResponse, error) {
	var admin models.Admin
	if err := s.db.Where("admin_id = ? AND is_deleted = ?", id, false).First(&admin).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("admin not found")
		}
		return nil, err
	}
	return DTO.FromAdmin(&admin), nil
}

func (s *AdminService) Update(id uuid.UUID, req *DTO.UpdateAdminRequest) (*DTO.AdminResponse, error) {
	var admin models.Admin
	if err := s.db.Where("admin_id = ? AND is_deleted = ?", id, false).First(&admin).Error; err != nil {
		return nil, errors.New("admin not found")
	}

	updates := make(map[string]interface{})
	if req.FullName != nil {
		updates["full_name"] = *req.FullName
	}
	if req.Phone != nil {
		updates["phone"] = *req.Phone
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}

	if len(updates) > 0 {
		if err := s.db.Model(&admin).Updates(updates).Error; err != nil {
			return nil, err
		}
	}

	s.db.First(&admin, "admin_id = ?", id)
	return DTO.FromAdmin(&admin), nil
}

func (s *AdminService) Delete(id uuid.UUID) error {
	result := s.db.Model(&models.Admin{}).Where("admin_id = ?", id).Updates(map[string]interface{}{
		"is_deleted": true,
		"is_active":  false,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("admin not found")
	}
	return nil
}

func (s *AdminService) ChangePassword(id uuid.UUID, req *DTO.ChangeAdminPasswordRequest) error {
	var admin models.Admin
	if err := s.db.Where("admin_id = ? AND is_deleted = ?", id, false).First(&admin).Error; err != nil {
		return errors.New("admin not found")
	}

	match, err := core.ComparePwd(req.CurrentPassword, admin.Password)
	if err != nil || !match {
		return errors.New("invalid current password")
	}

	hashed, err := core.HashPwd(req.NewPassword)
	if err != nil {
		return err
	}

	return s.db.Model(&admin).Update("password", hashed).Error
}