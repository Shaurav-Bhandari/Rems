package DTO

import (
    "time"
    "github.com/google/uuid"
)

// LoginRequest - User login
type LoginRequest struct {
    Email    string `json:"email" binding:"required,email"`
    Password string `json:"password" binding:"required,min=8"`
}

// LoginResponse - Login success
type LoginResponse struct {
    Token        string    `json:"token"`
    RefreshToken string    `json:"refresh_token"`
    ExpiresAt    time.Time `json:"expires_at"`
    User         UserInfo  `json:"user"`
}

type UserInfo struct {
    UserID       uuid.UUID `json:"user_id"`
    Email        string    `json:"email"`
    FullName     string    `json:"full_name"`
    TenantID     uuid.UUID `json:"tenant_id"`
    TenantName   string    `json:"tenant_name"`
    DefaultRole  string    `json:"default_role"`
}

// RegisterRequest - New user registration
type RegisterRequest struct {
    Email           string `json:"email" binding:"required,email"`
    Password        string `json:"password" binding:"required,min=8"`
    PasswordConfirm string `json:"password_confirm" binding:"required,eqfield=Password"`
    FullName        string `json:"full_name" binding:"required,min=2,max=255"`
    Phone           string `json:"phone" binding:"omitempty,e164"` // E.164 format
    TenantName      string `json:"tenant_name" binding:"required,min=2,max=255"`
}

// ChangePasswordRequest - Password change
type ChangePasswordRequest struct {
    CurrentPassword string `json:"current_password" binding:"required"`
    NewPassword     string `json:"new_password" binding:"required,min=8"`
    ConfirmPassword string `json:"confirm_password" binding:"required,eqfield=NewPassword"`
}

// ForgotPasswordRequest - Request password reset
type ForgotPasswordRequest struct {
    Email string `json:"email" binding:"required,email"`
}

// ResetPasswordRequest - Reset with token
type ResetPasswordRequest struct {
    Token           string `json:"token" binding:"required"`
    NewPassword     string `json:"new_password" binding:"required,min=8"`
    ConfirmPassword string `json:"confirm_password" binding:"required,eqfield=NewPassword"`
}