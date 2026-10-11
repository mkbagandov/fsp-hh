// doctor проверяет окружение перед запуском: инструменты, .env, базу данных, порт и сборку фронтенда.
// Каждая строка — одна проверка с подсказкой, что сделать. Код выхода 1, если есть ошибки.
//
//	task doctor
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
)

var failed bool

func ok(format string, a ...any)   { fmt.Printf("  ✓ "+format+"\n", a...) }
func info(format string, a ...any) { fmt.Printf("  · "+format+"\n", a...) }
func bad(format string, a ...any) {
	failed = true
	fmt.Printf("  ✗ "+format+"\n", a...)
}

func main() {
	if run() {
		os.Exit(1)
	}
}

// run выполняет проверки; возвращает true, если среди них есть ошибки.
func run() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	fmt.Println("Проверка окружения «ФСП»")

	ok("Go %s (%s/%s)", strings.TrimPrefix(runtime.Version(), "go"), runtime.GOOS, runtime.GOARCH)
	if out, err := exec.CommandContext(ctx, "node", "--version").Output(); err == nil {
		ok("Node.js %s", strings.TrimSpace(string(out)))
	} else {
		bad("Node.js не найден — нужен для сборки фронтенда (task web:install, task web:build)")
	}
	if _, err := exec.LookPath("task"); err != nil {
		info("go-task не в PATH: команды task недоступны, установка — go install github.com/go-task/task/v3/cmd/task@latest")
	}

	if err := godotenv.Load(".env"); err != nil {
		bad(".env не найден в текущем каталоге — cp .env.example .env и подставьте пароль базы")
	} else {
		ok(".env прочитан")
	}
	for _, key := range []string{"DATABASE_URL", "TEST_DATABASE_URL", "JWT_SECRET"} {
		v := os.Getenv(key)
		switch {
		case v == "":
			bad("%s не задан", key)
		case strings.Contains(v, "CHANGE_ME"):
			bad("%s содержит CHANGE_ME — подставьте реальное значение", key)
		}
	}
	if env := os.Getenv("APP_ENV"); env == "" {
		info("APP_ENV не задан — процесс стартует как prod и потребует COOKIE_SECURE=true и SMTP; для локального запуска APP_ENV=dev")
	} else {
		ok("APP_ENV=%s", env)
	}

	checkDB(ctx, "база", os.Getenv("DATABASE_URL"), true)
	checkDB(ctx, "тестовая база", os.Getenv("TEST_DATABASE_URL"), false)

	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	if l, err := (&net.ListenConfig{}).Listen(ctx, "tcp", addr); err == nil {
		_ = l.Close()
		ok("порт %s свободен", addr)
	} else {
		info("порт %s занят — API уже запущен? (иначе смените HTTP_ADDR)", addr)
	}

	if _, err := os.Stat(filepath.Join("web", "dist", "index.html")); err == nil {
		ok("фронтенд собран (web/dist)")
	} else {
		bad("фронтенд не собран — task web:install && task web:build (API отдаёт web/dist на /)")
	}

	if failed {
		fmt.Println("Есть ошибки — исправьте отмеченные ✗ и запустите снова.")
		return true
	}
	fmt.Println("Всё готово: task run, затем http://127.0.0.1:8080")
	return false
}

// checkDB подключается к базе, сообщает версию PostgreSQL, число применённых миграций и наличие демо-данных.
func checkDB(ctx context.Context, name, url string, withSeed bool) {
	if url == "" {
		return
	}
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		bad("%s: %s — проверьте пароль и имя базы в URL, роль fsp_platform и что PostgreSQL запущен", name, short(err))
		return
	}
	defer func() { _ = conn.Close(ctx) }()
	var version string
	_ = conn.QueryRow(ctx, "SELECT current_setting('server_version')").Scan(&version)
	var migrations int
	err = conn.QueryRow(ctx, "SELECT count(*) FROM goose_db_version WHERE is_applied AND version_id > 0").Scan(&migrations)
	switch {
	case err != nil:
		ok("%s: PostgreSQL %s, миграции ещё не применены — применятся при старте API", name, version)
	default:
		ok("%s: PostgreSQL %s, миграций применено %d", name, version, migrations)
	}
	if withSeed && err == nil {
		var users int
		if qerr := conn.QueryRow(ctx, "SELECT count(*) FROM users WHERE email LIKE '%@seed.fsp.local'").Scan(&users); qerr == nil {
			if users > 0 {
				ok("демо-данные: %d аккаунтов @seed.fsp.local (пароль Demo1234!)", users)
			} else {
				info("демо-данных нет — task seed создаст демо-аккаунты и 300 кандидатов")
			}
		}
	}
}

func short(err error) string {
	var pgErr interface{ Error() string }
	if errors.As(err, &pgErr) {
		s := pgErr.Error()
		if i := strings.Index(s, "("); i > 0 && len(s) > 120 {
			s = s[:i]
		}
		return strings.TrimSpace(s)
	}
	return err.Error()
}
