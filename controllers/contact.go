package controllers

import (
	"net/http"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

type ContactForm struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

func ContactHandler(c *fiber.Ctx) error {
	// Extract user email from context
	emailValue := c.Locals("userEmail")
	email, ok := emailValue.(string)
	if !ok || strings.TrimSpace(email) == "" {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{
			"error": "User email not found",
		})
	}

	form := new(ContactForm)
	if err := c.BodyParser(form); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid input",
		})
	}

	if len(strings.TrimSpace(form.Message)) < 15 {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Message is too short",
		})
	}

	sender := mail.NewEmail("Gable Game Contact", "noreply@gablegame.com")
	recipient := mail.NewEmail("Sammy", "sammy.cokeley@gmail.com")
	subject := "[Gable Game] New Contact Message"

	plainText := "From: " + form.Name + "\nEmail: " + email + "\n\n" + form.Message
	htmlText := "<strong>From:</strong> " + form.Name + "<br><strong>Email:</strong> " + email + "<br><br>" + form.Message

	message := mail.NewSingleEmail(sender, subject, recipient, plainText, htmlText)
	client := sendgrid.NewSendClient(os.Getenv("SENDGRID_API_KEY"))

	_, err := client.Send(message)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to send message",
		})
	}

	return c.SendStatus(http.StatusOK)
}
