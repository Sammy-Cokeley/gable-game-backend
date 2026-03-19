# gable-backend — Project CLAUDE.md

## Project Overview

**Stack:** Go 1.24, PostgreSQL, Fiber v2 (HTTP), Docker, JWT auth, SendGrid (email)

**Architecture:** Flat package layout — controllers handle HTTP logic, models define data types, routes register endpoints, middleware handles auth/CORS, database handles migrations and DB connection.

## File Structure

```
main.go                        # Entry point — DB connect, migrations, Fiber setup, routes
controllers/                   # HTTP handlers (one file per domain)
  wrestler_controller.go
  rankings_admin_controller.go
  user_controller.go
  platform_wrestlers.go
  platform_rankings.go
  contact.go
routes/
  wrestlers.go                 # Route registration
  platform.go
models/                        # Data types and DB query structs
middleware/                    # JWT auth middleware
database/                      # DB connection and migration runner
  migrations/                  # Plain SQL migration files (001_, 002_, ...)
config/                        # App config loading
mail/                          # SendGrid email helpers
```

## Critical Rules

### Go Conventions

- Follow Effective Go — prefer early returns over deep nesting
- Use `fmt.Errorf("context: %w", err)` for error wrapping — never return bare `err` without context
- `context.Context` must be the first parameter for any function that calls the DB or external services
- No global mutable state — pass dependencies via function parameters or constructors
- All SQL queries must use parameterized placeholders (`$1`, `$2`) — never string formatting

### Database

- Migrations live in `database/migrations/` as plain SQL files — named `NNN_description.sql`
- Never alter the database directly outside of a migration file
- Use transactions for multi-step writes

### Error Handling

- Return errors, never panic for recoverable situations
- HTTP handlers should map domain errors to appropriate Fiber status codes
- Log errors with context before returning HTTP 500

### Code Style

- No emojis in code or comments
- Keep functions under 50 lines — extract helpers
- Use table-driven tests for logic with multiple cases

## Key Commands

```bash
# Build and run
go build ./...
go run main.go

# Tests
go test ./...
go test ./... -race
go vet ./...

# Specific package
go test ./controllers/... -v
```

## ECC Workflow

```bash
/go-review           # Review Go code for idioms, error handling, security
/go-test             # TDD workflow for Go
/go-build            # Fix build errors
/plan "feature"      # Plan before implementing features
/code-review         # General code review
/refactor-clean      # Remove dead code
```

## Environment Variables

```bash
PORT=3000
DATABASE_URL=postgres://user:pass@localhost:5432/gable?sslmode=disable
JWT_SECRET=
SENDGRID_API_KEY=
RENDER=               # Set to any value in production (skips .env load)
```

## Git Workflow

- `feat:` new features, `fix:` bug fixes, `refactor:` code changes, `chore:` maintenance
- Feature branches from `main`
- SQL migrations: always add a new numbered file, never edit existing ones
