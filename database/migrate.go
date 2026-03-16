package database

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RunMigrations applies any SQL migration files in the migrations/ directory
// that have not yet been recorded in the schema_migrations table.
// Files must be named NNN_description.sql and are applied in numeric order.
func RunMigrations() {
	ensureMigrationsTable()

	dir := migrationsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Fatalf("migrations: cannot read directory %s: %v", dir, err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, name := range files {
		if isApplied(name) {
			continue
		}
		applyMigration(filepath.Join(dir, name), name)
	}
}

func ensureMigrationsTable() {
	_, err := DB.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename   TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	if err != nil {
		log.Fatalf("migrations: failed to create schema_migrations table: %v", err)
	}
}

func isApplied(filename string) bool {
	var count int
	err := DB.QueryRow(
		`SELECT COUNT(*) FROM schema_migrations WHERE filename = $1`, filename,
	).Scan(&count)
	if err != nil {
		log.Fatalf("migrations: failed to check migration status for %s: %v", filename, err)
	}
	return count > 0
}

func applyMigration(path, filename string) {
	sql, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("migrations: failed to read %s: %v", path, err)
	}

	tx, err := DB.Begin()
	if err != nil {
		log.Fatalf("migrations: failed to begin transaction for %s: %v", filename, err)
	}

	if _, err := tx.Exec(string(sql)); err != nil {
		tx.Rollback()
		log.Fatalf("migrations: failed to apply %s: %v", filename, err)
	}

	if _, err := tx.Exec(
		`INSERT INTO schema_migrations (filename) VALUES ($1)`, filename,
	); err != nil {
		tx.Rollback()
		log.Fatalf("migrations: failed to record %s: %v", filename, err)
	}

	if err := tx.Commit(); err != nil {
		log.Fatalf("migrations: failed to commit %s: %v", filename, err)
	}

	fmt.Printf("migrations: applied %s\n", filename)
}

// migrationsDir returns the path to the migrations folder relative to the
// working directory. On Render (and in local dev), the binary runs from the
// project root, so this resolves to ./database/migrations.
func migrationsDir() string {
	return filepath.Join("database", "migrations")
}
