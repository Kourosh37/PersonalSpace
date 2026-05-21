package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"space/backend/internal/auth"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	var username string
	var password string
	flag.StringVar(&username, "username", "", "admin username")
	flag.StringVar(&password, "password", "", "admin password")
	flag.Parse()

	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		log.Fatal("username and password are required")
	}

	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		log.Fatal("DB_DSN is required")
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		log.Fatalf("hash password: %v", err)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	var exists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE username=$1)`, username).Scan(&exists); err != nil {
		log.Fatalf("query user existence: %v", err)
	}
	if exists {
		log.Fatalf("user already exists: %s", username)
	}

	id := uuid.NewString()
	now := time.Now().UTC()
	_, err = pool.Exec(ctx, `
		INSERT INTO users (id, username, password_hash, role, is_active, used_storage_bytes, created_at, updated_at)
		VALUES ($1,$2,$3,'admin',true,0,$4,$5)
	`, id, username, hash, now, now)
	if err != nil {
		log.Fatalf("insert user: %v", err)
	}

	fmt.Printf("Admin user created: %s\n", username)
}