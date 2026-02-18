package controllers

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"gable-backend/database"

	"github.com/PuerkitoBio/goquery"
	"github.com/gofiber/fiber/v2"
)

type WrestleStatCandidate struct {
	WrestleStatID int     `json:"wrestlestatId"`
	Name          string  `json:"name"`   // "First Last"
	School        string  `json:"school"` // "NC State"
	Score         float64 `json:"score"`
}

type parsedProfile struct {
	WrestleStatID int
	Name          string
	Team          string
	ClassYear     string // FR/SO/JR/SR or RSFR/RSO/RSJR/RSSR if found
	Wins          int
	Losses        int
	ProfileURL    string
}

type wsEntry struct {
	ID     int
	Name   string // "First Last"
	School string
}

type wsCacheEntry struct {
	FetchedAt time.Time
	Entries   []wsEntry
}

var (
	wsCacheMu sync.Mutex
	wsCache   = map[int]wsCacheEntry{} // key: weightClass (125, 133, ...)
)

const wsCacheTTL = 10 * time.Minute

var (
	reWrestlerID  = regexp.MustCompile(`/wrestler/(\d+)/`)
	reLeadingRank = regexp.MustCompile(`^#\d+\s+`)
	reRecord      = regexp.MustCompile(`(\d+)\s*-\s*(\d+)`)
)

// GET /api/admin/wrestlestat/candidates?weightClass=125&name=Vincent%20Robinson&school=NC%20State
func GetWrestleStatCandidates(c *fiber.Ctx) error {
	weightClass, err := strconv.Atoi(strings.TrimSpace(c.Query("weightClass")))
	if err != nil || weightClass <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "weightClass query param is required (integer)"})
	}

	name := strings.TrimSpace(c.Query("name"))
	school := strings.TrimSpace(c.Query("school"))
	if name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "name query param is required"})
	}

	entries, err := wsGetOrFetchWeightEntries(weightClass)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "Failed to fetch WrestleStat candidates"})
	}

	targetName := normalizeLoose(name)
	targetSchool := normalizeLoose(school)

	scored := make([]WrestleStatCandidate, 0, 8)
	for _, e := range entries {
		s := scoreCandidate(targetName, targetSchool, e)
		if s <= 0 {
			continue
		}
		scored = append(scored, WrestleStatCandidate{
			WrestleStatID: e.ID,
			Name:          e.Name,
			School:        e.School,
			Score:         s,
		})
	}

	sort.Slice(scored, func(i, j int) bool { return scored[i].Score > scored[j].Score })

	if len(scored) > 5 {
		scored = scored[:5]
	}
	return c.JSON(scored)
}

// POST /api/admin/rankings/releases/:id/resolve/lookup?weightClass=125
// Returns candidate lists for each unresolved staging row (NO DB writes).
func BulkLookupWrestleStatCandidates(c *fiber.Ctx) error {
	releaseID, err := strconv.Atoi(c.Params("id"))
	if err != nil || releaseID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid release id"})
	}

	weightClass, err := strconv.Atoi(strings.TrimSpace(c.Query("weightClass")))
	if err != nil || weightClass <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "weightClass query param is required (integer)"})
	}

	// Validate release exists and is draft
	var status string
	if err := database.DB.QueryRow(`SELECT status FROM rankings_releases WHERE id = $1`, releaseID).Scan(&status); err != nil {
		if err == sql.ErrNoRows {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Release not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to validate release"})
	}
	if status != "draft" {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Cannot resolve candidates for a published release"})
	}

	// Load unresolved rows for this weight class
	type stagingRow struct {
		ID     int
		Name   string
		School string
	}
	rows := make([]stagingRow, 0, 64)

	q := `
		SELECT id, name, school
		FROM rankings_release_staging_rows
		WHERE release_id = $1
		  AND weight_class = $2
		  AND wrestlestat_id IS NULL
		  AND (row_status = 'unresolved' OR row_status = 'needs_review')
		ORDER BY rank ASC
	`
	rset, err := database.DB.Query(q, releaseID, weightClass)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load staging rows"})
	}
	defer rset.Close()

	for rset.Next() {
		var r stagingRow
		if err := rset.Scan(&r.ID, &r.Name, &r.School); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to read staging rows"})
		}
		rows = append(rows, r)
	}
	if err := rset.Err(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to read staging rows"})
	}

	// Fetch WrestleStat entries ONCE for this weight class (cached)
	wsEntries, err := wsGetOrFetchWeightEntries(weightClass)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "Failed to fetch WrestleStat data"})
	}

	// Response payload: rowId -> top candidates
	type rowCandidates struct {
		RowID      int                    `json:"rowId"`
		Name       string                 `json:"name"`
		School     string                 `json:"school"`
		Candidates []WrestleStatCandidate `json:"candidates"`
	}

	out := make([]rowCandidates, 0, len(rows))

	for _, r := range rows {
		targetName := normalizeLoose(r.Name)
		targetSchool := normalizeLoose(r.School)

		cands := make([]WrestleStatCandidate, 0, 10)
		for _, e := range wsEntries {
			s := scoreCandidate(targetName, targetSchool, e)
			if s <= 0 {
				continue
			}
			cands = append(cands, WrestleStatCandidate{
				WrestleStatID: e.ID,
				Name:          e.Name,
				School:        e.School,
				Score:         s,
			})
		}

		sort.Slice(cands, func(i, j int) bool { return cands[i].Score > cands[j].Score })
		if len(cands) > 5 {
			cands = cands[:5]
		}

		out = append(out, rowCandidates{
			RowID:      r.ID,
			Name:       r.Name,
			School:     r.School,
			Candidates: cands,
		})
	}

	return c.JSON(fiber.Map{
		"releaseId":   releaseID,
		"weightClass": weightClass,
		"rows":        out,
	})
}

// POST /api/admin/rankings/releases/:id/enrich ----
func EnrichRankingsRelease(c *fiber.Ctx) error {
	releaseID, err := strconv.Atoi(c.Params("id"))
	if err != nil || releaseID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid release id"})
	}

	// Load release season (and optionally status)
	var season string
	var status string
	err = database.DB.QueryRow(`
		SELECT season, status
		FROM rankings_releases
		WHERE id = $1
	`, releaseID).Scan(&season, &status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Release not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load release"})
	}

	// Strong recommendation: only enrich published (adjust if you prefer)
	if status != "published" {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error": "Release must be published before enrichment",
		})
	}

	// Pull distinct wrestlers from staging for this release (gives us a name for slug fallback)
	type wsRow struct {
		WrestleStatID int
		NameGuess     string
		SchoolGuess   string
	}

	rows, err := database.DB.Query(`
		SELECT DISTINCT wrestlestat_id, name, school
		FROM rankings_release_staging_rows
		WHERE release_id = $1
		  AND wrestlestat_id IS NOT NULL
		  AND wrestlestat_id > 0
	`, releaseID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load staging rows"})
	}
	defer rows.Close()

	var items []wsRow
	for rows.Next() {
		var r wsRow
		if err := rows.Scan(&r.WrestleStatID, &r.NameGuess, &r.SchoolGuess); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to read staging rows"})
		}
		items = append(items, r)
	}
	if err := rows.Err(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to iterate staging rows"})
	}
	if len(items) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "No wrestlers found to enrich for this release"})
	}

	client := &http.Client{
		Timeout: 12 * time.Second,
		// allow redirects
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 6 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	type failure struct {
		WrestleStatID int    `json:"wrestlestatId"`
		Error         string `json:"error"`
	}

	okCount := 0
	failures := make([]failure, 0)

	for _, it := range items {
		prof, err := fetchAndParseWrestleStatProfile(client, it.WrestleStatID, it.NameGuess, it.SchoolGuess)
		if err != nil {
			failures = append(failures, failure{WrestleStatID: it.WrestleStatID, Error: err.Error()})
			continue
		}

		if err := upsertWrestlerProfileAndSeason(prof, season); err != nil {
			failures = append(failures, failure{WrestleStatID: it.WrestleStatID, Error: err.Error()})
			continue
		}
		okCount++
	}

	return c.JSON(fiber.Map{
		"releaseId": releaseID,
		"season":    season,
		"total":     len(items),
		"succeeded": okCount,
		"failed":    len(failures),
		"failures":  failures,
	})
}

func wsGetOrFetchWeightEntries(weightClass int) ([]wsEntry, error) {
	wsCacheMu.Lock()
	defer wsCacheMu.Unlock()

	if ce, ok := wsCache[weightClass]; ok {
		if time.Since(ce.FetchedAt) < wsCacheTTL && len(ce.Entries) > 0 {
			return ce.Entries, nil
		}
	}

	entries, err := wsScrapeStartersByWeight(weightClass)
	if err != nil {
		return nil, err
	}

	wsCache[weightClass] = wsCacheEntry{
		FetchedAt: time.Now(),
		Entries:   entries,
	}
	return entries, nil
}

func wsScrapeStartersByWeight(weightClass int) ([]wsEntry, error) {
	url := "https://www.wrestlestat.com/d1/rankings/starters/weight/" + strconv.Itoa(weightClass)

	client := &http.Client{Timeout: 12 * time.Second}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "gablegame-admin-resolver/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errors.New("non-200 from wrestlestat")
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	out := make([]wsEntry, 0, 120)
	seen := map[int]bool{}

	// Wrestler links look like: /wrestler/78062/robinson-vincent/profile
	doc.Find(`a[href*="/wrestler/"]`).Each(func(_ int, a *goquery.Selection) {
		href, ok := a.Attr("href")
		if !ok {
			return
		}
		m := reWrestlerID.FindStringSubmatch(href)
		if len(m) != 2 {
			return
		}
		id, err := strconv.Atoi(m[1])
		if err != nil || id <= 0 {
			return
		}
		if seen[id] {
			return
		}

		// Anchor text on starters page is "Last, First"
		rawName := strings.TrimSpace(a.Text())
		name := wsDisplayNameToFirstLast(rawName)
		if name == "" {
			return
		}

		// Try to find the adjacent team link in the same “block”
		school := findSchoolNearWrestlerAnchor(a)
		school = strings.TrimSpace(school)
		if school == "" {
			// still include; name-only matching may help
			school = ""
		}

		seen[id] = true
		out = append(out, wsEntry{ID: id, Name: name, School: school})
	})

	if len(out) == 0 {
		return nil, errors.New("no wrestlers parsed from wrestlestat page")
	}

	return out, nil
}

func findSchoolNearWrestlerAnchor(a *goquery.Selection) string {
	// On the starters page, the school is typically in a nearby anchor like "#7 NC State"
	// We look in the parent block first, then scan following siblings.
	parent := a.Parent()
	if parent != nil {
		if s := extractTeamAnchorText(parent); s != "" {
			return s
		}
	}

	// scan next siblings (limited) to find a team link
	next := a
	for i := 0; i < 8; i++ {
		next = next.Next()
		if next == nil || next.Length() == 0 {
			break
		}
		if s := extractTeamAnchorText(next); s != "" {
			return s
		}
	}

	// fallback: look slightly up then down
	if gp := a.Parent().Parent(); gp != nil {
		if s := extractTeamAnchorText(gp); s != "" {
			return s
		}
	}

	return ""
}

func extractTeamAnchorText(sel *goquery.Selection) string {
	team := sel.Find(`a[href*="/team/"]`).First()
	if team.Length() == 0 {
		return ""
	}
	txt := strings.TrimSpace(team.Text())
	txt = reLeadingRank.ReplaceAllString(txt, "") // "#7 NC State" -> "NC State"
	return strings.TrimSpace(txt)
}

func wsDisplayNameToFirstLast(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// "Robinson, Vincent" -> "Vincent Robinson"
	parts := strings.Split(s, ",")
	if len(parts) == 2 {
		last := strings.TrimSpace(parts[0])
		first := strings.TrimSpace(parts[1])
		if first != "" && last != "" {
			return first + " " + last
		}
	}
	return s
}

func normalizeLoose(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.ReplaceAll(s, "'", "")
	s = strings.Join(strings.Fields(s), " ")
	return s
}

func scoreCandidate(targetName, targetSchool string, e wsEntry) float64 {
	name := normalizeLoose(e.Name)
	school := normalizeLoose(e.School)

	// Name scoring (strong weight)
	nameScore := 0.0
	if name == targetName {
		nameScore = 1.0
	} else {
		// last-name match + first initial match is common and strong
		tParts := strings.Fields(targetName)
		cParts := strings.Fields(name)
		if len(tParts) >= 2 && len(cParts) >= 2 {
			tFirst, tLast := tParts[0], tParts[len(tParts)-1]
			cFirst, cLast := cParts[0], cParts[len(cParts)-1]
			if tLast == cLast && tFirst[:1] == cFirst[:1] {
				nameScore = 0.85
			} else if tLast == cLast {
				nameScore = 0.70
			}
		}
	}

	if nameScore == 0 {
		return 0
	}

	// School scoring (nice-to-have, but not always present)
	schoolScore := 0.0
	if targetSchool != "" && school != "" {
		if school == targetSchool {
			schoolScore = 1.0
		} else if strings.Contains(school, targetSchool) || strings.Contains(targetSchool, school) {
			schoolScore = 0.75
		} else {
			// weak match, keep as 0
			schoolScore = 0.0
		}
	}

	// Combine: name dominates; school boosts
	return (nameScore * 0.85) + (schoolScore * 0.15)
}

func fetchAndParseWrestleStatProfile(client *http.Client, wsid int, nameGuess, schoolGuess string) (*parsedProfile, error) {
	// 1) Try ID-only URL (may or may not work on WrestleStat; safe attempt)
	idOnlyURL := fmt.Sprintf("https://www.wrestlestat.com/wrestler/%d/profile", wsid)

	// 2) Fallback: pretty URL with slug
	slug := slugifyLastFirst(nameGuess)
	prettyURL := fmt.Sprintf("https://www.wrestlestat.com/wrestler/%d/%s/profile", wsid, slug)

	// Try id-only first
	doc, finalURL, err := getDoc(client, idOnlyURL)
	if err != nil || doc == nil {
		// fallback
		doc, finalURL, err = getDoc(client, prettyURL)
		if err != nil || doc == nil {
			if err != nil {
				return nil, fmt.Errorf("fetch failed: %w", err)
			}
			return nil, fmt.Errorf("fetch failed")
		}
	}

	// Parse the top summary from the document.
	// Based on WrestleStat profile pages: top area contains "#rank Name", then "#team", then "SR", then "wins - losses". :contentReference[oaicite:1]{index=1}
	name, team, classYear, wins, losses := parseTopSummary(doc)

	// Fallbacks
	if strings.TrimSpace(name) == "" {
		name = strings.TrimSpace(nameGuess)
	}
	if strings.TrimSpace(team) == "" {
		team = strings.TrimSpace(schoolGuess)
	}

	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("could not parse name for wsid=%d", wsid)
	}

	return &parsedProfile{
		WrestleStatID: wsid,
		Name:          name,
		Team:          team,
		ClassYear:     classYear,
		Wins:          wins,
		Losses:        losses,
		ProfileURL:    finalURL,
	}, nil
}

func getDoc(client *http.Client, url string) (*goquery.Document, string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "GableGame/1.0 (+https://gable-game.com)")

	res, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, "", fmt.Errorf("http %d for %s", res.StatusCode, url)
	}

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, "", err
	}

	finalURL := res.Request.URL.String()
	return doc, finalURL, nil
}

func parseTopSummary(doc *goquery.Document) (name, team, classYear string, wins, losses int) {
	// Find the first h3 that looks like "#<rank> <name>"
	h3 := doc.Find("h3").FilterFunction(func(_ int, s *goquery.Selection) bool {
		t := strings.TrimSpace(s.Text())
		return strings.HasPrefix(t, "#") && len(t) > 2
	}).First()

	if h3.Length() == 0 {
		// fallback: sometimes content is in h1/h2 depending on layout
		h3 = doc.Find("h2").FilterFunction(func(_ int, s *goquery.Selection) bool {
			t := strings.TrimSpace(s.Text())
			return strings.HasPrefix(t, "#") && len(t) > 2
		}).First()
	}

	if h3.Length() > 0 {
		rawName := strings.TrimSpace(h3.Text())
		rawName = reLeadingRank.ReplaceAllString(rawName, "")
		name = strings.TrimSpace(rawName)
	}

	// Team link typically appears near the top and links to /team/{id}/... :contentReference[oaicite:2]{index=2}
	teamLink := doc.Find("a[href*='/team/']").First()
	if teamLink.Length() > 0 {
		rawTeam := strings.TrimSpace(teamLink.Text())
		// remove leading rank like "#7 Minnesota"
		rawTeam = reLeadingRank.ReplaceAllString(rawTeam, "")
		team = strings.TrimSpace(rawTeam)
	}

	// Collect text around the top summary area, and scan for class year and record.
	// This is intentionally heuristic but works well on typical WrestleStat profile pages. :contentReference[oaicite:3]{index=3}
	pageText := cleanSpaces(doc.Find("body").Text())

	// Class year tokens – include common redshirt formats
	for _, tok := range []string{"RSSR", "RSJR", "RSO", "RSFR", "SR", "JR", "SO", "FR"} {
		if strings.Contains(pageText, " "+tok+" ") || strings.Contains(pageText, "\n"+tok+"\n") {
			classYear = tok
			break
		}
	}

	// Record – choose the first plausible "W - L" after the name appears if possible
	if m := reRecord.FindStringSubmatch(pageText); len(m) == 3 {
		wins, _ = strconv.Atoi(m[1])
		losses, _ = strconv.Atoi(m[2])
	}

	return name, team, classYear, wins, losses
}

func cleanSpaces(s string) string {
	// normalize whitespace
	var b strings.Builder
	b.Grow(len(s))
	space := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !space {
				b.WriteRune(' ')
				space = true
			}
			continue
		}
		space = false
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

func slugifyLastFirst(fullName string) string {
	fullName = strings.TrimSpace(fullName)
	if fullName == "" {
		return "unknown"
	}

	// remove punctuation except spaces and hyphens
	clean := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == ' ' || r == '-' {
			return unicode.ToLower(r)
		}
		return -1
	}, fullName)

	parts := strings.Fields(clean)
	if len(parts) == 1 {
		return parts[0]
	}
	last := parts[len(parts)-1]
	firstParts := parts[:len(parts)-1]

	// join: last-first-first...
	return last + "-" + strings.Join(firstParts, "-")
}

func upsertWrestlerProfileAndSeason(p *parsedProfile, season string) error {
	tx, err := database.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1) Upsert into wrestler_profiles
	_, err = tx.Exec(`
		INSERT INTO wrestler_profiles (wrestlestat_id, name, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (wrestlestat_id)
		DO UPDATE SET
			name = EXCLUDED.name,
			updated_at = NOW()
	`, p.WrestleStatID, p.Name)
	if err != nil {
		return fmt.Errorf("upsert wrestler_profiles: %w", err)
	}

	// 2) Upsert into wrestler_seasons
	_, err = tx.Exec(`
		INSERT INTO wrestler_seasons
			(wrestlestat_id, season, class_year, team, wins, losses, updated_at)
		VALUES
			($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (wrestlestat_id, season)
		DO UPDATE SET
			class_year = EXCLUDED.class_year,
			team = EXCLUDED.team,
			wins = EXCLUDED.wins,
			losses = EXCLUDED.losses,
			updated_at = NOW()
	`, p.WrestleStatID, season, nullIfEmpty(p.ClassYear), nullIfEmpty(p.Team), nullIfZero(p.Wins), nullIfZero(p.Losses))
	if err != nil {
		return fmt.Errorf("upsert wrestler_seasons: %w", err)
	}

	return tx.Commit()
}

func nullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.TrimSpace(s)
}

func nullIfZero(n int) any {
	if n == 0 {
		return nil
	}
	return n
}
