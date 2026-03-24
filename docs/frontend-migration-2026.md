# Frontend Migration Notes — 2026 Season

## Overview

The backend has been updated to serve 2026 NCAA qualifier data sourced from WrestleStat. The legacy `wrestlers_2025` table is no longer used by the game. All wrestler data now comes from `core.wrestler_season` for season year 2026.

---

## Wrestler ID

**Changed.** The `id` field on wrestler objects is now the WrestleStat integer ID, not the legacy `wrestlers_2025` integer ID.

- Old: legacy DB auto-increment INT (e.g. `142`)
- New: WrestleStat ID INT (e.g. `78062`)

This affects:
- `GET /api/gable/wrestlers` — each wrestler's `id` is now the WrestleStat ID
- `POST /api/gable/user/guess` — `wrestler_id` in the request body must be the WrestleStat ID
- `GET /api/gable/user/guesses` — `wrestler_id` in returned guesses is the WrestleStat ID

---

## Win Percentage

**Changed.** `win_percentage` is now returned as a 0–100 percentage string, not a 0–1 decimal.

| | Value | Display |
|---|---|---|
| Old | `"0.850"` | needed `* 100` |
| New | `"85.0000..."` | use as-is, truncate to 1 decimal |

Recommended display: `parseFloat(win_percentage).toFixed(1) + "%"`

---

## NCAA Finish Values

**Unchanged.** Values are the same as prior seasons.

| Value | Meaning |
|-------|---------|
| `1st` | NCAA Champion |
| `2nd` | Runner-up |
| `3rd` | Third place |
| `4th` | Fourth place |
| `5th` | Fifth place (All-American) |
| `6th` | Sixth place (All-American) |
| `7th` | Seventh place (All-American) |
| `8th` | Eighth place (All-American) |
| `R12` | Lost in round of 12 (did not All-American) |
| `R16` | Lost in round of 16 (did not All-American) |
| `NQ`  | National Qualifier |

---

## API Endpoints — No URL Changes

All existing route paths are unchanged.

| Endpoint | Notes |
|----------|-------|
| `GET /api/gable/wrestlers` | Returns 330 wrestlers. `id`, `win_percentage`, and `ncaa_finish` format changed (see above). |
| `GET /api/gable/wrestlers?name=X` | Unchanged behavior, updated data. |
| `GET /api/gable/daily` | Returns today's wrestler from the 2026 schedule (seeded March 23, 2026). |
| `POST /api/gable/user/guess` | `wrestler_id` must now be the WrestleStat ID. |
| `GET /api/gable/user/guesses` | Returns wrestler attributes for each guess — same shape, updated values. |

---

## Wrestler Response Shape

No field names changed. All existing JSON keys are the same.

```json
{
  "id": 78062,
  "weight_class": "125",
  "name": "Luke Lilledahl",
  "year": "SO",
  "team": "Penn State",
  "conference": "Big Ten",
  "win_percentage": "100.0000000000000000",
  "ncaa_finish": "1st"
}
```

---

## Daily Schedule

330 wrestlers are scheduled one per day starting **March 23, 2026** through approximately **January 17, 2027**. The schedule is fixed and randomized with a deterministic seed.
