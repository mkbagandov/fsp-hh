// Package mail — отправка писем. В dev письма не уходят наружу: консольный мейлер
// пишет их в лог и хранит последние N в памяти для страницы /dev/outbox,
// чтобы подтверждение e-mail работало без SMTP.
package mail

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Message — письмо.
type Message struct {
	To      string    `json:"to"`
	Subject string    `json:"subject"`
	Text    string    `json:"text"`
	SentAt  time.Time `json:"sent_at"`
}

// Mailer — интерфейс отправки.
type Mailer interface {
	Send(ctx context.Context, msg Message) error
}

// Outbox — доступ к последним письмам (dev).
type Outbox interface {
	Recent(limit int) []Message
}

// Console — мейлер для разработки и тестов.
type Console struct {
	log   *slog.Logger
	mu    sync.Mutex
	items []Message
	cap   int
}

// NewConsole создаёт консольный мейлер с буфером на capacity писем.
func NewConsole(log *slog.Logger, capacity int) *Console {
	if capacity <= 0 {
		capacity = 100
	}
	return &Console{log: log, cap: capacity}
}

// Send пишет письмо в лог и буфер.
func (c *Console) Send(_ context.Context, msg Message) error {
	msg.SentAt = time.Now()
	c.mu.Lock()
	c.items = append(c.items, msg)
	if len(c.items) > c.cap {
		c.items = c.items[len(c.items)-c.cap:]
	}
	c.mu.Unlock()
	if c.log != nil {
		c.log.Info("mail (console)", "to", msg.To, "subject", msg.Subject, "text", msg.Text)
	}
	return nil
}

// Recent возвращает последние письма, новые первыми.
func (c *Console) Recent(limit int) []Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := len(c.items)
	if limit <= 0 || limit > n {
		limit = n
	}
	out := make([]Message, 0, limit)
	for i := n - 1; i >= n-limit; i-- {
		out = append(out, c.items[i])
	}
	return out
}

// Last возвращает последнее письмо адресату (для тестов).
func (c *Console) Last(to string) (Message, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := len(c.items) - 1; i >= 0; i-- {
		if c.items[i].To == to {
			return c.items[i], true
		}
	}
	return Message{}, false
}
