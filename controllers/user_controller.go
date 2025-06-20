package controllers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"gable-backend/database"
	"gable-backend/mail"
	"gable-backend/models"
	"log"
	"net/url"
	"os"
	"strconv"
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

	token, err := generateVerificationToken()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to generate verification token"})
	}

	result := database.DB.QueryRow(`
		INSERT INTO users (email, password_hash, verified, verification_token)
		VALUES ($1, $2, $3, $4)
		RETURNING id
		`, data.Email, string(hash), false, token)

	var userID int
	if err = result.Scan(&userID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Could not create"})
	}

	_, err = database.DB.Exec(`
		INSERT INTO user_stats (user_id, win_distribution)
		VALUES ($1, '{}'::jsonb)
		`, userID)

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create user_stats"})
	}

	verificationURL := fmt.Sprintf("%s/verify-email?token=%s&email=%s",
		os.Getenv("FRONTEND_URL"),
		url.QueryEscape(token),
		url.QueryEscape(data.Email))

	err = mail.SendVerificationEmail(data.Email, verificationURL)
	if err != nil {
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

	var verified bool
	var token string

	err := database.DB.QueryRow(`
        SELECT verified, verification_token FROM users WHERE email = $1
    `, data.Email).Scan(&verified, &token)

	if err != nil {
		return c.JSON(fiber.Map{"message": "If your email exists in our system, a verification link has been sent"})
	}

	if verified {
		return c.JSON(fiber.Map{"message": "Your email is already verified"})
	}

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

	verificationURL := fmt.Sprintf("%s/verify-email?token=%s&email=%s",
		os.Getenv("FRONTEND_URL"),
		url.QueryEscape(token),
		url.QueryEscape(data.Email))

	err = mail.SendVerificationEmail(data.Email, verificationURL)
	if err != nil {
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

	return c.JSON(fiber.Map{
		"token": tokenString,
		"user": fiber.Map{
			"id":    user.ID,
			"email": user.Email,
		},
	})
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

func SubmitUserGuess(c *fiber.Ctx) error {
	userID := c.Locals("user_id")
	if userID == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Unauthorized",
		})
	}

	var input models.GuessInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid input",
		})
	}

	parsedDate, err := time.Parse("2006-01-02", input.GuessDate)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid date format. Use YYYY-MM-DD",
		})
	}

	_, err = database.DB.Exec(`
		INSERT INTO user_guesses (user_id, wrestler_id, guess_date, guess_order)
		VALUES ($1, $2, $3, $4)
	`, userID, input.WrestlerID, parsedDate, input.GuessOrder)

	if err != nil {
		fmt.Printf("Insert error: %v\nuserID: %v, wrestlerID: %v, date: %v, order: %v\n",
			err, userID, input.WrestlerID, parsedDate, input.GuessOrder)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to save guess",
		})
	}

	return c.JSON(fiber.Map{
		"message": "Guess submitted successfully",
	})
}

func GetUserGuesses(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(int)

	dateStr := c.Query("date")
	var targetDate time.Time
	var err error

	if dateStr == "" {
		loc, _ := time.LoadLocation("America/New_York")
		targetDate = time.Now().In(loc)
	} else {
		targetDate, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid date format. Use YYYY-MM-DD",
			})
		}
	}

	rows, err := database.DB.Query(`
		SELECT g.id, g.wrestler_id, w.name, w.weight_class, w.year, w.team, w.conference,
		       w.win_percentage, w.ncaa_finish, g.guess_order
		FROM user_guesses g
		JOIN wrestlers_2025 w ON g.wrestler_id = w.id
		WHERE g.user_id = $1 AND g.guess_date = $2
		ORDER BY g.guess_order ASC
	`, userID, targetDate.Format("2006-01-02"))

	if err != nil {
		fmt.Println("DB query error:", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Could not load guesses",
		})
	}
	defer rows.Close()

	var guesses []models.Guess
	for rows.Next() {
		var g models.Guess
		err := rows.Scan(
			new(interface{}),
			new(interface{}),
			&g.Name, &g.WeightClass, &g.Year, &g.Team, &g.Conference,
			&g.WinPercentage, &g.NcaaFinish, &g.GuessOrder,
		)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Error reading guess",
			})
		}
		guesses = append(guesses, g)
	}

	return c.JSON(fiber.Map{
		"guesses": guesses,
	})
}

func UpdateUserStats(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(int)

	type Input struct {
		Result  string `json:"result"`
		Guesses int    `json:"guesses"` // 1–8
	}

	var input Input
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid input"})
	}

	if input.Result != "win" && input.Result != "loss" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid result type"})
	}

	loc, _ := time.LoadLocation("America/New_York")
	today := time.Now().In(loc).Truncate(24 * time.Hour)
	yesterday := today.AddDate(0, 0, -1)

	var stats struct {
		TotalWins     int
		TotalLosses   int
		CurrentStreak int
		MaxStreak     int
		LastWinDate   *time.Time
		WinDist       map[string]int
	}

	row := database.DB.QueryRow(`
		SELECT total_wins, total_losses, current_streak, max_streak, last_win_date, win_distribution
		FROM user_stats
		WHERE user_id = $1
	`, userID)

	var winDistRaw []byte
	err := row.Scan(&stats.TotalWins, &stats.TotalLosses, &stats.CurrentStreak, &stats.MaxStreak, &stats.LastWinDate, &winDistRaw)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Could not load stats"})
	}

	// Unmarshal distribution JSON
	if err := json.Unmarshal(winDistRaw, &stats.WinDist); err != nil {
		stats.WinDist = map[string]int{} // fallback
	}

	if input.Result == "win" {
		// Increment total wins
		stats.TotalWins += 1

		if stats.LastWinDate != nil {

			if stats.LastWinDate.Equal(yesterday) {
				log.Println("inside second if statement")
				stats.CurrentStreak += 1
			} else {
				log.Println("inside else statement")
				stats.CurrentStreak = 1
			}
		}
		if stats.CurrentStreak > stats.MaxStreak {
			stats.MaxStreak = stats.CurrentStreak
		}

		// Update last win date
		stats.LastWinDate = &today

		// Update win distribution
		key := strconv.Itoa(input.Guesses)
		stats.WinDist[key] += 1

	} else if input.Result == "loss" {
		stats.TotalLosses += 1
		stats.CurrentStreak = 0
	}

	// Marshal updated distribution
	updatedDist, _ := json.Marshal(stats.WinDist)

	_, err = database.DB.Exec(`
		UPDATE user_stats
		SET total_wins = $1,
		    total_losses = $2,
		    current_streak = $3,
		    max_streak = $4,
		    last_win_date = $5,
		    win_distribution = $6
		WHERE user_id = $7
	`, stats.TotalWins, stats.TotalLosses, stats.CurrentStreak, stats.MaxStreak, stats.LastWinDate, updatedDist, userID)

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update stats"})
	}

	return c.JSON(fiber.Map{"message": "Stats updated"})
}

func GetUserStats(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(int)

	var stats struct {
		TotalWins       int             `json:"total_wins"`
		TotalLosses     int             `json:"total_losses"`
		CurrentStreak   int             `json:"current_streak"`
		MaxStreak       int             `json:"max_streak"`
		LastWinDate     *string         `json:"last_win_date"`
		WinDistribution json.RawMessage `json:"win_distribution"`
	}

	err := database.DB.QueryRow(`
		SELECT total_wins, total_losses, current_streak, max_streak, last_win_date, win_distribution
		FROM user_stats
		WHERE user_id = $1
	`, userID).Scan(&stats.TotalWins, &stats.TotalLosses, &stats.CurrentStreak, &stats.MaxStreak, &stats.LastWinDate, &stats.WinDistribution)

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve stats"})
	}

	return c.JSON(stats)

}
