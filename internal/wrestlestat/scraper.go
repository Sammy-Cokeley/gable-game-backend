package wrestlestat

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/PuerkitoBio/goquery"
)

// WrestlerEntry holds all attributes scraped from WrestleStat for a single wrestler.
type WrestlerEntry struct {
	WrestleStatID int
	ProfileURL    string
	Name          string // "First Last"
	School        string // display name, e.g. "NC State"
	SchoolSlug    string // slugified for DB lookup, e.g. "nc-state"
	Conference    string // e.g. "Big Ten"
	WeightClass   string // e.g. "125"
	ClassYear     string // FR, SO, JR, SR, RSFR, RSJR, etc.
	Wins          int
	Losses        int
	NCAAFinish    string // populated separately by FetchNCAAFinish
}

var (
	reWrestlerID   = regexp.MustCompile(`/wrestler/(\d+)/`)
	reLeadingRank  = regexp.MustCompile(`^#\d+\s+`)
	reRecord       = regexp.MustCompile(`(\d+)\s*-\s*(\d+)`)
	reWeightDigits = regexp.MustCompile(`\b(125|133|141|149|157|165|174|184|197|285)\b`)
)

// ScrapeStartersByWeight scrapes the WrestleStat current-season starters page
// for a single weight class. Each entry is populated with name, school,
// conference, record, class year, and WrestleStat ID.
// NCAAFinish is left empty — call FetchNCAAFinish to populate it.
func ScrapeStartersByWeight(client *http.Client, weightClass int) ([]WrestlerEntry, error) {
	url := fmt.Sprintf("https://www.wrestlestat.com/d1/rankings/starters/weight/%d", weightClass)
	doc, err := fetchDoc(client, url)
	if err != nil {
		return nil, err
	}

	weight := strconv.Itoa(weightClass)
	seen := map[int]bool{}
	entries := parseWrestlerRows(doc.Selection, weight, seen)
	if len(entries) == 0 {
		return nil, errors.New("no wrestlers found on starters page")
	}
	return entries, nil
}

// ScrapeAllStartersByYear scrapes the WrestleStat year-indexed starters page,
// which lists all weight classes in tab sections on a single page.
// Pass weightFilter=0 to return all weight classes, or a specific weight (e.g. 125)
// to return only that class.
func ScrapeAllStartersByYear(client *http.Client, seasonYear int, weightFilter int) ([]WrestlerEntry, error) {
	url := fmt.Sprintf("https://www.wrestlestat.com/d1/season/%d/rankings/starters", seasonYear)
	doc, err := fetchDoc(client, url)
	if err != nil {
		return nil, err
	}

	seen := map[int]bool{}
	var all []WrestlerEntry

	// Each weight class is in a Bootstrap tab pane. Find each pane, extract its
	// weight label from the pane heading or ID, then parse the wrestler rows.
	//
	// Note: if the page uses a different container class (e.g. "weight-section"),
	// update this selector on first run by inspecting the page source.
	doc.Find(".tab-pane").Each(func(_ int, pane *goquery.Selection) {
		weight := extractWeightLabel(pane)
		if weight == "" {
			return
		}
		if weightFilter > 0 && weight != strconv.Itoa(weightFilter) {
			return
		}
		entries := parseWrestlerRows(pane, weight, seen)
		all = append(all, entries...)
	})

	if len(all) == 0 {
		return nil, errors.New("no wrestlers found on year-indexed starters page")
	}
	return all, nil
}

// FetchNCAAFinish fetches the wrestler's WrestleStat profile page and sets
// entry.NCAAFinish for the given season year. Pass seasonYear=0 to use the
// most recent season (existing behavior). If no placement is found for that
// year, NCAAFinish remains empty (the wrestler was not a qualifier).
func FetchNCAAFinish(client *http.Client, entry *WrestlerEntry, seasonYear int) error {
	url := fmt.Sprintf(
		"https://www.wrestlestat.com/wrestler/%d/%s/profile",
		entry.WrestleStatID, nameToSlug(entry.Name),
	)
	entry.ProfileURL = url

	doc, err := fetchDoc(client, url)
	if err != nil {
		return err
	}

	entry.NCAAFinish = parseNCAAFinish(doc, seasonYear)

	// Also refresh wins/losses from profile if starters page had zeros
	if entry.Wins == 0 && entry.Losses == 0 {
		if w, l, ok := parseRecord(doc); ok {
			entry.Wins = w
			entry.Losses = l
		}
	}

	return nil
}

// Slugify converts a display name or school name to a URL slug.
func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
		} else if !prevDash {
			b.WriteRune('-')
			prevDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// ---- internal helpers -------------------------------------------------------

// parseWrestlerRows parses wrestler table rows from a goquery selection (either
// the full document or a single tab-pane). Each entry is tagged with the given
// weight label. seen deduplicates by WrestleStat ID across calls.
func parseWrestlerRows(sel *goquery.Selection, weight string, seen map[int]bool) []WrestlerEntry {
	var entries []WrestlerEntry
	sel.Find("tr").Each(func(_ int, row *goquery.Selection) {
		tds := row.Find("td")
		if tds.Length() < 4 {
			return
		}

		cell2 := tds.Eq(2)
		wLink := cell2.Find(`a[href*="/wrestler/"]`).First()
		if wLink.Length() == 0 {
			return
		}

		href, _ := wLink.Attr("href")
		m := reWrestlerID.FindStringSubmatch(href)
		if len(m) != 2 {
			return
		}
		wsid, err := strconv.Atoi(m[1])
		if err != nil || wsid <= 0 || seen[wsid] {
			return
		}
		seen[wsid] = true

		rawName := strings.TrimSpace(wLink.Text())
		name := displayNameToFirstLast(rawName)
		if name == "" {
			return
		}

		classYear := extractClassYear(cell2.Text())

		sLink := cell2.Find(`a[href*="/team/"]`).First()
		school := ""
		if sLink.Length() > 0 {
			rawSchool := strings.TrimSpace(sLink.Text())
			rawSchool = reLeadingRank.ReplaceAllString(rawSchool, "")
			school = strings.TrimSpace(rawSchool)
		}

		conference, wins, losses := parseConferenceAndRecord(tds.Eq(3))

		profileURL := "https://www.wrestlestat.com" + strings.TrimSuffix(href, "/")
		if !strings.HasSuffix(href, "/profile") {
			profileURL = "https://www.wrestlestat.com/wrestler/" + strconv.Itoa(wsid) + "/" + nameToSlug(name) + "/profile"
		}

		entries = append(entries, WrestlerEntry{
			WrestleStatID: wsid,
			ProfileURL:    profileURL,
			Name:          name,
			School:        school,
			SchoolSlug:    Slugify(school),
			Conference:    conference,
			WeightClass:   weight,
			ClassYear:     classYear,
			Wins:          wins,
			Losses:        losses,
		})
	})
	return entries
}

// extractWeightLabel extracts the weight class string (e.g. "125", "285") from
// a tab-pane selection by checking its heading text then its ID attribute.
func extractWeightLabel(pane *goquery.Selection) string {
	heading := strings.TrimSpace(pane.Find("h2,h3,h4").First().Text())
	if w := normalizeWeightLabel(heading); w != "" {
		return w
	}
	id, _ := pane.Attr("id")
	return normalizeWeightLabel(id)
}

// normalizeWeightLabel extracts a valid weight class label from a raw string.
// Maps "HWT" / "Heavyweight" → "285"; returns "" if unrecognised.
func normalizeWeightLabel(s string) string {
	s = strings.TrimSpace(s)
	if m := reWeightDigits.FindString(s); m != "" {
		return m
	}
	upper := strings.ToUpper(s)
	if strings.Contains(upper, "HWT") || strings.Contains(upper, "HEAVYWEIGHT") {
		return "285"
	}
	return ""
}

func fetchDoc(client *http.Client, url string) (*goquery.Document, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "GableGame/1.0 (+https://gable-game.com)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("http %d for %s", resp.StatusCode, url)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}
	return doc, nil
}

// displayNameToFirstLast converts "Robinson, Vincent" → "Vincent Robinson".
// If there's no comma, returns the string as-is.
func displayNameToFirstLast(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	parts := strings.SplitN(s, ",", 2)
	if len(parts) == 2 {
		last := strings.TrimSpace(parts[0])
		first := strings.TrimSpace(parts[1])
		if first != "" && last != "" {
			return first + " " + last
		}
	}
	return s
}

// extractClassYear finds FR/SO/JR/SR (and redshirt variants) in the cell text.
func extractClassYear(cellText string) string {
	cellText = strings.TrimSpace(cellText)
	// Look for "| SO" or "| JR" etc. pattern
	if idx := strings.Index(cellText, "|"); idx >= 0 {
		after := strings.TrimSpace(cellText[idx+1:])
		for _, tok := range []string{"RSSR", "RSJR", "RSSO", "RSFR", "SR", "JR", "SO", "FR"} {
			if strings.HasPrefix(strings.ToUpper(after), tok) {
				return tok
			}
		}
	}
	return ""
}

// parseConferenceAndRecord extracts conference name and win/loss record from a <td>
// whose HTML looks like "Big Ten<br>25 - 0".
func parseConferenceAndRecord(cell *goquery.Selection) (conference string, wins, losses int) {
	// Collect text nodes separated by <br> elements
	parts := make([]string, 0, 2)
	cell.Contents().Each(func(_ int, node *goquery.Selection) {
		if goquery.NodeName(node) == "br" {
			parts = append(parts, "")
			return
		}
		text := strings.TrimSpace(node.Text())
		if text == "" {
			return
		}
		if len(parts) == 0 {
			parts = append(parts, text)
		} else {
			parts[len(parts)-1] += text
		}
	})

	if len(parts) >= 1 {
		conference = strings.TrimSpace(parts[0])
	}
	if len(parts) >= 2 {
		if m := reRecord.FindStringSubmatch(parts[1]); len(m) == 3 {
			wins, _ = strconv.Atoi(m[1])
			losses, _ = strconv.Atoi(m[2])
		}
	}
	// Fallback: search entire cell text for record pattern
	if wins == 0 && losses == 0 {
		if m := reRecord.FindStringSubmatch(cell.Text()); len(m) == 3 {
			wins, _ = strconv.Atoi(m[1])
			losses, _ = strconv.Atoi(m[2])
		}
	}
	return
}

// validNCAAFinish is the set of values WrestleStat places in the finish badge.
var validNCAAFinish = map[string]bool{
	"1st": true, "2nd": true, "3rd": true, "4th": true,
	"5th": true, "6th": true, "7th": true, "8th": true,
	"R12": true, "R16": true, "NQ": true,
}

// parseNCAAFinish reads NCAA tournament finish badge(s) from the profile's main
// responsive table. WrestleStat shows one row per season, each potentially
// containing a badge (1st–8th, R12, R16, NQ).
// When seasonYear is 0, returns the first/most-recent badge (existing behavior).
// When seasonYear is non-zero, finds the row whose first cell contains that year
// and returns the badge from that specific row.
//
// Note: WrestleStat rows use the season end-year (e.g. "2024" for 2023-24).
// If the site instead shows academic-year format ("2023-24"), update the yearStr
// format to: fmt.Sprintf("%d-%02d", seasonYear-1, seasonYear%100)
func parseNCAAFinish(doc *goquery.Document, seasonYear int) string {
	if seasonYear == 0 {
		return parseFirstNCAABadge(doc)
	}
	yearStr := strconv.Itoa(seasonYear)
	var found string
	doc.Find("div.table-responsive.d-none.d-sm-block tr").EachWithBreak(func(_ int, row *goquery.Selection) bool {
		if !strings.Contains(row.Find("td").First().Text(), yearStr) {
			return true // not this row
		}
		row.Find("span.badge.bg-light.text-dark.fw-bold").EachWithBreak(func(_ int, s *goquery.Selection) bool {
			text := strings.TrimSpace(s.Text())
			if validNCAAFinish[text] {
				found = text
				return false
			}
			return true
		})
		return false // stop after matching row
	})
	return found
}

// parseFirstNCAABadge returns the first valid NCAA finish badge on the page,
// corresponding to the most recent season.
func parseFirstNCAABadge(doc *goquery.Document) string {
	var found string
	doc.Find("div.table-responsive.d-none.d-sm-block span.badge.bg-light.text-dark.fw-bold").
		EachWithBreak(func(_ int, s *goquery.Selection) bool {
			text := strings.TrimSpace(s.Text())
			if validNCAAFinish[text] {
				found = text
				return false
			}
			return true
		})
	return found
}


func parseRecord(doc *goquery.Document) (wins, losses int, ok bool) {
	text := cleanPageText(doc.Find("body").Text())
	if m := reRecord.FindStringSubmatch(text); len(m) == 3 {
		w, _ := strconv.Atoi(m[1])
		l, _ := strconv.Atoi(m[2])
		return w, l, true
	}
	return 0, 0, false
}

func cleanPageText(s string) string {
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

// nameToSlug converts "Vincent Robinson" → "robinson-vincent" (last-first format for WrestleStat URLs).
func nameToSlug(fullName string) string {
	clean := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == ' ' || r == '-' {
			return unicode.ToLower(r)
		}
		return -1
	}, fullName)

	parts := strings.Fields(clean)
	if len(parts) == 0 {
		return "unknown"
	}
	if len(parts) == 1 {
		return parts[0]
	}
	last := parts[len(parts)-1]
	first := strings.Join(parts[:len(parts)-1], "-")
	return last + "-" + first
}
