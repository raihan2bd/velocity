package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/raihan2bd/velocity"
)

type createUserRequest struct {
	Name  string `json:"name" validate:"required,min=3,max=80"`
	Email string `json:"email" validate:"required,email"`
}

func main() {
	app := velocity.Default()

	app.GET("/health", func(c *velocity.Context) error {
		return c.JSON(http.StatusOK, velocity.H{"status": "ok"})
	})

	api := app.Group("/api/v1")
	api.GET("/users/:id", func(c *velocity.Context) error {
		return c.JSON(http.StatusOK, velocity.H{"id": c.Param("id")})
	})
	api.POST("/users", velocity.MaxBodyBytes(1<<20), func(c *velocity.Context) error {
		var request createUserRequest
		if err := c.BindJSON(&request); err != nil {
			return err
		}
		return c.JSON(http.StatusCreated, request)
	})

	log.Println("Velocity listening on http://127.0.0.1:8080")
	if err := app.Run(":8080"); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
