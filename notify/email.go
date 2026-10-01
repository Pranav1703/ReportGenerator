package notify

import (
	"fmt"

	mail "github.com/wneessen/go-mail"
)

type SMTPConfig struct {
	Host string
	Port int
	User string
	Pass string
	From string
}

func SendReport(cfg SMTPConfig, filePath, subject string, recipients []string) error {
	if len(recipients) == 0 {
		return fmt.Errorf("no recipients provided")
	}

	m := mail.NewMsg()
	if err := m.From(cfg.From); err != nil {
		return fmt.Errorf("invalid from address: %w", err)
	}
	if err := m.To(recipients...); err != nil {
		return fmt.Errorf("invalid recipient: %w", err)
	}
	m.Subject(subject)
	m.SetBodyString(mail.TypeTextPlain, "Here is the tech report.")
	m.AttachFile(filePath)

	host := cfg.Host
	if host == "" {
		host = "smtp.gmail.com"
	}
	port := cfg.Port
	if port == 0 {
		port = 587
	}

	c, err := mail.NewClient(host,
		mail.WithPort(port),
		mail.WithSMTPAuth(mail.SMTPAuthAutoDiscover),
		mail.WithUsername(cfg.User),
		mail.WithPassword(cfg.Pass),
		mail.WithTLSPortPolicy(mail.TLSMandatory),
	)
	if err != nil {
		return fmt.Errorf("creating SMTP client: %w", err)
	}

	if err := c.DialAndSend(m); err != nil {
		return fmt.Errorf("sending email: %w", err)
	}

	return nil
}
