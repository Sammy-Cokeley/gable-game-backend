package middleware

import (
	"fmt"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

func RequireAuth(c *fiber.Ctx) error {
	tokenString := c.Get("Authorization")
	if tokenString == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Missing token"})
	}
	if !strings.HasPrefix(tokenString, "Bearer ") {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid header format"})
	}

	rawToken := tokenString[len("Bearer "):]
	token, err := jwt.Parse(rawToken, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(os.Getenv("JWT_SECRET")), nil
	})

	if err != nil || token == nil || !token.Valid {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid or expired token"})
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid token claims"})
	}

	userIDFloat, ok := claims["user_id"].(float64)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Missing user ID in token"})
	}
	c.Locals("user_id", int(userIDFloat))

	email, _ := claims["email"].(string)
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Missing email in token"})
	}
	c.Locals("email", email)

	return c.Next()
}

func RequireAdmin() fiber.Handler {
	allowed := parseAllowedEmails(os.Getenv("ADMIN_EMAILS"))

	return func(c *fiber.Ctx) error {
		if len(allowed) == 0 {
			return c.Status(fiber.StatusInternalServerError).
				JSON(fiber.Map{"error": "ADMIN_EMAILS is not configured"})
		}

		emailAny := c.Locals("email")
		email, _ := emailAny.(string)
		email = strings.ToLower(strings.TrimSpace(email))

		if email == "" {
			return c.SendStatus(fiber.StatusUnauthorized)
		}

		if !allowed[email] {
			return c.SendStatus(fiber.StatusForbidden)
		}

		return c.Next()
	}
}

func parseAllowedEmails(raw string) map[string]bool {
	m := make(map[string]bool)
	for _, e := range strings.Split(raw, ",") {
		e = strings.TrimSpace(strings.ToLower(e))
		if e != "" {
			m[e] = true
		}
	}
	return m
}
