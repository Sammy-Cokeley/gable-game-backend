package controllers

import (
	"database/sql"
	"log"
	"strconv"
	"strings"

	"gable-backend/database"

	"github.com/gofiber/fiber/v2"
)

// ---------------------------------------------------------------------------
// Response types
// ---------------------------------------------------------------------------

type WrestlerSeasonSummary struct {
	SeasonYear    int     `json:"season_year"`
	SeasonLabel   string  `json:"season_label"`
	School        string  `json:"school"`
	SchoolSlug    string  `json:"school_slug"`
	Conference    string  `json:"conference"`
	ConferenceSlug string `json:"conference_slug"`
	WeightClass   string  `json:"weight_class"`
	ClassYear     string  `json:"class_year"`
	RecordWins    *int    `json:"record_wins"`
	RecordLosses  *int    `json:"record_losses"`
	WinPct        *string `json:"win_percentage"`
	NcaaFinish    *string `json:"ncaa_finish"`
}

type WrestlerListItem struct {
	ID            string                `json:"id"`
	FullName      string                `json:"full_name"`
	Slug          string                `json:"slug"`
	WrestlestatID *string               `json:"wrestlestat_id"`
	Season        WrestlerSeasonSummary `json:"season"`
}

type WrestlerProfile struct {
	ID            string                  `json:"id"`
	FullName      string                  `json:"full_name"`
	Slug          string                  `json:"slug"`
	WrestlestatID *string                 `json:"wrestlestat_id"`
	Seasons       []WrestlerSeasonSummary `json:"seasons"`
}

type PaginatedWrestlers struct {
	Data       []WrestlerListItem `json:"data"`
	Page       int                `json:"page"`
	PerPage    int                `json:"per_page"`
	Total      int                `json:"total"`
}

type SchoolListItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	ShortName string `json:"short_name"`
}

type SchoolProfile struct {
	ID         string                   `json:"id"`
	Name       string                   `json:"name"`
	Slug       string                   `json:"slug"`
	ShortName  string                   `json:"short_name"`
	Conferences []ConferenceSeasonEntry `json:"conferences"`
	Roster     []WrestlerListItem       `json:"roster"`
}

type ConferenceSeasonEntry struct {
	ConferenceID   string `json:"conference_id"`
	ConferenceName string `json:"conference_name"`
	ConferenceSlug string `json:"conference_slug"`
	SeasonYear     int    `json:"season_year"`
	SeasonLabel    string `json:"season_label"`
}

type ConferenceListItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type SeasonListItem struct {
	ID        string  `json:"id"`
	Year      int     `json:"year"`
	Label     string  `json:"label"`
	StartDate *string `json:"start_date"`
	EndDate   *string `json:"end_date"`
}

// ---------------------------------------------------------------------------
// GET /api/v1/wrestlers
// Query params: season (year int), weight_class, school (slug), conference (slug),
//               page (default 1), per_page (default 50, max 200)
// ---------------------------------------------------------------------------
func V1GetWrestlers(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	perPage, _ := strconv.Atoi(c.Query("per_page", "50"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 200 {
		perPage = 50
	}

	args := []interface{}{}
	where := []string{}
	i := 1

	if s := c.Query("season"); s != "" {
		args = append(args, s)
		where = append(where, "se.year = $"+strconv.Itoa(i))
		i++
	}
	if wc := c.Query("weight_class"); wc != "" {
		args = append(args, wc)
		where = append(where, "wc.label = $"+strconv.Itoa(i))
		i++
	}
	if school := c.Query("school"); school != "" {
		args = append(args, school)
		where = append(where, "sc.slug = $"+strconv.Itoa(i))
		i++
	}
	if conf := c.Query("conference"); conf != "" {
		args = append(args, conf)
		where = append(where, "co.slug = $"+strconv.Itoa(i))
		i++
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	// Total count
	countQuery := `
		SELECT COUNT(*)
		FROM core.wrestler w
		JOIN core.wrestler_season ws ON ws.wrestler_id = w.id
		JOIN core.season se ON se.id = ws.season_id
		JOIN core.school sc ON sc.id = ws.school_id
		JOIN core.weight_class wc ON wc.id = ws.primary_weight_class_id
		LEFT JOIN core.school_conference_season scs ON scs.school_id = sc.id AND scs.season_id = se.id
		LEFT JOIN core.conference co ON co.id = scs.conference_id
		` + whereClause

	var total int
	if err := database.DB.QueryRow(countQuery, args...).Scan(&total); err != nil {
		log.Printf("platform_wrestlers error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
	}

	// Paginated results
	offset := (page - 1) * perPage
	args = append(args, perPage, offset)

	dataQuery := `
		SELECT
			w.id, w.full_name, w.slug, w.wrestlestat_id::TEXT,
			se.year, se.label,
			sc.name, sc.slug,
			COALESCE(co.name, ''), COALESCE(co.slug, ''),
			wc.label,
			COALESCE(ws.class_year, ''),
			ws.wins, ws.losses,
			ws.win_percentage::TEXT, ws.ncaa_finish
		FROM core.wrestler w
		JOIN core.wrestler_season ws ON ws.wrestler_id = w.id
		JOIN core.season se ON se.id = ws.season_id
		JOIN core.school sc ON sc.id = ws.school_id
		JOIN core.weight_class wc ON wc.id = ws.primary_weight_class_id
		LEFT JOIN core.school_conference_season scs ON scs.school_id = sc.id AND scs.season_id = se.id
		LEFT JOIN core.conference co ON co.id = scs.conference_id
		` + whereClause + `
		ORDER BY wc.sort_order, w.full_name
		LIMIT $` + strconv.Itoa(i) + ` OFFSET $` + strconv.Itoa(i+1)

	rows, err := database.DB.Query(dataQuery, args...)
	if err != nil {
		log.Printf("platform_wrestlers error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
	}
	defer rows.Close()

	wrestlers := []WrestlerListItem{}
	for rows.Next() {
		var item WrestlerListItem
		var s WrestlerSeasonSummary
		var winPct sql.NullString
		var ncaaFinish sql.NullString
		if err := rows.Scan(
			&item.ID, &item.FullName, &item.Slug, &item.WrestlestatID,
			&s.SeasonYear, &s.SeasonLabel,
			&s.School, &s.SchoolSlug,
			&s.Conference, &s.ConferenceSlug,
			&s.WeightClass, &s.ClassYear,
			&s.RecordWins, &s.RecordLosses,
			&winPct, &ncaaFinish,
		); err != nil {
			log.Printf("platform_wrestlers error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
		}
		if winPct.Valid {
			s.WinPct = &winPct.String
		}
		if ncaaFinish.Valid {
			s.NcaaFinish = &ncaaFinish.String
		}
		item.Season = s
		wrestlers = append(wrestlers, item)
	}

	return c.JSON(PaginatedWrestlers{
		Data:    wrestlers,
		Page:    page,
		PerPage: perPage,
		Total:   total,
	})
}

// ---------------------------------------------------------------------------
// GET /api/v1/wrestlers/:slug
// ---------------------------------------------------------------------------
func V1GetWrestler(c *fiber.Ctx) error {
	id := c.Params("id")

	// Basic identity
	var profile WrestlerProfile
	err := database.DB.QueryRow(`
		SELECT id, full_name, slug, wrestlestat_id
		FROM core.wrestler WHERE id = $1
	`, id).Scan(&profile.ID, &profile.FullName, &profile.Slug, &profile.WrestlestatID)
	if err == sql.ErrNoRows {
		return c.Status(404).JSON(fiber.Map{"error": "wrestler not found"})
	}
	if err != nil {
		log.Printf("platform_wrestlers error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
	}

	// All seasons
	rows, err := database.DB.Query(`
		SELECT
			se.year, se.label,
			sc.name, sc.slug,
			COALESCE(co.name, ''), COALESCE(co.slug, ''),
			wc.label,
			COALESCE(ws.class_year, ''),
			ws.wins, ws.losses,
			ws.win_percentage::TEXT, ws.ncaa_finish
		FROM core.wrestler_season ws
		JOIN core.season se ON se.id = ws.season_id
		JOIN core.school sc ON sc.id = ws.school_id
		JOIN core.weight_class wc ON wc.id = ws.primary_weight_class_id
		LEFT JOIN core.school_conference_season scs ON scs.school_id = sc.id AND scs.season_id = se.id
		LEFT JOIN core.conference co ON co.id = scs.conference_id
		WHERE ws.wrestler_id = $1
		ORDER BY se.year DESC
	`, profile.ID)
	if err != nil {
		log.Printf("platform_wrestlers error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
	}
	defer rows.Close()

	profile.Seasons = []WrestlerSeasonSummary{}
	for rows.Next() {
		var s WrestlerSeasonSummary
		var winPct sql.NullString
		var ncaaFinish sql.NullString
		if err := rows.Scan(
			&s.SeasonYear, &s.SeasonLabel,
			&s.School, &s.SchoolSlug,
			&s.Conference, &s.ConferenceSlug,
			&s.WeightClass, &s.ClassYear,
			&s.RecordWins, &s.RecordLosses,
			&winPct, &ncaaFinish,
		); err != nil {
			log.Printf("platform_wrestlers error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
		}
		if winPct.Valid {
			s.WinPct = &winPct.String
		}
		if ncaaFinish.Valid {
			s.NcaaFinish = &ncaaFinish.String
		}
		profile.Seasons = append(profile.Seasons, s)
	}

	return c.JSON(profile)
}

// ---------------------------------------------------------------------------
// GET /api/v1/schools
// ---------------------------------------------------------------------------
func V1GetSchools(c *fiber.Ctx) error {
	rows, err := database.DB.Query(`
		SELECT id, name, slug, COALESCE(short_name, '')
		FROM core.school
		ORDER BY name
	`)
	if err != nil {
		log.Printf("platform_wrestlers error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
	}
	defer rows.Close()

	schools := []SchoolListItem{}
	for rows.Next() {
		var s SchoolListItem
		if err := rows.Scan(&s.ID, &s.Name, &s.Slug, &s.ShortName); err != nil {
			log.Printf("platform_wrestlers error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
		}
		schools = append(schools, s)
	}
	return c.JSON(schools)
}

// ---------------------------------------------------------------------------
// GET /api/v1/schools/:slug
// Query params: season (year int) — filters roster to that season
// ---------------------------------------------------------------------------
func V1GetSchool(c *fiber.Ctx) error {
	slug := c.Params("slug")

	var school SchoolProfile
	err := database.DB.QueryRow(`
		SELECT id, name, slug, COALESCE(short_name, '')
		FROM core.school WHERE slug = $1
	`, slug).Scan(&school.ID, &school.Name, &school.Slug, &school.ShortName)
	if err == sql.ErrNoRows {
		return c.Status(404).JSON(fiber.Map{"error": "school not found"})
	}
	if err != nil {
		log.Printf("platform_wrestlers error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
	}

	// Conference history
	confRows, err := database.DB.Query(`
		SELECT co.id, co.name, co.slug, se.year, se.label
		FROM core.school_conference_season scs
		JOIN core.conference co ON co.id = scs.conference_id
		JOIN core.season se ON se.id = scs.season_id
		WHERE scs.school_id = $1
		ORDER BY se.year DESC
	`, school.ID)
	if err != nil {
		log.Printf("platform_wrestlers error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
	}
	defer confRows.Close()

	school.Conferences = []ConferenceSeasonEntry{}
	for confRows.Next() {
		var e ConferenceSeasonEntry
		if err := confRows.Scan(&e.ConferenceID, &e.ConferenceName, &e.ConferenceSlug, &e.SeasonYear, &e.SeasonLabel); err != nil {
			log.Printf("platform_wrestlers error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
		}
		school.Conferences = append(school.Conferences, e)
	}

	// Roster — optional season filter
	rosterArgs := []interface{}{school.ID}
	seasonFilter := ""
	if s := c.Query("season"); s != "" {
		rosterArgs = append(rosterArgs, s)
		seasonFilter = "AND se.year = $2"
	}

	rosterRows, err := database.DB.Query(`
		SELECT
			w.id, w.full_name, w.slug, w.wrestlestat_id::TEXT,
			se.year, se.label,
			sc.name, sc.slug,
			COALESCE(co.name, ''), COALESCE(co.slug, ''),
			wc.label,
			COALESCE(ws.class_year, ''),
			ws.wins, ws.losses,
			ws.win_percentage::TEXT, ws.ncaa_finish
		FROM core.wrestler_season ws
		JOIN core.wrestler w ON w.id = ws.wrestler_id
		JOIN core.season se ON se.id = ws.season_id
		JOIN core.school sc ON sc.id = ws.school_id
		JOIN core.weight_class wc ON wc.id = ws.primary_weight_class_id
		LEFT JOIN core.school_conference_season scs ON scs.school_id = sc.id AND scs.season_id = se.id
		LEFT JOIN core.conference co ON co.id = scs.conference_id
		WHERE ws.school_id = $1 `+seasonFilter+`
		ORDER BY wc.sort_order, w.full_name
	`, rosterArgs...)
	if err != nil {
		log.Printf("platform_wrestlers error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
	}
	defer rosterRows.Close()

	school.Roster = []WrestlerListItem{}
	for rosterRows.Next() {
		var item WrestlerListItem
		var s WrestlerSeasonSummary
		var winPct sql.NullString
		var ncaaFinish sql.NullString
		if err := rosterRows.Scan(
			&item.ID, &item.FullName, &item.Slug, &item.WrestlestatID,
			&s.SeasonYear, &s.SeasonLabel,
			&s.School, &s.SchoolSlug,
			&s.Conference, &s.ConferenceSlug,
			&s.WeightClass, &s.ClassYear,
			&s.RecordWins, &s.RecordLosses,
			&winPct, &ncaaFinish,
		); err != nil {
			log.Printf("platform_wrestlers error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
		}
		if winPct.Valid {
			s.WinPct = &winPct.String
		}
		if ncaaFinish.Valid {
			s.NcaaFinish = &ncaaFinish.String
		}
		item.Season = s
		school.Roster = append(school.Roster, item)
	}

	return c.JSON(school)
}

// ---------------------------------------------------------------------------
// GET /api/v1/conferences
// ---------------------------------------------------------------------------
func V1GetConferences(c *fiber.Ctx) error {
	rows, err := database.DB.Query(`
		SELECT id, name, slug FROM core.conference ORDER BY name
	`)
	if err != nil {
		log.Printf("platform_wrestlers error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
	}
	defer rows.Close()

	conferences := []ConferenceListItem{}
	for rows.Next() {
		var conf ConferenceListItem
		if err := rows.Scan(&conf.ID, &conf.Name, &conf.Slug); err != nil {
			log.Printf("platform_wrestlers error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
		}
		conferences = append(conferences, conf)
	}
	return c.JSON(conferences)
}

// ---------------------------------------------------------------------------
// GET /api/v1/seasons
// ---------------------------------------------------------------------------
func V1GetSeasons(c *fiber.Ctx) error {
	rows, err := database.DB.Query(`
		SELECT id, year, label, start_date::TEXT, end_date::TEXT
		FROM core.season ORDER BY year DESC
	`)
	if err != nil {
		log.Printf("platform_wrestlers error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
	}
	defer rows.Close()

	seasons := []SeasonListItem{}
	for rows.Next() {
		var s SeasonListItem
		var start, end sql.NullString
		if err := rows.Scan(&s.ID, &s.Year, &s.Label, &start, &end); err != nil {
			log.Printf("platform_wrestlers error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "internal server error"})
		}
		if start.Valid {
			s.StartDate = &start.String
		}
		if end.Valid {
			s.EndDate = &end.String
		}
		seasons = append(seasons, s)
	}
	return c.JSON(seasons)
}
