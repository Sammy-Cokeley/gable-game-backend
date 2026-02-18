# AGENTS.md — Gable Game (Codex Instructions)

## Mission
Help implement Gable Game V2 (platform refactor + in-season ranked pool mode) while preserving existing behavior.
Prefer small, reviewable commits and avoid breaking production.

## Repo Overview
- Frontend: Vue 3 + Quasar (hosted on Vercel)
- Backend: Go + Fiber (hosted on Render)
- Database: Postgres (Render)
- This repo may contain one or both; inspect root structure first and summarize architecture before changes.

## Operating Rules
1. Default to **Plan → Implement → Verify**.
2. Make **minimal, incremental edits**. Prefer adding new code paths over rewriting working ones.
3. Keep controllers thin; put business logic in services.
4. No breaking API changes unless explicitly specified in the spec.
5. Never commit secrets. Do not log tokens or personal data.
6. When touching DB schema, include migration + rollback (as supported by tooling).
7. Write tests for new logic when feasible; otherwise add lightweight validation steps.

## How to Work
### Step 1: Orient
- Identify frontend/backend directories.
- List key entrypoints (router, controllers, services, DB layer).
- Identify migration tooling (golang-migrate, sqlx, prisma, etc.) and follow existing conventions.

### Step 2: Make Changes
- Create new packages/modules for:
  - ModeProvider interface
  - InSeasonModeProvider
  - Ranking ingestion models/repository
- Avoid changing existing Daily mode behavior; refactor by extraction.

### Step 3: Verify
Run the repo’s standard checks. If unknown, search README/package scripts/Makefile.
Prefer these command patterns if they exist:
- Go backend:
  - `go test ./...`
  - `go test ./... -race` (if not too slow)
  - `golangci-lint run` (only if configured)
- Frontend:
  - `npm ci` / `pnpm i`
  - `npm run lint`
  - `npm run build`

If the project uses Docker Compose or Make targets, use those instead.

## Coding Conventions (Backend)
- Go: keep packages small, prefer interfaces at boundaries (service/repo).
- Error handling: wrap with context; return typed errors where appropriate.
- DB access: follow existing repository patterns; avoid raw SQL duplication.

## Coding Conventions (Frontend)
- Vue 3 `<script setup>` and Quasar patterns.
- Keep state in existing store pattern (Pinia/Vuex/etc.).
- Guest state: localStorage. Authed state: API-backed.

## Deliverables Expectations
For each task:
- Explain what files were changed and why (short).
- Include how to test locally.
- If you add endpoints, document request/response examples.

## Scope Guardrails
- Do not attempt full D1 data ingestion in this V2 task unless asked.
- Do not add scraping logic for proprietary sources without explicit instruction.
- Keep "ranked pool" ingestion adapter-based and swappable.
