package services

import (
	"crypto/tls"
	"fmt"
	"html/template"
	"log"
	"os"
	"strings"
	"time"

	"github.com/wneessen/go-mail"
)

// ============================================================================
// EMAIL SERVICE — replaces the no-op stub
// ============================================================================

// EmailConfig holds SMTP connection details loaded from environment.
type EmailConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	From     string
	UseTLS   bool
}

// LoadEmailConfig reads SMTP settings from environment variables.
func LoadEmailConfig() *EmailConfig {
	port := 587
	if p := os.Getenv("SMTP_PORT"); p != "" {
		fmt.Sscanf(p, "%d", &port)
	}

	return &EmailConfig{
		Host:     getEnvOrDefault("SMTP_HOST", ""),
		Port:     port,
		User:     getEnvOrDefault("SMTP_USER", ""),
		Password: getEnvOrDefault("SMTP_PASSWORD", ""),
		From:     getEnvOrDefault("SMTP_FROM", "noreply@rems.local"),
		UseTLS:   getEnvOrDefault("SMTP_TLS", "true") == "true",
	}
}

func getEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// EmailService sends transactional emails via SMTP.
type EmailService struct {
	config *EmailConfig
}

// NewEmailService creates an EmailService. If config is nil or Host is empty,
// all sends become no-ops with a log warning (graceful degradation).
func NewEmailService(cfg *EmailConfig) *EmailService {
	if cfg == nil || cfg.Host == "" {
		log.Println("⚠️  EmailService: SMTP not configured — emails will be logged only")
	}
	return &EmailService{config: cfg}
}

// ── Public methods (same signatures the rest of the codebase expects) ────────

func (e *EmailService) SendVerificationEmail(email, name, token string) {
	subject := "Verify your ReMS account"
	body := renderTemplate(verificationEmailTpl, map[string]string{
		"Name":  name,
		"Token": token,
	})
	e.send(email, subject, body)
}

func (e *EmailService) SendPasswordChangedNotification(email, name, ip string) {
	subject := "Your ReMS password was changed"
	body := renderTemplate(passwordChangedTpl, map[string]string{
		"Name":      name,
		"IPAddress": ip,
		"Time":      time.Now().Format("2006-01-02 15:04:05 MST"),
	})
	e.send(email, subject, body)
}

func (e *EmailService) SendAccountLockedNotification(email, name string, until time.Time) {
	subject := "Your ReMS account has been locked"
	body := renderTemplate(accountLockedTpl, map[string]string{
		"Name":        name,
		"LockedUntil": until.Format("2006-01-02 15:04:05 MST"),
	})
	e.send(email, subject, body)
}

func (e *EmailService) SendSuspiciousLoginAlert(email, name, ip, location string) {
	subject := "⚠️ Suspicious login detected on your ReMS account"
	body := renderTemplate(suspiciousLoginTpl, map[string]string{
		"Name":      name,
		"IPAddress": ip,
		"Location":  location,
		"Time":      time.Now().Format("2006-01-02 15:04:05 MST"),
	})
	e.send(email, subject, body)
}

// ── Internal ─────────────────────────────────────────────────────────────────

func (e *EmailService) send(to, subject, htmlBody string) {
	if e.config == nil || e.config.Host == "" {
		log.Printf("📧 [EMAIL-LOG] To=%s Subject=%q (SMTP not configured, email not sent)", to, subject)
		return
	}

	msg := mail.NewMsg()
	if err := msg.From(e.config.From); err != nil {
		log.Printf("❌ EmailService: invalid from address: %v", err)
		return
	}
	if err := msg.To(to); err != nil {
		log.Printf("❌ EmailService: invalid to address %q: %v", to, err)
		return
	}
	msg.Subject(subject)
	msg.SetBodyString(mail.TypeTextHTML, htmlBody)

	opts := []mail.Option{
		mail.WithPort(e.config.Port),
		mail.WithUsername(e.config.User),
		mail.WithPassword(e.config.Password),
	}

	if e.config.UseTLS {
		opts = append(opts, mail.WithTLSPortPolicy(mail.TLSMandatory))
	}
	opts = append(opts, mail.WithTLSConfig(&tls.Config{
		ServerName: e.config.Host,
	}))

	if e.config.User != "" {
		opts = append(opts, mail.WithSMTPAuth(mail.SMTPAuthPlain))
	}

	client, err := mail.NewClient(e.config.Host, opts...)
	if err != nil {
		log.Printf("❌ EmailService: SMTP client error: %v", err)
		return
	}

	if err := client.DialAndSend(msg); err != nil {
		log.Printf("❌ EmailService: send failed to=%s subject=%q: %v", to, subject, err)
		return
	}

	log.Printf("✅ EmailService: sent to=%s subject=%q", to, subject)
}

// ── HTML Templates ───────────────────────────────────────────────────────────

func renderTemplate(tplStr string, data map[string]string) string {
	tpl, err := template.New("email").Parse(tplStr)
	if err != nil {
		log.Printf("❌ EmailService: template parse error: %v", err)
		return ""
	}
	var buf strings.Builder
	if err := tpl.Execute(&buf, data); err != nil {
		log.Printf("❌ EmailService: template execute error: %v", err)
		return ""
	}
	return buf.String()
}

const emailStyles = `
<style>
  body { font-family: 'Segoe UI', Arial, sans-serif; background: #f4f6f9; margin: 0; padding: 0; }
  .container { max-width: 560px; margin: 40px auto; background: #fff; border-radius: 8px; box-shadow: 0 2px 8px rgba(0,0,0,.08); overflow: hidden; }
  .header { background: linear-gradient(135deg, #3b82f6, #1d4ed8); padding: 28px 32px; }
  .header h1 { color: #fff; margin: 0; font-size: 22px; }
  .body { padding: 32px; color: #1e293b; line-height: 1.6; }
  .body p { margin: 0 0 16px; }
  .code { display: inline-block; background: #f1f5f9; border: 1px solid #e2e8f0; border-radius: 6px; padding: 10px 20px; font-size: 20px; font-weight: 700; letter-spacing: 3px; color: #0f172a; }
  .info-box { background: #fef3c7; border-left: 4px solid #f59e0b; padding: 12px 16px; margin: 16px 0; border-radius: 0 6px 6px 0; font-size: 14px; }
  .alert-box { background: #fee2e2; border-left: 4px solid #ef4444; padding: 12px 16px; margin: 16px 0; border-radius: 0 6px 6px 0; font-size: 14px; }
  .footer { padding: 20px 32px; background: #f8fafc; text-align: center; font-size: 12px; color: #94a3b8; }
</style>
`

var verificationEmailTpl = `<!DOCTYPE html><html><head>` + emailStyles + `</head><body>
<div class="container">
  <div class="header"><h1>🔐 ReMS — Email Verification</h1></div>
  <div class="body">
    <p>Hi <strong>{{.Name}}</strong>,</p>
    <p>Thank you for registering with ReMS. Please use the following code to verify your email:</p>
    <p style="text-align:center"><span class="code">{{.Token}}</span></p>
    <p>This code expires in <strong>24 hours</strong>.</p>
    <div class="info-box">If you did not create a ReMS account, you can safely ignore this email.</div>
  </div>
  <div class="footer">&copy; ReMS — Restaurant Management System</div>
</div></body></html>`

var passwordChangedTpl = `<!DOCTYPE html><html><head>` + emailStyles + `</head><body>
<div class="container">
  <div class="header"><h1>🔑 Password Changed</h1></div>
  <div class="body">
    <p>Hi <strong>{{.Name}}</strong>,</p>
    <p>Your ReMS account password was changed successfully.</p>
    <div class="info-box">
      <strong>Time:</strong> {{.Time}}<br>
      <strong>IP Address:</strong> {{.IPAddress}}
    </div>
    <div class="alert-box">If you did not make this change, please contact support immediately and reset your password.</div>
  </div>
  <div class="footer">&copy; ReMS — Restaurant Management System</div>
</div></body></html>`

var accountLockedTpl = `<!DOCTYPE html><html><head>` + emailStyles + `</head><body>
<div class="container">
  <div class="header"><h1>🔒 Account Locked</h1></div>
  <div class="body">
    <p>Hi <strong>{{.Name}}</strong>,</p>
    <p>Your ReMS account has been temporarily locked due to too many failed login attempts.</p>
    <div class="alert-box">
      <strong>Locked until:</strong> {{.LockedUntil}}
    </div>
    <p>If this was not you, we recommend changing your password as soon as the lockout expires.</p>
  </div>
  <div class="footer">&copy; ReMS — Restaurant Management System</div>
</div></body></html>`

var suspiciousLoginTpl = `<!DOCTYPE html><html><head>` + emailStyles + `</head><body>
<div class="container">
  <div class="header"><h1>⚠️ Suspicious Login Detected</h1></div>
  <div class="body">
    <p>Hi <strong>{{.Name}}</strong>,</p>
    <p>We detected an unusual login attempt on your ReMS account.</p>
    <div class="alert-box">
      <strong>IP Address:</strong> {{.IPAddress}}<br>
      <strong>Location:</strong> {{.Location}}<br>
      <strong>Time:</strong> {{.Time}}
    </div>
    <p>If this was you, no action is needed. If not, please change your password immediately and contact support.</p>
  </div>
  <div class="footer">&copy; ReMS — Restaurant Management System</div>
</div></body></html>`
