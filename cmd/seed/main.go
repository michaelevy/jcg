package main

import (
	"flag"
	"fmt"
	"log"

	"golang.org/x/crypto/bcrypt"
	"jcg/internal/db"
)

func main() {
	dsn := flag.String("db", "file:./jcg.db?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000", "SQLite DSN")
	username := flag.String("u", "", "create/update a user account (requires -p)")
	password := flag.String("p", "", "password for the user (used with -u)")
	player := flag.String("player", "", "add a player by name")
	flag.Parse()

	if *username == "" && *player == "" {
		log.Fatal("provide either -u username -p password, or -player name")
	}

	database, err := db.Open(*dsn)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer database.Close()

	if *player != "" {
		_, err = database.Exec(`INSERT OR IGNORE INTO players (name) VALUES (?)`, *player)
		if err != nil {
			log.Fatalf("insert player: %v", err)
		}
		fmt.Printf("Player %q added (or already exists).\n", *player)
	}

	if *username != "" {
		if *password == "" {
			log.Fatal("-p password is required when using -u")
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(*password), bcrypt.DefaultCost)
		if err != nil {
			log.Fatalf("bcrypt: %v", err)
		}
		_, err = database.Exec(
			`INSERT OR REPLACE INTO users (username, password_hash) VALUES (?, ?)`,
			*username, string(hash),
		)
		if err != nil {
			log.Fatalf("insert user: %v", err)
		}
		fmt.Printf("User %q created/updated.\n", *username)
	}
}
