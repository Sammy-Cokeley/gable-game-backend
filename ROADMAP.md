# College Wrestling Database Platform — Roadmap

## Vision

Transform `gable-backend` from a single-product game server into a **canonical College Wrestling Database Platform** — a general-purpose backend that stores, ingests, and serves authoritative wrestling data to power any data-backed project:

- **Gable Game** (existing daily guessing game)
- **Public stats & wrestler profiles**
- **Rankings tracker** (week-over-week movement)
- **Match/bout results database**
- **Third-party API** (future)

The platform follows a **monolithic architecture** (single Go/Fiber service, single PostgreSQL database), with clean internal separation between:
- `platform/` — canonical data layer (entities, ingestion, services)
- `game/` — game modes, user sessions, guesses
- `admin/` — data curation and ingestion tooling

---

## Current State (Baseline)

| Layer | Status |
|---|---|
| Daily guessing game (guest + authed) | ✅ Production |
| JWT auth + email verification | ✅ Production |
| WrestleStat scraper + enrichment | ✅ Production |
| Rankings ingestion admin UI | ✅ Production |
| Canonical schema design (V2 spec) | 📄 Designed, not implemented |
| In-Season game mode | 📄 Designed, not implemented |
| Match/bout results | ❌ Not started |
| Public platform API | ❌ Not started |

---

## Data Model Target

The platform's canonical schema uses the `core` PostgreSQL schema namespace and UUID primary keys. The full design is documented in [`specs/v2-platform/README.md`](specs/v2-platform/README.md).

### Core Entities

```sql
core.season               -- (id, year, label, start_date, end_date)
core.school               -- (id, name, slug, short_name)
core.conference           -- (id, name, slug)
core.school_conference_season -- (school_id, conference_id, season_id)
core.weight_class         -- (id, label, pounds, sort_order)
core.wrestler             -- (id, full_name, slug, wrestlestat_id, created_at)
core.wrestler_alias       -- (wrestler_id, alias, source)
core.wrestler_season      -- (wrestler_id, season_id, school_id, weight_class_id, class_year, record_wins, record_losses, metadata jsonb)
core.event                -- (id, season_id, name, type, location, start_date, end_date)
core.bout                 -- (id, event_id?, dual_meet_id?, weight_class_id, red_wrestler_id, blue_wrestler_id, winner_id, win_type, score_red, score_blue, period)
core.dual_meet            -- (id, season_id, date, home_school_id, away_school_id, score_home, score_away)
core.ingest_batch         -- (id, source, season_id, status, started_at, completed_at, stats jsonb)
core.ingest_error         -- (id, batch_id, entity_type, raw_data jsonb, error_message, created_at)
core.legacy_wrestler_map  -- (legacy_id, wrestler_id)  -- maps wrestlers_2025 → core.wrestler
```

### Ranking Schema

```sql
ranking_source            -- (id, name, slug, is_active, notes)
ranking_snapshot          -- (id, source_id, season_id, weight_class_id, snapshot_date, status)
ranking_entry             -- (id, snapshot_id, wrestler_id, rank, previous_rank, metadata jsonb)
ranked_pool_rule          -- (id, season_id, rule_config jsonb)  -- configurable pool composition
ranked_pool_member        -- (id, season_id, snapshot_date, weight_class_id, wrestler_id, reason jsonb)
```

---

## Phased Roadmap

### Phase 1 — Foundation: Canonical Schema Migration
**Goal:** Establish the canonical data model without breaking production.

- [ ] Create `core` schema migrations (season, school, conference, wrestler, wrestler_season, weight_class)
- [ ] Write migration script to populate `core.*` from `wrestlers_2025` (via `core.legacy_wrestler_map`)
- [ ] Migrate ranking system: connect existing `rankings_release_entries` → `core.ranking_*` tables
- [ ] Keep `wrestlers_2025` + `daily_wrestlers` fully functional (game reads from legacy until Phase 3)
- [ ] Add `core.ingest_batch` + `core.ingest_error` tables for observability

**Deliverable:** Dual-write state — legacy game reads from old tables, new data lands in canonical schema.

---

### Phase 2 — Platform API: Wrestlers, Schools, Rankings
**Goal:** Expose canonical data via a clean, versioned REST API.

- [ ] Add `/api/v1/` route namespace
- [ ] `GET /api/v1/wrestlers` — paginated, filterable (season, weight class, school, conference)
- [ ] `GET /api/v1/wrestlers/:slug` — full wrestler profile (career seasons, rankings history)
- [ ] `GET /api/v1/schools` — list all schools
- [ ] `GET /api/v1/schools/:slug` — school profile (roster, conference history)
- [ ] `GET /api/v1/conferences` — list conferences
- [ ] `GET /api/v1/seasons` — list seasons
- [ ] `GET /api/v1/rankings` — query rankings by season + source + week
- [ ] `GET /api/v1/rankings/history/:wrestler_slug` — ranking history for a wrestler

**Access model:** Read-only, no auth required. Write endpoints remain admin-only.

---

### Phase 3 — Game Layer Upgrade (V2 In-Season Mode)
**Goal:** Migrate game logic onto canonical schema; add In-Season mode.

- [ ] Service layer: `GameEngine`, `ComparisonEngine`, `ModeProvider` interface (per V2 spec)
- [ ] `DailyModeProvider` rewritten to use `core.wrestler` + `core.wrestler_season`
- [ ] `InSeasonModeProvider` driven by `ranked_pool_member` (weekly ranked pool)
- [ ] Configurable ranked pool rules stored in `ranked_pool_rule` (no code changes to adjust pool)
- [ ] Retire reads from `wrestlers_2025` (game now fully on canonical schema)
- [ ] Game mode selection is DB-driven (`game_modes` table with `is_active` flag)

**Acceptance criteria:**
- Daily mode remains stable (guest + authed)
- In-Season mode can select a target from the ranked pool and evaluate guesses
- State persisted per user in DB (authed) or handed off to frontend (guest)

---

### Phase 4 — Match & Results Layer
**Goal:** Store dual meet and bout results; expose via API.

- [ ] `core.dual_meet` + `core.bout` migrations
- [ ] Admin ingestion UI for dual meet results (CSV import + manual entry)
- [ ] `GET /api/v1/results/duals` — query dual meet results by season/school
- [ ] `GET /api/v1/results/bouts` — individual bout search (by wrestler, event, weight class)
- [ ] `GET /api/v1/wrestlers/:slug/results` — full bout history for a wrestler
- [ ] Event support: `core.event` for tournaments (NCAAs, conference tournaments, invitationals)

---

### Phase 5 — Stats Aggregation & Search
**Goal:** Derive and serve useful stats from the results layer.

- [ ] Win/loss record aggregations per wrestler per season (materialized or computed)
- [ ] Head-to-head records
- [ ] Full-text search across wrestlers, schools, events
- [ ] `GET /api/v1/stats/wrestler/:slug` — aggregated season stats
- [ ] `GET /api/v1/search?q=...` — unified search across entities

---

### Phase 6 — Public Access Layer (Future)
**Goal:** Open the platform API for external use.

- [ ] API key registration + management
- [ ] Rate limiting per key
- [ ] Public documentation (OpenAPI / Swagger)
- [ ] Usage analytics

---

## Non-Goals (This Roadmap)

- Video analytics
- Elo / advanced rating systems
- Bracket modeling / tournament simulations
- Full D1 roster ingestion (beyond ranked pool needs)
- Mobile app

---

## Architecture Principles

1. **Monolith, clean layers** — one service, but controllers stay thin; logic lives in services.
2. **Canonical schema first** — all new features build on `core.*`; legacy tables are deprecated, not deleted.
3. **Ingestion is observable** — every import goes through `core.ingest_batch`; errors logged in `core.ingest_error`.
4. **Configurable, not hardcoded** — pool rules, ranking sources, game modes are DB-driven.
5. **Internal-first, public-ready** — API is internal now; designed for public exposure without breaking changes.

---

## Data Sources

| Source | Current Status | Used For |
|---|---|---|
| WrestleStat | ✅ Integrated (scraper + fuzzy match) | Wrestler enrichment, profile lookup |
| Manual CSV import | ✅ Rankings admin UI | Weekly rankings ingestion |
| NCAA.com | ❌ Not integrated | Future: official results, brackets |
| FloWrestling / TrackWrestling | ❌ Not integrated | Future: additional ranking sources |

---

## Key Files

| File | Purpose |
|---|---|
| [`specs/v2-platform/README.md`](specs/v2-platform/README.md) | Existing V2 design spec (game modes, service architecture) |
| [`controllers/rankings_admin_controller.go`](controllers/rankings_admin_controller.go) | Admin ranking ingestion |
| [`controllers/wrestlestat_admin_controller.go`](controllers/wrestlestat_admin_controller.go) | WrestleStat enrichment |
| [`database/migrations/`](database/migrations/) | SQL migration files |
| [`models/wrestler.go`](models/wrestler.go) | Legacy wrestler model (to be superseded by canonical) |
