package utils

import "github.com/gofiber/fiber/v3"



type ApiResponse struct {
	Success bool        `json:"success"`
	Status  int         `json:"status"`
	Message string      `json:"message,omitempty"`
	Data    any `json:"data,omitempty"`
}

func SendResponse(
	c fiber.Ctx,
	status int,
	message string,
	data any,	
) error {
	return c.Status(status).JSON(ApiResponse{
		Success: status >= 100 && status < 300,
		Status:  status,
		Message: message,
		Data:    data,
	})
}