# Frontend Migration Guide

Tracks all backend changes that require corresponding updates in the Gable Game frontend.

---

## 1. API Route Prefix Change

All game API calls must be updated from `/api/` to `/api/gable/`.

| Old Route                       | New Route                              |
|---------------------------------|----------------------------------------|
| `GET /api/wrestlers`            | `GET /api/gable/wrestlers`             |
| `GET /api/wrestlers?name=...`   | `GET /api/gable/wrestlers?name=...`    |
| `GET /api/daily`                | `GET /api/gable/daily`                 |
| `GET /api/me`                   | `GET /api/gable/me`                    |
| `GET /api/user/guesses`         | `GET /api/gable/user/guesses`          |
| `GET /api/user/stats`           | `GET /api/gable/user/stats`            |
| `POST /api/register`            | `POST /api/gable/register`             |
| `POST /api/login`               | `POST /api/gable/login`                |
| `POST /api/verify-email`        | `POST /api/gable/verify-email`         |
| `POST /api/resend-verification` | `POST /api/gable/resend-verification`  |
| `POST /api/user/guess`          | `POST /api/gable/user/guess`           |
| `POST /api/user/stats`          | `POST /api/gable/user/stats`           |
| `POST /api/contact`             | `POST /api/gable/contact`              |

---

## 2. Wrestler Response — Data Source Change (No Shape Change)

The `/api/gable/wrestlers` and `/api/gable/daily` responses now read from the canonical
`core.*` schema instead of `wrestlers_2025`. The JSON shape is identical — no frontend
changes required for these fields.

```json
{
  "id": 42,
  "weight_class": "125",
  "name": "Pat McKee",
  "year": "SR",
  "team": "Minnesota",
  "conference": "Big Ten",
  "win_percentage": "0.8800",
  "ncaa_finish": "3rd"
}
```

---

## Notes

- Admin routes (`/api/admin/...`) are unchanged.
- The new platform API (`/api/v1/...`) is separate and not used by the game frontend.
