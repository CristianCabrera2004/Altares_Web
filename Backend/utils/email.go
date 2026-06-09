// Backend/utils/email.go
// ─────────────────────────────────────────────────────────────────────────────
// Utilidad para envío de correos electrónicos con SMTP y archivos adjuntos.
// ─────────────────────────────────────────────────────────────────────────────
package utils

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/smtp"
	"os"
)

// SendEmail con soporte opcional de adjunto (PDF).
func SendEmail(to, subject, bodyHTML, pdfBase64, filename string) error {
	smtpHost := os.Getenv("SMTP_HOST")
	smtpPort := os.Getenv("SMTP_PORT")
	smtpUser := os.Getenv("SMTP_USER")
	smtpPass := os.Getenv("SMTP_PASSWORD")
	smtpFrom := os.Getenv("SMTP_FROM")

	// Si no hay configuración SMTP, loggear pero no fallar si estamos en dev
	if smtpHost == "" || smtpPort == "" || smtpUser == "" || smtpPass == "" {
		fmt.Printf("⚠️ CONFIGURACIÓN SMTP INCOMPLETA. Email simulado a %s. Asunto: %s\n", to, subject)
		return nil
	}

	if smtpFrom == "" {
		smtpFrom = smtpUser
	}

	auth := smtp.PlainAuth("", smtpUser, smtpPass, smtpHost)

	// Si no hay adjunto, enviar email simple multipart/alternative o html
	if pdfBase64 == "" {
		msg := []byte("To: " + to + "\r\n" +
			"From: " + smtpFrom + "\r\n" +
			"Subject: " + subject + "\r\n" +
			"MIME-Version: 1.0\r\n" +
			"Content-Type: text/html; charset=UTF-8\r\n" +
			"\r\n" +
			bodyHTML + "\r\n")
		return smtp.SendMail(smtpHost+":"+smtpPort, auth, smtpFrom, []string{to}, msg)
	}

	// Decodificar base64 del PDF para validación
	fileBytes, err := base64.StdEncoding.DecodeString(pdfBase64)
	if err != nil {
		return fmt.Errorf("error al decodificar pdf base64: %w", err)
	}

	boundary := "my-multipart-boundary-altares"

	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("To: %s\r\n", to))
	buf.WriteString(fmt.Sprintf("From: %s\r\n", smtpFrom))
	buf.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=%s\r\n", boundary))
	buf.WriteString("\r\n")

	// Cuerpo HTML
	buf.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	buf.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	buf.WriteString("\r\n")
	buf.WriteString(bodyHTML)
	buf.WriteString("\r\n\r\n")

	// Adjunto PDF
	buf.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	buf.WriteString("Content-Type: application/pdf\r\n")
	buf.WriteString("Content-Transfer-Encoding: base64\r\n")
	buf.WriteString(fmt.Sprintf("Content-Disposition: attachment; filename=\"%s\"\r\n", filename))
	buf.WriteString("\r\n")

	b64Content := base64.StdEncoding.EncodeToString(fileBytes)
	for i := 0; i < len(b64Content); i += 76 {
		end := i + 76
		if end > len(b64Content) {
			end = len(b64Content)
		}
		buf.WriteString(b64Content[i:end] + "\r\n")
	}
	buf.WriteString("\r\n")
	buf.WriteString(fmt.Sprintf("--%s--\r\n", boundary))

	addr := fmt.Sprintf("%s:%s", smtpHost, smtpPort)
	return smtp.SendMail(addr, auth, smtpFrom, []string{to}, buf.Bytes())
}
