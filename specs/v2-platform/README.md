# Gable Game V2 — Platform Refactor + In-Season Mode (Ranked Pool)

## Context
Gable Game is currently a daily guessing game (Vue/Quasar frontend, Go/Fiber backend, Postgres).
We are planning a V2 rollout after the NCAA tournament. V2 introduces pluggable game modes and
an in-season mode driven by a "ranked pool" (weekly rankings snapshots), while simultaneously
laying the foundation for a long-term wrestling data product (wrestlers, schools, seasons, bouts, duals).

## Goals
1. Convert backend from "single mode" into a platform that supports multiple game modes:
   - Daily (existing)
   - In-Season (new)
   - Future: Archive, Tournament, etc.
2. Implement In-Season mode using a computed "ranked pool" derived from one or more ranking sources.
3. Establish canonical data model for long-term expansion:
   - Wrestler / School / Season identity
   - Match/bout results and dual meets
4. Preserve stability during peak season: avoid risky changes until post-NCAAs.

## Non-Goals (for this phase)
- Full D1 roster ingestion (beyond what is needed for ranked pool)
- Video analytics
- Elo/advanced rating system
- Bracket modeling / tournament simulations

## In-Season Mode Definition
- "Ranked" means a wrestler included in a ranked pool derived from weekly snapshots.
- Ranked pool rule should be configurable:
  - examples: top 33 in >= 1 source; top 33 in >= 2 sources; consensus by best rank/avg rank
- Default behavior (V2 initial): current-week pool (not rolling season), with optional rolling later.

## Data Model (V2 additions)
### Ranking ingestion
- ranking_source(id, name, notes, is_active)
- ranking_snapshot(id, season_id, ranking_date OR week, source_id, weight_class)
- ranking_entry(snapshot_id, wrestler_id, rank, points?, metadata jsonb)
- ranked_pool_rule(season_id, mode_id, rule_config jsonb)
- ranked_pool_member(season_id, ranking_date/week, weight_class, wrestler_id, eligibility_score?, included_reason jsonb)

### Canonical identity
- season(id, label, start_date, end_date)
- school(id, name, conference?)
- wrestler(id, full_name, slug, created_at, ...)
- wrestler_season(wrestler_id, season_id, school_id, weight_class, class_year?, metadata)

### Results foundation (post-V2, but schema-ready)
- dual_meet(id, season_id, date, home_school_id, away_school_id, score_home, score_away, ...)
- bout(id, dual_meet_id?, event_id?, weight_class, red_wrestler_id, blue_wrestler_id, winner_id, win_type, score_red, score_blue, ...)

## API / Service Architecture Requirements
- Controllers must be thin (validate request, call service, return JSON).
- Core logic lives in services:
  - GameEngine (state, attempt limits, evaluation)
  - ComparisonEngine (attribute comparisons & feedback)
  - ModeProvider interface (DailyModeProvider, InSeasonModeProvider)
- Mode selection is DB-driven (game_modes table or equivalent).

## Acceptance Criteria
- Existing Daily mode remains functional (guest + authed).
- New In-Season mode endpoint(s) exists and can:
  - choose a target wrestler from ranked_pool_member for a given ranking_date/week
  - evaluate guesses using the same comparison engine
  - persist state per user (db for authed, localStorage for guests in frontend)
- Ranked pool rule is configurable without code changes (jsonb rule_config).
- Migrations are deterministic and reversible (where your migration system allows).

## Rollout Plan
- During NCAAs: bugfixes only, no risky schema refactors.
- Post-NCAAs: merge V2 schema + services behind feature flag; release in-season mode after validation.

## Notes on Data Sources
Ranking sources may be proprietary. Do not hardcode assumptions about scraping.
System must support "source adapters" and allow replacing inputs without changing the game logic.