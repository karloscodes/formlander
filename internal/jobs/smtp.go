package jobs

import (
	"crypto/tls"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// smtpConfig holds the resolved settings for one SMTP send.
type smtpConfig struct {
	Host       string
	Port       int
	Username   string
	Password   string
	Encryption string // starttls | tls | none
	From       string // header form, e.g. "Name <addr>"
	To         string
}

// sendSMTP delivers a pre-built message via SMTP. TLS modes:
//   - "tls":      implicit TLS from connect (typically port 465)
//   - "starttls": upgrade the plaintext connection (typically port 587)
//   - "none":     plaintext (port 25); auth is only allowed to localhost relays
//
// Certificates are always verified; credentials are never logged.
func sendSMTP(cfg *smtpConfig, msg []byte) error {
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))

	var client *smtp.Client
	if cfg.Encryption == "tls" {
		conn, err := tls.Dial("tcp", addr, tlsConfigFor(cfg.Host))
		if err != nil {
			return err
		}
		client, err = smtp.NewClient(conn, cfg.Host)
		if err != nil {
			return err
		}
	} else {
		var err error
		client, err = smtp.Dial(addr)
		if err != nil {
			return err
		}
	}
	defer client.Close()

	if cfg.Encryption == "starttls" {
		if err := client.StartTLS(tlsConfigFor(cfg.Host)); err != nil {
			return err
		}
	}

	if cfg.Username != "" {
		auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
		if err := client.Auth(auth); err != nil {
			return err
		}
	}

	if err := client.Mail(envelopeAddr(cfg.From)); err != nil {
		return err
	}
	if err := client.Rcpt(envelopeAddr(cfg.To)); err != nil {
		return err
	}

	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func tlsConfigFor(host string) *tls.Config {
	return &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
}

// envelopeAddr extracts the bare address for MAIL FROM / RCPT TO, stripping any
// display name. Falls back to the trimmed input if it can't be parsed.
func envelopeAddr(s string) string {
	if addr, err := mail.ParseAddress(s); err == nil {
		return addr.Address
	}
	return strings.TrimSpace(s)
}

// buildSMTPMessage assembles a minimal RFC 5322 plain-text email message.
// Header and body line endings are normalized to CRLF as required by SMTP.
func buildSMTPMessage(from, to, subject, body string) []byte {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("Date: " + time.Now().UTC().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	b.WriteString("\r\n")
	b.WriteString(strings.ReplaceAll(body, "\n", "\r\n"))
	return []byte(b.String())
}
