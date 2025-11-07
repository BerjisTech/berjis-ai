package auth

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
)

type VerifyClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type Options struct {
	CoreAPIBase string
	HTTP        VerifyClient
}

type VerifyResp struct {
	Success bool `json:"success"`
	Data    struct {
		Valid bool   `json:"valid"`
		UUID  string `json:"uuid"`
		Email string `json:"email"`
	} `json:"data"`
}

// RequireAuth verifies the bearer/cookie token against Core API /v1/auth/verify.
// On success, it sets c.Locals("uuid") and c.Locals("email").
func RequireAuth(opts Options) fiber.Handler {
	httpClient := opts.HTTP
	return func(c *fiber.Ctx) error {
		// Pass through bearer or cookie
		token := ""
		if authz := c.Get("Authorization"); strings.HasPrefix(strings.ToLower(authz), "bearer ") {
			token = strings.TrimSpace(authz[7:])
		}
		if token == "" {
			if b := c.Cookies("access", ""); b != "" {
				token = b
			}
		}
		if token == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"success": false, "message": "missing token"})
		}
		req, _ := http.NewRequest(http.MethodPost, strings.TrimRight(opts.CoreAPIBase, "/")+"/v1/auth/verify", http.NoBody)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := httpClient.Do(req)
		if err != nil {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"success": false, "message": "auth verify failed"})
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"success": false, "message": "unauthorized"})
		}
		var vr VerifyResp
		if err := json.NewDecoder(resp.Body).Decode(&vr); err != nil || !vr.Success || !vr.Data.Valid || vr.Data.UUID == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"success": false, "message": "invalid token"})
		}
		c.Locals("uuid", vr.Data.UUID)
		c.Locals("email", vr.Data.Email)
		return c.Next()
	}
}
