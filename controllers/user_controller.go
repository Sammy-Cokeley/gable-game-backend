package controllers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"gable-backend/database"
	"gable-backend/mail"
	"gable-backend/models"
	"net/url"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

func generateVerificationToken() (string, error) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func Register(c *fiber.Ctx) error {
	var data struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.BodyParser(&data); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid input"})
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(data.Password), 14)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to hash password"})
	}

	// Generate verification token
	token, err := generateVerificationToken()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to generate verification token"})
	}

	_, err = database.DB.Exec(`
        INSERT INTO users (email, password_hash, verified, verification_token)
        VALUES ($1, $2, $3, $4)
    `, data.Email, string(hash), false, token)
	if err != nil {
		fmt.Println("error: ", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "User already exists or invalid data"})
	}

	// Send verification email
	verificationURL := fmt.Sprintf("%s/verify-email?token=%s&email=%s",
		os.Getenv("FRONTEND_URL"),
		url.QueryEscape(token),
		url.QueryEscape(data.Email))

	// Using a mail package you'll create
	err = mail.SendVerificationEmail(data.Email, verificationURL)
	if err != nil {
		// Log error but don't return it to user
		fmt.Printf("Failed to send verification email: %v\n", err)
	}

	return c.JSON(fiber.Map{
		"message":              "User registered successfully. Please check your email to verify your account.",
		"requiresVerification": true,
	})
}

func VerifyEmail(c *fiber.Ctx) error {
	token := c.Query("token")
	email := c.Query("email")

	if token == "" || email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid verification link"})
	}

	result, err := database.DB.Exec(`
        UPDATE users 
        SET verified = true, verification_token = NULL 
        WHERE email = $1 AND verification_token = $2 AND verified = false
    `, email, token)

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Verification failed"})
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid or expired verification link"})
	}

	return c.JSON(fiber.Map{"message": "Email verified successfully. You can now log in."})
}

func ResendVerification(c *fiber.Ctx) error {
	var data struct {
		Email string `json:"email"`
	}
	if err := c.BodyParser(&data); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid input"})
	}

	// Check if user exists and is not verified
	var verified bool
	var token string

	err := database.DB.QueryRow(`
        SELECT verified, verification_token FROM users WHERE email = $1
    `, data.Email).Scan(&verified, &token)

	if err != nil {
		// Don't reveal if email exists
		return c.JSON(fiber.Map{"message": "If your email exists in our system, a verification link has been sent"})
	}

	if verified {
		return c.JSON(fiber.Map{"message": "Your email is already verified"})
	}

	// Generate new token if needed
	if token == "" {
		token, err = generateVerificationToken()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to generate verification token"})
		}

		_, err = database.DB.Exec(`
            UPDATE users SET verification_token = $1 WHERE email = $2
        `, token, data.Email)

		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update verification token"})
		}
	}

	// Send verification email
	verificationURL := fmt.Sprintf("%s/verify-email?token=%s&email=%s",
		os.Getenv("FRONTEND_URL"),
		url.QueryEscape(token),
		url.QueryEscape(data.Email))

	err = mail.SendVerificationEmail(data.Email, verificationURL)
	if err != nil {
		// Log error but don't reveal to user
		fmt.Printf("Failed to send verification email: %v\n", err)
	}

	return c.JSON(fiber.Map{"message": "Verification email has been sent"})
}

func Login(c *fiber.Ctx) error {
	var data struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.BodyParser(&data); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid input"})
	}

	var user models.User
	var verified bool
	err := database.DB.QueryRow(`
        SELECT id, email, password_hash, verified FROM users WHERE email = $1
    `, data.Email).Scan(&user.ID, &user.Email, &user.PasswordHash, &verified)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid credentials"})
	}

	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(data.Password)) != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Incorrect password"})
	}

	if !verified {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error":                "Email not verified",
			"requiresVerification": true,
		})
	}

	// Generate JWT (same as before)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": user.ID,
		"exp":     time.Now().Add(time.Hour * 72).Unix(),
	})

	secret := os.Getenv("JWT_SECRET")
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create token"})
	}

	return c.JSON(fiber.Map{"token": tokenString})
}

func GetMe(c *fiber.Ctx) error {
	user := c.Locals("user").(*jwt.Token)
	claims := user.Claims.(jwt.MapClaims)
	userID := claims["user_id"].(float64)

	var userData models.User
	err := database.DB.QueryRow(`
		SELECT id, email FROM users WHERE id = $1
	`, int(userID)).Scan(&userData.ID, &userData.Email)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "User not found"})
	}

	return c.JSON(userData)
}
