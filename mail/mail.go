package mail

import (
	"fmt"
	"os"

	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

func SendVerificationEmail(to, verificationURL string) error {
	fromEmail := os.Getenv("EMAIL_FROM") // e.g., no-reply@gablegame.com
	apiKey := os.Getenv("SENDGRID_API_KEY")

	subject := "Verify Your Email"
	plainTextContent := fmt.Sprintf("Click the link to verify your email: %s", verificationURL)
	htmlContent := fmt.Sprintf(`
        <html>
        <body>
            <h2>Email Verification</h2>
            <p>Thank you for registering! Please verify your email by clicking the link below:</p>
            <p><a href="%s">Verify Email</a></p>
            <p>If you didn't create this account, you can safely ignore this email.</p>
        </body>
        </html>
    `, verificationURL)

	from := mail.NewEmail("Gable Game", fromEmail)
	toEmail := mail.NewEmail("", to)
	message := mail.NewSingleEmail(from, subject, toEmail, plainTextContent, htmlContent)

	client := sendgrid.NewSendClient(apiKey)
	response, err := client.Send(message)

	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	if response.StatusCode >= 400 {
		return fmt.Errorf("sendgrid error: %d - %s", response.StatusCode, response.Body)
	}

	return nil
}
