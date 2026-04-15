package main

import (
	"embed"
	"flag"
	"html/template"
	"io/fs"
	"log"
	"net/http"

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

	// ParseFS loads templates: "head", "nav", "home" are reserved names; future templates must avoid naming collisions.
	tmpl := template.Must(template.ParseFS(templateFS, "templates/*.html"))

	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatal(err)
	}

	h := handlers.New(database, tmpl)

	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))
	mux.HandleFunc("GET /{$}", h.Home)

	mux.HandleFunc("GET /login", h.LoginPage)
	// TODO: add CSRF token protection before production deployment
	mux.HandleFunc("POST /login", h.LoginSubmit)
	// TODO: add CSRF token protection before production deployment
	mux.HandleFunc("POST /logout", h.Logout)

	mux.Handle("GET /enter", middleware.RequireAuth(http.HandlerFunc(h.EntryPage)))
	mux.Handle("POST /enter", middleware.RequireAuth(http.HandlerFunc(h.EntrySubmit)))
	mux.Handle("POST /enter/season", middleware.RequireAuth(http.HandlerFunc(h.CreateSeason)))

	log.Printf("listening on %s", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}
