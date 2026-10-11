package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// SMTP — отправка писем через SMTP-сервер: STARTTLS на 587 (или любом порту, где сервер его
// поддерживает), неявный TLS на 465. Учётные данные никогда не передаются без TLS.
type SMTP struct {
	host, user, password string
	port                 int
	from                 mail.Address
	timeout              time.Duration
	// insecureLocal разрешает отправку без TLS на localhost без учётных данных (тесты, локальный relay).
	insecureLocal bool
}

// NewSMTP проверяет параметры и создаёт мейлер. from — адрес отправителя ("ФСП <noreply@example.ru>").
func NewSMTP(host string, port int, user, password, from string) (*SMTP, error) {
	if host == "" || port <= 0 {
		return nil, errors.New("mail: SMTP_HOST and SMTP_PORT are required")
	}
	addr, err := mail.ParseAddress(from)
	if err != nil {
		return nil, fmt.Errorf("mail: MAIL_FROM: %w", err)
	}
	return &SMTP{host: host, port: port, user: user, password: password, from: *addr, timeout: 30 * time.Second}, nil
}

// Send отправляет письмо. Заголовки собираются из проверенных значений: перевод строки в адресе
// или теме (внедрение заголовков) отклоняется.
func (s *SMTP) Send(ctx context.Context, msg Message) error {
	to, err := mail.ParseAddress(msg.To)
	if err != nil || strings.ContainsAny(msg.To+msg.Subject, "\r\n") {
		return fmt.Errorf("mail: invalid recipient or subject")
	}
	addr := net.JoinHostPort(s.host, strconv.Itoa(s.port))
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	tlsCfg := &tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12}
	var conn net.Conn
	if s.port == 465 {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: tlsCfg}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("mail: dial: %w", err)
	}
	_ = conn.SetDeadline(time.Now().Add(s.timeout))
	c, err := smtp.NewClient(conn, s.host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("mail: smtp handshake: %w", err)
	}
	defer func() { _ = c.Close() }()
	if s.port != 465 {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(tlsCfg); err != nil {
				return fmt.Errorf("mail: starttls: %w", err)
			}
		} else if !s.insecureLocal || s.user != "" {
			return errors.New("mail: server does not offer STARTTLS; refusing to send without TLS")
		}
	}
	if s.user != "" {
		if err := c.Auth(smtp.PlainAuth("", s.user, s.password, s.host)); err != nil {
			return fmt.Errorf("mail: auth: %w", err)
		}
	}
	if err := c.Mail(s.from.Address); err != nil {
		return fmt.Errorf("mail: MAIL FROM: %w", err)
	}
	if err := c.Rcpt(to.Address); err != nil {
		return fmt.Errorf("mail: RCPT TO: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("mail: DATA: %w", err)
	}
	if _, err := w.Write(s.compose(to, msg)); err != nil {
		return fmt.Errorf("mail: write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("mail: send: %w", err)
	}
	return c.Quit()
}

// compose собирает письмо: UTF-8, тема в RFC 2047, тело в base64 с переносами по 76 символов.
func (s *SMTP) compose(to *mail.Address, msg Message) []byte {
	var b bytes.Buffer
	id := make([]byte, 12)
	_, _ = rand.Read(id)
	domain := s.from.Address[strings.LastIndex(s.from.Address, "@")+1:]
	headers := [][2]string{
		{"From", s.from.String()},
		{"To", to.String()},
		{"Subject", mime.BEncoding.Encode("UTF-8", msg.Subject)},
		{"Date", time.Now().Format(time.RFC1123Z)},
		{"Message-ID", "<" + hex.EncodeToString(id) + "@" + domain + ">"},
		{"MIME-Version", "1.0"},
		{"Content-Type", "text/plain; charset=UTF-8"},
		{"Content-Transfer-Encoding", "base64"},
	}
	for _, h := range headers {
		b.WriteString(h[0] + ": " + h[1] + "\r\n")
	}
	b.WriteString("\r\n")
	enc := base64.StdEncoding.EncodeToString([]byte(msg.Text))
	for len(enc) > 76 {
		b.WriteString(enc[:76] + "\r\n")
		enc = enc[76:]
	}
	b.WriteString(enc + "\r\n")
	return b.Bytes()
}
