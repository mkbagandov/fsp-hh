package mail

import (
	"bufio"
	"context"
	"encoding/base64"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeSMTP — минимальный SMTP-сервер для теста: принимает одно письмо и возвращает DATA.
func fakeSMTP(t *testing.T, offerStartTLS bool) (port int, got <-chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	ch := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
		r := bufio.NewReader(conn)
		write := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
		write("220 fake ESMTP")
		var data strings.Builder
		inData := false
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				ch <- data.String()
				return
			}
			if inData {
				if line == ".\r\n" {
					inData = false
					write("250 queued")
					continue
				}
				data.WriteString(line)
				continue
			}
			switch cmd := strings.ToUpper(strings.TrimSpace(line)); {
			case strings.HasPrefix(cmd, "EHLO"):
				if offerStartTLS {
					write("250-fake")
					write("250 STARTTLS")
				} else {
					write("250 fake")
				}
			case strings.HasPrefix(cmd, "STARTTLS"):
				write("454 TLS not available")
			case strings.HasPrefix(cmd, "DATA"):
				inData = true
				write("354 go ahead")
			case strings.HasPrefix(cmd, "QUIT"):
				write("221 bye")
				ch <- data.String()
				return
			default:
				write("250 ok")
			}
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port, ch
}

func TestSMTPSend(t *testing.T) {
	port, got := fakeSMTP(t, false)
	m, err := NewSMTP("127.0.0.1", port, "", "", "ФСП <noreply@fsp.example>")
	if err != nil {
		t.Fatal(err)
	}
	m.insecureLocal = true
	if err := m.Send(context.Background(), Message{To: "user@example.com", Subject: "ФСП: код подтверждения", Text: "Код: 123456"}); err != nil {
		t.Fatal(err)
	}
	data := <-got
	if !strings.Contains(data, "To: <user@example.com>") || !strings.Contains(data, "Subject: =?UTF-8?b?") || !strings.Contains(data, "charset=UTF-8") {
		t.Fatalf("headers:\n%s", data)
	}
	body := data[strings.Index(data, "\r\n\r\n")+4:]
	text, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(strings.TrimSpace(body), "\r\n", ""))
	if err != nil || string(text) != "Код: 123456" {
		t.Fatalf("body: %q %v", text, err)
	}
}

func TestSMTPRefusesPlaintextAndInjection(t *testing.T) {
	port, _ := fakeSMTP(t, false)
	m, err := NewSMTP("127.0.0.1", port, "user", "secret", "noreply@fsp.example")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Send(context.Background(), Message{To: "user@example.com", Subject: "x", Text: "y"}); err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("credentials must never go without TLS, got %v", err)
	}
	if err := m.Send(context.Background(), Message{To: "user@example.com", Subject: "x\r\nBcc: all@example.com", Text: "y"}); err == nil {
		t.Fatal("header injection must be rejected")
	}
	if _, err := NewSMTP("", 25, "", "", "a@b.c"); err == nil {
		t.Fatal("host is required")
	}
	if _, err := NewSMTP("h", 25, "", "", "not an address"); err == nil {
		t.Fatal("from must be an address")
	}
	_ = strconv.Itoa(port)
}
