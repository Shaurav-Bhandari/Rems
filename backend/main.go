package main

import (
	"backend/DB"
	"backend/config"
	"fmt"
	// "backend/models"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/log"
)

func main() {
	s := "gopher"
	cfg := config.LoadDBConfig()
    DB.InitDB(cfg)

	fmt.Printf("Hello and welcome, %s!\n", s)
	app := fiber.New()
	api := app.Group("/api/V1")
	api.Get("/", func(c fiber.Ctx) error {
		return c.SendString("Hello from API V1!")
	})

	log.Fatal(app.Listen(":3000"))
}