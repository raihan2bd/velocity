package velocity_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/raihan2bd/velocity"
)

func Example() {
	engine := velocity.New()
	engine.GET("/users/:id", func(c *velocity.Context) error {
		return c.JSON(http.StatusOK, velocity.H{"id": c.Param("id")})
	})
	engine.Freeze()

	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/users/42", nil))

	fmt.Println(response.Code)
	fmt.Println(response.Body.String())
	// Output:
	// 200
	// {"id":"42"}
}
