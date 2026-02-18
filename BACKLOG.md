# Gable Game — V2 Backlog

## Epic: Platform refactor (Modes)
- [ ] Introduce `game_modes` (or equivalent) table and seed Daily + InSeason
- [ ] Create ModeProvider interface and refactor Daily to use it
- [ ] Add InSeasonModeProvider (uses ranked_pool_member)

## Epic: Ranking ingestion + ranked pool
- [ ] Add ranking tables: ranking_source, ranking_snapshot, ranking_entry
- [ ] Implement ranked_pool_rule + ranked_pool_member computation
- [ ] Create admin/CLI script to import ranking snapshot from a CSV (source-agnostic)

## Epic: Canonical identity foundation
- [ ] Add season, school, wrestler_season tables (minimal fields)
- [ ] Ensure wrestler identity supports aliases (optional table: wrestler_alias)

## Epic: Frontend — In-season mode UX
- [ ] Mode selector UI (Daily vs In-Season)
- [ ] Store: separate state keys per mode/day/week
- [ ] Share results formatting per mode

## Epic: Post-NCAAs (results foundation)
- [ ] Add dual_meet + bout tables
- [ ] Repository/service scaffolding for results ingestion
