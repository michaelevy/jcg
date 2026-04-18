package main

import (
	"database/sql"
	"embed"
	"flag"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"jcg/internal/db"
	"jcg/internal/handlers"
	"jcg/internal/middleware"
)

// Templates and static assets live alongside main.go under cmd/server/
// so //go:embed can reference them without forbidden ".." path components.
//go:embed templates
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

func main() {
	dsn := flag.String("db", "file:./jcg.db?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000", "SQLite DSN")
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	database, err := db.Open(*dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer database.Close()

	applySeed(database)

	// ParseFS loads templates: "head", "nav", "leaderboard" are reserved names; future templates must avoid naming collisions.
	tmpl := template.Must(
		template.New("").Funcs(template.FuncMap{
			"add": func(a, b int) int { return a + b },
		}).ParseFS(templateFS, "templates/*.html"),
	)

	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatal(err)
	}

	h := handlers.New(database, tmpl)

	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))
	mux.Handle("GET /{$}", middleware.LoadSession(http.HandlerFunc(h.Leaderboard)))
	mux.Handle("GET /history", middleware.LoadSession(http.HandlerFunc(h.SeasonGames)))
	mux.Handle("GET /players/{id}", middleware.LoadSession(http.HandlerFunc(h.PlayerProfile)))
	mux.Handle("GET /game-results/{id}", middleware.LoadSession(http.HandlerFunc(h.GameResultDetail)))

	mux.HandleFunc("GET /login", h.LoginPage)
	// TODO: add CSRF token protection before production deployment
	mux.HandleFunc("POST /login", h.LoginSubmit)
	// TODO: add CSRF token protection before production deployment
	mux.HandleFunc("POST /logout", h.Logout)

	mux.Handle("GET /enter", middleware.RequireAuth(http.HandlerFunc(h.EntryPage)))
	mux.Handle("POST /enter", middleware.RequireAuth(http.HandlerFunc(h.EntrySubmit)))
	mux.Handle("GET /enter/next-game-number", middleware.RequireAuth(http.HandlerFunc(h.NextGameNumber)))
	mux.Handle("POST /enter/season", middleware.RequireAuth(http.HandlerFunc(h.CreateSeason)))

	log.Printf("listening on %s", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}

// applySeed reads JCG_SEED_PLAYERS and JCG_SEED_USERS env vars and idempotently
// creates any listed players and users that don't already exist.
//
//	JCG_SEED_PLAYERS=Alice,Bob,Carol
//	JCG_SEED_USERS=admin:password,viewer:pass2
func applySeed(database *sql.DB) {
	if players := os.Getenv("JCG_SEED_PLAYERS"); players != "" {
		for _, name := range strings.Split(players, ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if _, err := database.Exec(`INSERT OR IGNORE INTO players (name) VALUES (?)`, name); err != nil {
				log.Printf("seed player %q: %v", name, err)
			}
		}
	}

	if users := os.Getenv("JCG_SEED_USERS"); users != "" {
		for _, entry := range strings.Split(users, ",") {
			parts := strings.SplitN(strings.TrimSpace(entry), ":", 2)
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				log.Printf("seed user: invalid entry %q (want username:password)", entry)
				continue
			}
			hash, err := bcrypt.GenerateFromPassword([]byte(parts[1]), bcrypt.DefaultCost)
			if err != nil {
				log.Printf("seed user %q: bcrypt: %v", parts[0], err)
				continue
			}
			if _, err := database.Exec(
				`INSERT INTO users (username, password_hash) VALUES (?, ?)
				 ON CONFLICT(username) DO UPDATE SET password_hash = excluded.password_hash`,
				parts[0], string(hash),
			); err != nil {
				log.Printf("seed user %q: %v", parts[0], err)
			}
		}
	}
}

