// ingest_season_tournaments fetches all tournament (non-dual) bout results for
// every D1 team in a TrackWrestling season and writes them to a single CSV.
//
// Phase 1 — Schedule enumeration (root /seasons/ subsystem):
//  1. GET index.jsp + LoadBalance + MainFrame — establish session
//  2. getTeams — list all ~78 D1 teams
//  3. getTeamSchedule — collect tournament eventIds from each team's schedule
//     (schedule rows where row[1] != "0"; row[0] is the eventId)
//
// Phase 2 — Bout data (tw/seasons/ subsystem):
//  4. Establish a second session under /tw/seasons/
//  5. For each unique tournament eventId:
//     a. EventTeams.jsp — list all participating teams with regular-season teamIds
//     b. getEventMatchesJSP — fetch bout rows per team, deduplicate by match ID
//
// Usage:
//
//	go run ./cmd/ingest_season_tournaments -out tournaments_2026.csv
//	go run ./cmd/ingest_season_tournaments -seasonid 841725138 -season 2025 -out tournaments_2025.csv
package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// TrackWrestling season IDs for NCAA College Men — update each season.
// 2025-26: 1560238138  |  2024-25: 841725138
const defaultSeasonID = "1560238138"
const defaultGbID = "2026076"

const browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

const seasonsBase = "https://www.trackwrestling.com/seasons"
const twSeasonsBase = "https://www.trackwrestling.com/tw/seasons"

// bootstrapTeamID is any valid D1 regular-season teamId; used to seed the
// /tw/seasons/ session. Air Force used here.
const bootstrapTeamID = "758758150"

var schoolSlugOverrides = map[string]string{
	// Dual-meet abbreviations
	"OKST": "oklahoma-state",
	"IOWA": "iowa",
	"PSU":  "penn-state",
	"OSU":  "ohio-state",
	"NEB":  "nebraska",
	"MINN": "minnesota",
	"MICH": "michigan",
	"NWTN": "northwestern",
	"ILL":  "illinois",
	"IND":  "indiana",
	"PURD": "purdue",
	"MSU":  "michigan-state",
	"WISC": "wisconsin",
	"UNI":  "northern-iowa",
	"ISU":  "iowa-state",
	"MIZZ": "missouri",
	"OKLA": "oklahoma",
	"WVU":  "west-virginia",
	"PITT": "pittsburgh",
	"NCST": "nc-state",
	"VT":   "virginia-tech",
	"UVA":  "virginia",
	"DUKE": "duke",
	"UNC":  "north-carolina",
	"CLEM": "clemson",
	"PRIN": "princeton",
	"CORN": "cornell",
	"LEHG": "lehigh",
	"HOFP": "hofstra",
	"APPST": "app-state",
	"ARIZ": "arizona-state",
	"CAL":  "cal-poly",
	"CLEV": "cleveland-state",
	"CSUB": "csub",
	"SDSU": "south-dakota-state",
	"WYOM": "wyoming",
	// Tournament abbreviations (EventTeams data)
	"AF":   "air-force",
	"AMER": "american",
	"APP":  "app-state",
	"ASU":  "arizona-state",
	"ARMY": "army",
	"BU":   "bellarmine",
	"BING": "binghamton",
	"BLOO": "bloomsburg",
	"BRWN": "brown",
	"BUCK": "bucknell",
	"CAMP": "campbell",
	"CENT": "central-michigan",
	"CHAT": "chattanooga",
	"CLMS": "clemson",
	"DREX": "drexel",
	"EDIN": "edinboro",
	"EMU":  "eastern-michigan",
	"KENT": "kent-state",
	"LEH":  "lehigh",
	"LH":   "lock-haven",
	"NAVY": "navy",
	"NCAT": "nc-a-t",
	"NW":   "northwestern",
	"OHIO": "ohio",
	"RID":  "rider",
	"RUTG": "rutgers",
	"SDST": "south-dakota-state",
	"SHU":  "sacred-heart",
	"STAN": "stanford",
}

func schoolSlug(abbr, fullName string) string {
	if s, ok := schoolSlugOverrides[strings.ToUpper(abbr)]; ok {
		return s
	}
	s := strings.ToLower(strings.TrimSpace(fullName))
	s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

func formatTime(seconds int) string {
	if seconds == 0 {
		return ""
	}
	return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
}

func tim() string { return fmt.Sprintf("%d", time.Now().UnixMilli()) }

func addBrowserHeaders(req *http.Request) {
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Connection", "keep-alive")
}

func doGet(client *http.Client, u, referer string) ([]byte, error) {
	req, _ := http.NewRequest("GET", u, nil)
	addBrowserHeaders(req)
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return b, nil
}

func trimJSON(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

var twSessionRe = regexp.MustCompile(`twSessionId=([a-zA-Z0-9]+)`)
var mainFrameActionRe = regexp.MustCompile(`action\s*=\s*["']\.?/?MainFrame\.jsp\?([^"']+)["']`)

// ─── Phase 1: root /seasons/ session ─────────────────────────────────────────

func setupRootSession(client *http.Client, seasonID, gbID string) (sessionID, mfURL string, err error) {
	indexURL := seasonsBase + "/index.jsp?TIM=" + tim()
	body, err := doGet(client, indexURL, "")
	if err != nil {
		return "", "", fmt.Errorf("index: %w", err)
	}
	m := twSessionRe.FindSubmatch(body)
	if len(m) < 2 {
		return "", "", fmt.Errorf("no twSessionId in index")
	}
	sessionID = string(m[1])

	lbURL := fmt.Sprintf("%s/LoadBalance.jsp?TIM=%s&twSessionId=%s&seasonId=%s&gbId=%s&uname=&pword=",
		seasonsBase, tim(), sessionID, seasonID, gbID)
	lbBody, err := doGet(client, lbURL, indexURL)
	if err != nil {
		return "", "", fmt.Errorf("loadbalance: %w", err)
	}
	mfM := mainFrameActionRe.FindSubmatch(lbBody)
	if len(mfM) < 2 {
		return "", "", fmt.Errorf("no MainFrame action in loadbalance")
	}
	mfURL = seasonsBase + "/MainFrame.jsp?" + string(mfM[1])

	formVals := "seasonId=" + seasonID + "&gbId=" + gbID +
		"&uname=&pword=&myTrackId=&myTrackPW=&pageName=&hideFrame=&wrestlerId=&dualId=&teamId=&passcode=&eventId="
	req, _ := http.NewRequest("POST", mfURL, strings.NewReader(formVals))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", lbURL)
	addBrowserHeaders(req)
	resp, _ := client.Do(req)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	log.Printf("root session: %s", sessionID)
	return sessionID, mfURL, nil
}

func getD1Teams(client *http.Client, sessionID, seasonID, referer string) ([][]interface{}, error) {
	u := fmt.Sprintf("%s/AjaxFunctions.jsp?TIM=%s&twSessionId=%s&function=getTeams&seasonId=%s",
		seasonsBase, tim(), sessionID, seasonID)
	body, err := doGet(client, u, referer)
	if err != nil {
		return nil, err
	}
	var all [][]interface{}
	if err := json.Unmarshal(trimJSON(body), &all); err != nil {
		return nil, fmt.Errorf("unmarshal teams: %w", err)
	}
	d1Re := regexp.MustCompile(`\bDI\b`)
	var d1 [][]interface{}
	for _, t := range all {
		if len(t) < 7 {
			continue
		}
		gbID, _ := t[6].(string)
		div, _ := t[5].(string)
		if gbID == "3" && d1Re.MatchString(div) {
			d1 = append(d1, t)
		}
	}
	return d1, nil
}

// tournamentInfo holds metadata extracted from a team's schedule entry.
type tournamentInfo struct {
	EventID   string
	Name      string
	Date      string // YYYY-MM-DD
}

// getTournamentSchedule returns all non-dual entries from a team's schedule.
// row[1]=="0" → dual meet (skip); anything else → tournament.
func getTournamentSchedule(client *http.Client, sessionID, teamID, seasonID, referer string) ([]tournamentInfo, error) {
	u := fmt.Sprintf("%s/AjaxFunctions.jsp?TIM=%s&twSessionId=%s&function=getTeamSchedule&teamId=%s&seasonId=%s",
		seasonsBase, tim(), sessionID, teamID, seasonID)
	body, err := doGet(client, u, referer)
	if err != nil {
		return nil, err
	}
	data := trimJSON(body)
	if len(data) == 0 || data[0] != '[' {
		return nil, nil
	}
	var rows [][]interface{}
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, fmt.Errorf("unmarshal schedule: %w", err)
	}
	var events []tournamentInfo
	for _, row := range rows {
		if len(row) < 4 {
			continue
		}
		eventType, _ := row[1].(string)
		if eventType == "0" { // dual meet — skip
			continue
		}
		id, _ := row[0].(string)
		name, _ := row[2].(string)
		dateRaw, _ := row[3].(string) // YYYYMMDD
		date := ""
		if len(dateRaw) == 8 {
			date = dateRaw[0:4] + "-" + dateRaw[4:6] + "-" + dateRaw[6:8]
		}
		if id != "" {
			events = append(events, tournamentInfo{EventID: id, Name: name, Date: date})
		}
	}
	return events, nil
}

// ─── Phase 2: /tw/seasons/ session ───────────────────────────────────────────

type twSession struct {
	client    *http.Client
	sessionID string
	mfURL     string
}

func setupTWSession(seasonID, gbID string) (*twSession, error) {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 30 * time.Second}

	indexURL := twSeasonsBase + "/index.jsp?TIM=" + tim()
	body, err := doGet(client, indexURL, "")
	if err != nil {
		return nil, fmt.Errorf("tw index: %w", err)
	}
	m := twSessionRe.FindSubmatch(body)
	if len(m) < 2 {
		return nil, fmt.Errorf("no twSessionId in tw index")
	}
	sessionID := string(m[1])

	lbURL := fmt.Sprintf("%s/LoadBalance.jsp?TIM=%s&twSessionId=%s&seasonId=%s&gbId=%s&pageName=EventMatches.jsp&teamId=%s&uname=&pword=",
		twSeasonsBase, tim(), sessionID, seasonID, gbID, bootstrapTeamID)
	lbBody, err := doGet(client, lbURL, indexURL)
	if err != nil {
		return nil, fmt.Errorf("tw loadbalance: %w", err)
	}
	mfM := mainFrameActionRe.FindSubmatch(lbBody)
	if len(mfM) < 2 {
		return nil, fmt.Errorf("no MainFrame action in tw loadbalance")
	}
	mfURL := twSeasonsBase + "/MainFrame.jsp?" + string(mfM[1])

	fv := url.Values{}
	fv.Set("seasonId", seasonID); fv.Set("gbId", gbID); fv.Set("teamId", bootstrapTeamID)
	fv.Set("pageName", "EventMatches.jsp"); fv.Set("uname", ""); fv.Set("pword", "")
	fv.Set("myTrackId", ""); fv.Set("myTrackPW", ""); fv.Set("hideFrame", "")
	fv.Set("wrestlerId", ""); fv.Set("dualId", ""); fv.Set("passcode", ""); fv.Set("eventId", "")
	req, _ := http.NewRequest("POST", mfURL, strings.NewReader(fv.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", lbURL)
	addBrowserHeaders(req)
	resp, _ := client.Do(req)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	log.Printf("tw session: %s", sessionID)
	return &twSession{client: client, sessionID: sessionID, mfURL: mfURL}, nil
}

// teamEntry holds participating team data from EventTeams.jsp.
type teamEntry struct {
	regularTeamID string
	name          string
}

func fetchEventTeams(tw *twSession, eventID string) ([]teamEntry, error) {
	u := fmt.Sprintf("%s/EventTeams.jsp?TIM=%s&twSessionId=%s&eventId=%s",
		twSeasonsBase, tim(), tw.sessionID, eventID)
	body, err := doGet(tw.client, u, tw.mfURL)
	if err != nil {
		return nil, fmt.Errorf("EventTeams: %w", err)
	}

	pageStr := string(body)
	gridIdx := strings.Index(pageStr, "initDataGrid(")
	if gridIdx < 0 {
		return nil, fmt.Errorf("no initDataGrid in EventTeams.jsp")
	}
	after := pageStr[gridIdx:]
	start := strings.Index(after, `"[[`)
	if start < 0 {
		return nil, fmt.Errorf("no JSON in initDataGrid")
	}
	tail := after[start+3:]
	end := strings.Index(tail, `]]"`)
	if end < 0 {
		return nil, fmt.Errorf("no closing ]] in initDataGrid")
	}
	raw := "[[" + tail[:end] + "]]"
	raw = strings.ReplaceAll(raw, `\"`, `"`)

	var grid [][]interface{}
	if err := json.Unmarshal([]byte(raw), &grid); err != nil {
		return nil, fmt.Errorf("unmarshal EventTeams: %w", err)
	}
	teams := make([]teamEntry, 0, len(grid))
	for _, row := range grid {
		if len(row) < 4 {
			continue
		}
		teams = append(teams, teamEntry{
			regularTeamID: str(row[3]),
			name:          str(row[1]),
		})
	}
	return teams, nil
}

type boutRow []interface{}

func fetchTeamMatches(tw *twSession, eventID, teamID string) ([]boutRow, error) {
	u := fmt.Sprintf("%s/AjaxFunctions.jsp?TIM=%s&twSessionId=%s&function=getEventMatchesJSP&eventId=%s&teamId=%s",
		twSeasonsBase, tim(), tw.sessionID, eventID, teamID)
	body, err := doGet(tw.client, u, tw.mfURL)
	if err != nil {
		return nil, fmt.Errorf("getEventMatchesJSP: %w", err)
	}
	trimmed := trimJSON(body)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil, nil
	}
	if trimmed[0] != '[' {
		return nil, fmt.Errorf("unexpected response: %s", string(trimmed[:min(80, len(trimmed))]))
	}
	var grid [][]interface{}
	if err := json.Unmarshal(trimmed, &grid); err != nil {
		return nil, fmt.Errorf("unmarshal matches: %w", err)
	}
	result := make([]boutRow, len(grid))
	for i, row := range grid {
		result[i] = row
	}
	return result, nil
}

// fetchTournamentMatches collects all unique bouts for one event, iterating
// every participating team and deduplicating by source match ID.
func fetchTournamentMatches(tw *twSession, eventID string, delay time.Duration) (map[string]boutRow, []teamEntry, error) {
	teams, err := fetchEventTeams(tw, eventID)
	if err != nil {
		return nil, nil, err
	}

	allMatches := make(map[string]boutRow)
	for _, team := range teams {
		rows, err := fetchTeamMatches(tw, eventID, team.regularTeamID)
		if err != nil {
			log.Printf("  team %s (%s): %v — skipping", team.name, team.regularTeamID, err)
			continue
		}
		for _, row := range rows {
			if len(row) == 0 {
				continue
			}
			matchID := str(row[0])
			if _, exists := allMatches[matchID]; !exists {
				allMatches[matchID] = row
			}
		}
		time.Sleep(delay)
	}
	return allMatches, teams, nil
}

// ─── CSV output ───────────────────────────────────────────────────────────────

func writeTournamentBouts(cw *csv.Writer, matches map[string]boutRow, event tournamentInfo, seasonYear int) int {
	written := 0
	for _, row := range matches {
		if len(row) < 32 {
			continue
		}
		weight := str(row[1])
		w1Name := strings.TrimSpace(str(row[16]) + " " + str(row[17]))
		w1School := schoolSlug(str(row[19]), str(row[18]))
		w2Name := strings.TrimSpace(str(row[20]) + " " + str(row[21]))
		w2School := schoolSlug(str(row[23]), str(row[22]))

		// row[6] = 1 means wrestler 1 won; 2 means wrestler 2 won.
		// row[7] = w1 score (points), row[10] = w2 score (points).
		winnerIdx := str(row[6])
		winnerName := w2Name
		scoreWinner, scoreLoser := intVal(row[10]), intVal(row[7])
		if winnerIdx == "1" {
			winnerName = w1Name
			scoreWinner, scoreLoser = intVal(row[7]), intVal(row[10])
		}

		method := strings.ToUpper(str(row[4]))
		matchTime := formatTime(intVal(row[13]))
		roundName := str(row[31])

		_ = cw.Write([]string{
			strconv.Itoa(seasonYear),
			event.Name,
			event.Date,
			"tournament",
			event.EventID,
			weight,
			roundName,
			w1Name, w1School,
			w2Name, w2School,
			winnerName,
			method,
			strconv.Itoa(scoreWinner),
			strconv.Itoa(scoreLoser),
			matchTime,
			str(row[0]),
		})
		written++
	}
	return written
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func str(v interface{}) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func intVal(v interface{}) int {
	s := str(v)
	n, _ := strconv.Atoi(strings.TrimSuffix(s, ".0"))
	return n
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ─── Main ─────────────────────────────────────────────────────────────────────

func main() {
	seasonID := flag.String("seasonid", defaultSeasonID, "TrackWrestling internal season ID")
	gbID := flag.String("gbid", defaultGbID, "TrackWrestling governing body ID")
	seasonYear := flag.Int("season", 2026, "Season end year (e.g. 2026 for 2025-26)")
	outPath := flag.String("out", "tournaments.csv", "Output CSV path")
	delayMs := flag.Int("delay", 250, "Milliseconds between API requests")
	flag.Parse()

	delay := time.Duration(*delayMs) * time.Millisecond

	// ── Phase 1: enumerate tournament eventIds from D1 team schedules ──

	jar, _ := cookiejar.New(nil)
	rootClient := &http.Client{Jar: jar, Timeout: 30 * time.Second}

	log.Println("phase 1: establishing root session...")
	rootSID, rootMF, err := setupRootSession(rootClient, *seasonID, *gbID)
	if err != nil {
		log.Fatalf("root session: %v", err)
	}

	log.Println("fetching D1 team list...")
	time.Sleep(delay)
	teams, err := getD1Teams(rootClient, rootSID, *seasonID, rootMF)
	if err != nil {
		log.Fatalf("get D1 teams: %v", err)
	}
	log.Printf("found %d D1 teams", len(teams))

	tournaments := map[string]tournamentInfo{}
	for i, team := range teams {
		teamID, _ := team[0].(string)
		time.Sleep(delay)
		events, err := getTournamentSchedule(rootClient, rootSID, teamID, *seasonID, rootMF)
		if err != nil {
			log.Printf("schedule for team %s: %v (skipping)", teamID, err)
			continue
		}
		newCount := 0
		for _, ev := range events {
			if _, seen := tournaments[ev.EventID]; !seen {
				tournaments[ev.EventID] = ev
				newCount++
			}
		}
		log.Printf("[%d/%d] team %s (%s): %d tournament events, %d new (total unique: %d)",
			i+1, len(teams), teamID, team[1], len(events), newCount, len(tournaments))
	}
	log.Printf("phase 1 done — %d unique tournament eventIds discovered", len(tournaments))

	// ── Phase 2: fetch bout data for each tournament ──

	log.Println("phase 2: establishing /tw/seasons/ session...")
	tw, err := setupTWSession(*seasonID, *gbID)
	if err != nil {
		log.Fatalf("tw session: %v", err)
	}

	f, err := os.Create(*outPath)
	if err != nil {
		log.Fatalf("create output: %v", err)
	}
	defer f.Close()

	cw := csv.NewWriter(f)
	_ = cw.Write([]string{
		"season_year", "event_name", "event_date", "event_type", "event_id",
		"weight_label", "round_name",
		"wrestler_a_name", "wrestler_a_school_slug",
		"wrestler_b_name", "wrestler_b_school_slug",
		"winner_name", "result_method",
		"score_winner", "score_loser", "match_time", "source_match_id",
	})

	totalBouts := 0
	i := 0
	for _, event := range tournaments {
		i++
		log.Printf("[%d/%d] fetching %s (%s)...", i, len(tournaments), event.Name, event.EventID)

		matches, eventTeams, err := fetchTournamentMatches(tw, event.EventID, delay)
		if err != nil {
			log.Printf("  error fetching teams for event %s: %v — skipping", event.EventID, err)
			continue
		}
		if len(matches) == 0 {
			log.Printf("  no bouts found (event may not have results yet or may not use EventMatches)")
			continue
		}

		written := writeTournamentBouts(cw, matches, event, *seasonYear)
		totalBouts += written
		log.Printf("  done — %d participating teams, %d unique bouts written", len(eventTeams), written)

		cw.Flush()
		if err := cw.Error(); err != nil {
			log.Fatalf("csv flush: %v", err)
		}
	}

	log.Printf("done — wrote %d bouts across %d tournaments to %s", totalBouts, len(tournaments), *outPath)
}
