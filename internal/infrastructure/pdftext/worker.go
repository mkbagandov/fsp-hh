package pdftext

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"
	"time"
)

// WorkerArg — аргумент командной строки, с которым бинарник API работает как изолированный
// извлекатель текста: PDF читается из stdin, текст пишется в stdout.
const WorkerArg = "pdftext-worker"

// Лимиты дочернего процесса.
const (
	maxTextBytes    = 2 << 20   // больше текста в резюме не бывает
	workerMaxStack  = 64 << 20  // глубокая рекурсия на «петлях» в файле завершает процесс быстро
	workerMemoryCap = 512 << 20 // мягкий лимит кучи Go
)

// ErrTimeout — извлечение не уложилось во время и было прервано.
var ErrTimeout = errors.New("pdftext: extraction timed out")

// RunWorker — точка входа дочернего процесса. Возвращает код выхода.
func RunWorker(in io.Reader, out, errOut io.Writer) int {
	debug.SetMaxStack(workerMaxStack)
	debug.SetMemoryLimit(workerMemoryCap)
	data, err := io.ReadAll(io.LimitReader(in, MaxBytes+1))
	if err != nil || len(data) > MaxBytes {
		_, _ = fmt.Fprintln(errOut, "input is unreadable or larger than the limit")
		return 2
	}
	text, err := Extract(data)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, err)
		return 1
	}
	if len(text) > maxTextBytes {
		text = text[:maxTextBytes]
	}
	if _, err := io.WriteString(out, text); err != nil {
		return 1
	}
	return 0
}

// Isolated извлекает текст в дочернем процессе: зацикливание или падение разбора убивает только его.
// Одновременно работает не больше parallel процессов — загрузки не съедают все ядра.
type Isolated struct {
	exe     string
	timeout time.Duration
	sem     chan struct{}
}

// NewIsolated создаёт извлекатель; exe — бинарник, понимающий WorkerArg (обычно os.Executable()).
func NewIsolated(exe string, timeout time.Duration, parallel int) *Isolated {
	if parallel < 1 {
		parallel = 1
	}
	return &Isolated{exe: exe, timeout: timeout, sem: make(chan struct{}, parallel)}
}

// Extract — то же, что пакетная Extract, но в отдельном процессе с тайм-аутом.
func (x *Isolated) Extract(ctx context.Context, data []byte) (string, error) {
	if !IsPDF(data) {
		return "", ErrNotPDF
	}
	select {
	case x.sem <- struct{}{}:
		defer func() { <-x.sem }()
	case <-ctx.Done():
		return "", ctx.Err()
	}
	ctx, cancel := context.WithTimeout(ctx, x.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, x.exe, WorkerArg) //nolint:gosec // свой же бинарник, аргумент — константа
	cmd.Stdin = bytes.NewReader(data)
	stdout := &capped{max: maxTextBytes}
	stderr := &capped{max: 4 << 10}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	cmd.Env = workerEnv()
	cmd.WaitDelay = 2 * time.Second
	err := cmd.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "", ErrTimeout
	}
	if err != nil {
		return "", fmt.Errorf("pdftext worker: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// workerEnv — минимальное окружение: секреты API (DATABASE_URL, ключи) дочернему процессу не нужны.
func workerEnv() []string {
	env := []string{"GOMAXPROCS=1", "GOMEMLIMIT=512MiB"}
	for _, k := range []string{"SYSTEMROOT", "SystemRoot", "TEMP", "TMP", "TMPDIR"} {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return env
}

// capped — буфер, молча отбрасывающий всё сверх лимита (дочерний процесс не получает ошибку записи).
type capped struct {
	bytes.Buffer
	max int
}

func (c *capped) Write(p []byte) (int, error) {
	if room := c.max - c.Len(); room > 0 {
		if len(p) > room {
			c.Buffer.Write(p[:room])
		} else {
			c.Buffer.Write(p)
		}
	}
	return len(p), nil
}
