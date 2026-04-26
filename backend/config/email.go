package config

// SMTPConfig holds SMTP connection parameters.
type SMTPConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
	FromName string
	Enabled  bool
}

// LoadSMTPConfig loads SMTP config from environment variables.
func LoadSMTPConfig() SMTPConfig {
	host := GetEnvars("SMTP_HOST", "")
	return SMTPConfig{
		Host:     host,
		Port:     GetEnvars("SMTP_PORT", "587"),
		Username: GetEnvars("SMTP_USERNAME", ""),
		Password: GetEnvars("SMTP_PASSWORD", ""),
		From:     GetEnvars("SMTP_FROM", "noreply@rems.local"),
		FromName: GetEnvars("SMTP_FROM_NAME", "ReMS"),
		Enabled:  host != "",
	}
}
