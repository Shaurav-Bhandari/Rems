package services

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"html/template"
	"log"
	"net/smtp"
	"strings"
	"time"

	"backend/config"
)

// ============================================================================
// EMAIL SERVICE — Production SMTP Implementation
// Interface-based design: swap EmailSender for testing or different providers.
// ============================================================================

// EmailSender is the interface for sending emails. Implement this to swap
// providers (SMTP, SendGrid, AWS SES, etc.) without touching the service.
type EmailSender interface {
	SendVerificationEmail(email, name, token string)
	SendPasswordChangedNotification(email, name, ip string)
	SendAccountLockedNotification(email, name string, until time.Time)
	SendSuspiciousLoginAlert(email, name, ip, location string)
}

// EmailService implements EmailSender using SMTP with HTML templates.
type EmailService struct {
	host     string
	port     string
	username string
	password string
	from     string
	fromName string
	enabled  bool
	baseURL  string // e.g. "http://localhost:8080"
}

// NewEmailService creates a production EmailService from SMTP config.
func NewEmailService(cfg config.SMTPConfig) *EmailService {
	baseURL := config.GetEnvars("APP_BASE_URL", "http://localhost:8080")
	return &EmailService{
		host:     cfg.Host,
		port:     cfg.Port,
		username: cfg.Username,
		password: cfg.Password,
		from:     cfg.From,
		fromName: cfg.FromName,
		enabled:  cfg.Enabled,
		baseURL:  baseURL,
	}
}

// ────────────────────────────────────────────────────────────────────────────
// PUBLIC METHODS (satisfy EmailSender interface)
// ────────────────────────────────────────────────────────────────────────────

func (e *EmailService) SendVerificationEmail(email, name, token string) {
	if !e.enabled {
		log.Printf("[EMAIL] (disabled) Would send verification email to %s", email)
		return
	}

	verifyURL := fmt.Sprintf("%s/api/v1/auth/verify-email?token=%s", e.baseURL, token)

	data := map[string]interface{}{
		"Name":      name,
		"VerifyURL": verifyURL,
		"Year":      time.Now().Year(),
		"AppName":   e.fromName,
	}

	body, err := renderTemplate(templateVerifyEmail, data)
	if err != nil {
		log.Printf("[EMAIL] template error (verification): %v", err)
		return
	}

	if err := e.send(email, "Verify Your Email Address", body); err != nil {
		log.Printf("[EMAIL] failed to send verification to %s: %v", email, err)
	}
}

func (e *EmailService) SendPasswordChangedNotification(email, name, ip string) {
	if !e.enabled {
		log.Printf("[EMAIL] (disabled) Would send password changed notification to %s", email)
		return
	}

	data := map[string]interface{}{
		"Name":      name,
		"IPAddress": ip,
		"Timestamp": time.Now().Format("January 2, 2006 at 3:04 PM MST"),
		"Year":      time.Now().Year(),
		"AppName":   e.fromName,
	}

	body, err := renderTemplate(templatePasswordChanged, data)
	if err != nil {
		log.Printf("[EMAIL] template error (password changed): %v", err)
		return
	}

	if err := e.send(email, "Your Password Was Changed", body); err != nil {
		log.Printf("[EMAIL] failed to send password changed notification to %s: %v", email, err)
	}
}

func (e *EmailService) SendAccountLockedNotification(email, name string, until time.Time) {
	if !e.enabled {
		log.Printf("[EMAIL] (disabled) Would send account locked notification to %s", email)
		return
	}

	data := map[string]interface{}{
		"Name":       name,
		"LockedUntil": until.Format("January 2, 2006 at 3:04 PM MST"),
		"Duration":   time.Until(until).Round(time.Minute).String(),
		"Year":       time.Now().Year(),
		"AppName":    e.fromName,
	}

	body, err := renderTemplate(templateAccountLocked, data)
	if err != nil {
		log.Printf("[EMAIL] template error (account locked): %v", err)
		return
	}

	if err := e.send(email, "Account Temporarily Locked", body); err != nil {
		log.Printf("[EMAIL] failed to send account locked notification to %s: %v", email, err)
	}
}

func (e *EmailService) SendSuspiciousLoginAlert(email, name, ip, location string) {
	if !e.enabled {
		log.Printf("[EMAIL] (disabled) Would send suspicious login alert to %s", email)
		return
	}

	data := map[string]interface{}{
		"Name":      name,
		"IPAddress": ip,
		"Location":  location,
		"Timestamp": time.Now().Format("January 2, 2006 at 3:04 PM MST"),
		"Year":      time.Now().Year(),
		"AppName":   e.fromName,
	}

	body, err := renderTemplate(templateSuspiciousLogin, data)
	if err != nil {
		log.Printf("[EMAIL] template error (suspicious login): %v", err)
		return
	}

	if err := e.send(email, "⚠️ Suspicious Login Detected", body); err != nil {
		log.Printf("[EMAIL] failed to send suspicious login alert to %s: %v", email, err)
	}
}

// ────────────────────────────────────────────────────────────────────────────
// SMTP TRANSPORT
// ────────────────────────────────────────────────────────────────────────────

func (e *EmailService) send(to, subject, htmlBody string) error {
	headers := map[string]string{
		"From":         fmt.Sprintf("%s <%s>", e.fromName, e.from),
		"To":           to,
		"Subject":      subject,
		"MIME-Version": "1.0",
		"Content-Type": "text/html; charset=UTF-8",
	}

	var msg strings.Builder
	for k, v := range headers {
		msg.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	msg.WriteString("\r\n")
	msg.WriteString(htmlBody)

	auth := smtp.PlainAuth("", e.username, e.password, e.host)
	addr := fmt.Sprintf("%s:%s", e.host, e.port)

	// Use TLS for port 465, STARTTLS for 587
	if e.port == "465" {
		return e.sendTLS(addr, auth, to, msg.String())
	}
	return smtp.SendMail(addr, auth, e.from, []string{to}, []byte(msg.String()))
}

func (e *EmailService) sendTLS(addr string, auth smtp.Auth, to, msg string) error {
	tlsConfig := &tls.Config{
		ServerName: e.host,
		MinVersion: tls.VersionTLS13,
	}

	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("tls dial: %w", err)
	}

	client, err := smtp.NewClient(conn, e.host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Close()

	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}
	if err := client.Mail(e.from); err != nil {
		return fmt.Errorf("smtp mail: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close: %w", err)
	}

	return client.Quit()
}

// ────────────────────────────────────────────────────────────────────────────
// TEMPLATE ENGINE
// ────────────────────────────────────────────────────────────────────────────

func renderTemplate(tmplStr string, data map[string]interface{}) (string, error) {
	tmpl, err := template.New("email").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	return buf.String(), nil
}

// ────────────────────────────────────────────────────────────────────────────
// HTML TEMPLATES
// ────────────────────────────────────────────────────────────────────────────

const emailBaseStyle = `
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; margin: 0; padding: 0; background: #f4f4f7; }
  .container { max-width: 600px; margin: 0 auto; padding: 40px 20px; }
  .card { background: #ffffff; border-radius: 12px; padding: 40px; box-shadow: 0 2px 8px rgba(0,0,0,0.08); }
  .header { text-align: center; margin-bottom: 32px; }
  .header h1 { color: #1a1a2e; font-size: 24px; margin: 0; }
  .content { color: #4a4a68; font-size: 16px; line-height: 1.6; }
  .btn { display: inline-block; padding: 14px 32px; background: #6366f1; color: #ffffff; text-decoration: none; border-radius: 8px; font-weight: 600; font-size: 16px; margin: 24px 0; }
  .btn:hover { background: #4f46e5; }
  .alert { background: #fef2f2; border: 1px solid #fecaca; border-radius: 8px; padding: 16px; margin: 16px 0; color: #991b1b; }
  .info-box { background: #f0f9ff; border: 1px solid #bae6fd; border-radius: 8px; padding: 16px; margin: 16px 0; }
  .info-box p { margin: 4px 0; color: #0c4a6e; font-size: 14px; }
  .footer { text-align: center; margin-top: 32px; color: #9ca3af; font-size: 13px; }
</style>
`

const templateVerifyEmail = `<!DOCTYPE html><html><head>` + emailBaseStyle + `</head><body>
<div class="container"><div class="card">
  <div class="header"><h1>{{.AppName}}</h1></div>
  <div class="content">
    <p>Hi {{.Name}},</p>
    <p>Thanks for creating your account! Please verify your email address by clicking the button below:</p>
    <div style="text-align:center;">
      <a href="{{.VerifyURL}}" class="btn">Verify Email Address</a>
    </div>
    <p>Or copy and paste this URL into your browser:</p>
    <p style="word-break:break-all; font-size:14px; color:#6366f1;">{{.VerifyURL}}</p>
    <p>This link expires in 24 hours. If you didn't create an account, you can safely ignore this email.</p>
  </div>
</div>
<div class="footer"><p>© {{.Year}} {{.AppName}}. All rights reserved.</p></div>
</div></body></html>`

const templatePasswordChanged = `<!DOCTYPE html><html><head>` + emailBaseStyle + `</head><body>
<div class="container"><div class="card">
  <div class="header"><h1>{{.AppName}}</h1></div>
  <div class="content">
    <p>Hi {{.Name}},</p>
    <p>Your password was successfully changed.</p>
    <div class="info-box">
      <p><strong>When:</strong> {{.Timestamp}}</p>
      <p><strong>IP Address:</strong> {{.IPAddress}}</p>
    </div>
    <div class="alert">
      <p><strong>⚠️ Didn't make this change?</strong></p>
      <p>If you did not change your password, your account may be compromised. Please contact support immediately and reset your password.</p>
    </div>
  </div>
</div>
<div class="footer"><p>© {{.Year}} {{.AppName}}. All rights reserved.</p></div>
</div></body></html>`

const templateAccountLocked = `<!DOCTYPE html><html><head>` + emailBaseStyle + `</head><body>
<div class="container"><div class="card">
  <div class="header"><h1>{{.AppName}}</h1></div>
  <div class="content">
    <p>Hi {{.Name}},</p>
    <div class="alert">
      <p><strong>🔒 Your account has been temporarily locked</strong></p>
      <p>Multiple failed login attempts were detected on your account.</p>
    </div>
    <div class="info-box">
      <p><strong>Locked until:</strong> {{.LockedUntil}}</p>
      <p><strong>Duration:</strong> {{.Duration}}</p>
    </div>
    <p>Your account will be automatically unlocked after this period. If this wasn't you, please reset your password immediately after the lockout expires.</p>
  </div>
</div>
<div class="footer"><p>© {{.Year}} {{.AppName}}. All rights reserved.</p></div>
</div></body></html>`

const templateSuspiciousLogin = `<!DOCTYPE html><html><head>` + emailBaseStyle + `</head><body>
<div class="container"><div class="card">
  <div class="header"><h1>{{.AppName}}</h1></div>
  <div class="content">
    <p>Hi {{.Name}},</p>
    <div class="alert">
      <p><strong>⚠️ Suspicious Login Detected</strong></p>
      <p>We detected a login to your account from an unrecognized location or device.</p>
    </div>
    <div class="info-box">
      <p><strong>When:</strong> {{.Timestamp}}</p>
      <p><strong>IP Address:</strong> {{.IPAddress}}</p>
      <p><strong>Location:</strong> {{.Location}}</p>
    </div>
    <p>If this was you, you can ignore this email. If you don't recognize this activity, please change your password immediately and enable two-factor authentication.</p>
  </div>
</div>
<div class="footer"><p>© {{.Year}} {{.AppName}}. All rights reserved.</p></div>
</div></body></html>`
