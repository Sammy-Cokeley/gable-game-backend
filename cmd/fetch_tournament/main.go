// fetch_tournament fetches bout results for a TrackWrestling tournament event
// and writes a CSV in a format compatible with the dual-meet ingestion pipeline.
//
// Session flow (all under trackwrestling.com/tw/seasons/):
//  1. GET index.jsp               — obtain server-generated twSessionId
//  2. GET LoadBalance.jsp         — obtain MainFrame.jsp action URL
//  3. POST MainFrame.jsp          — establish season context in server session
//  4. GET EventTeams.jsp          — list all participating teams
//  5. GET AjaxFunctions.jsp       — fetch each team's bout data (getEventMatchesJSP)
//
// Results are deduplicated by source match ID since each match appears once
// per team queried.
//
// Usage:
//
//	go run ./cmd/fetch_tournament -event 8710102132 -season 2026 -out ncaa_2026.csv
//	go run ./cmd/fetch_tournament -event 8710102132 -event-name "2026 NCAA Division I Championships" -event-date 2026-03-20
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

// Governing body ID for College Men on TrackWrestling.
const defaultGbID = "2026076"

const browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

// tw/seasons base URL — distinct from the /seasons/ dual-meet subsystem.
const twSeasonsBase = "https://www.trackwrestling.com/tw/seasons"

var schoolSlugOverrides = map[string]string{
	// TW dual-meet abbreviations
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
	// TW tournament abbreviations (EventTeams data)
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
	"CAL":  "cal-poly",
	"CAMP": "campbell",
	"CENT": "central-michigan",
	"CHAT": "chattanooga",
	"CLEV": "cleveland-state",
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
	"SDSU": "south-dakota-state",
	"SHU":  "sacred-heart",
	"STAN": "stanford",
	"WYOM": "wyoming",
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

func tim() string {
	return fmt.Sprintf("%d", time.Now().UnixMilli())
}

func addBrowserHeaders(req *http.Request) {
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Connection", "keep-alive")
}

var twSessionRe = regexp.MustCompile(`twSessionId=([a-zA-Z0-9]+)`)
var mainFrameActionRe = regexp.MustCompile(`action\s*=\s*["']\.?/?MainFrame\.jsp\?([^"']+)["']`)

type sessionState struct {
	client    *http.Client
	sessionID string
	mfURL     string
}

// establishSession runs the 3-step TW session setup for the /tw/seasons/ subsystem.
func establishSession(seasonID, gbID, bootstrapTeamID string) (*sessionState, error) {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 30 * time.Second}

	indexURL := fmt.Sprintf("%s/index.jsp?TIM=%s", twSeasonsBase, tim())
	req1, _ := http.NewRequest("GET", indexURL, nil)
	addBrowserHeaders(req1)
	resp1, err := client.Do(req1)
	if err != nil {
		return nil, fmt.Errorf("get index: %w", err)
	}
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()

	m := twSessionRe.FindSubmatch(body1)
	if len(m) < 2 {
		return nil, fmt.Errorf("could not extract twSessionId from index page")
	}
	sessionID := string(m[1])
	log.Printf("session: %s", sessionID)

	lbURL := fmt.Sprintf(
		"%s/LoadBalance.jsp?TIM=%s&twSessionId=%s&seasonId=%s&gbId=%s&pageName=EventMatches.jsp&teamId=%s&uname=&pword=",
		twSeasonsBase, tim(), sessionID, seasonID, gbID, bootstrapTeamID,
	)
	req2, _ := http.NewRequest("GET", lbURL, nil)
	req2.Header.Set("Referer", indexURL)
	addBrowserHeaders(req2)
	resp2, err := client.Do(req2)
	if err != nil {
		return nil, fmt.Errorf("get loadbalance: %w", err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()

	mfM := mainFrameActionRe.FindSubmatch(body2)
	if len(mfM) < 2 {
		return nil, fmt.Errorf("could not find MainFrame.jsp action in LoadBalance response")
	}
	mfURL := twSeasonsBase + "/MainFrame.jsp?" + string(mfM[1])

	fv := url.Values{}
	fv.Set("seasonId", seasonID)
	fv.Set("gbId", gbID)
	fv.Set("teamId", bootstrapTeamID)
	fv.Set("pageName", "EventMatches.jsp")
	fv.Set("uname", "")
	fv.Set("pword", "")
	fv.Set("myTrackId", "")
	fv.Set("myTrackPW", "")
	fv.Set("hideFrame", "")
	fv.Set("wrestlerId", "")
	fv.Set("dualId", "")
	fv.Set("passcode", "")
	fv.Set("eventId", "")

	req3, _ := http.NewRequest("POST", mfURL, strings.NewReader(fv.Encode()))
	req3.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req3.Header.Set("Referer", lbURL)
	addBrowserHeaders(req3)
	resp3, err := client.Do(req3)
	if err != nil {
		return nil, fmt.Errorf("post mainframe: %w", err)
	}
	io.Copy(io.Discard, resp3.Body)
	resp3.Body.Close()

	return &sessionState{client: client, sessionID: sessionID, mfURL: mfURL}, nil
}

// teamEntry holds team info extracted from EventTeams.jsp.
type teamEntry struct {
	regularTeamID string // season-level team ID used by getEventMatchesJSP
	name          string
	abbr          string
}

// fetchEventTeams returns all participating teams for a tournament.
func fetchEventTeams(ss *sessionState, eventID string) ([]teamEntry, error) {
	etURL := fmt.Sprintf(
		"%s/EventTeams.jsp?TIM=%s&twSessionId=%s&eventId=%s",
		twSeasonsBase, tim(), ss.sessionID, eventID,
	)
	req, _ := http.NewRequest("GET", etURL, nil)
	req.Header.Set("Referer", ss.mfURL)
	addBrowserHeaders(req)
	resp, err := ss.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get EventTeams: %w", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	log.Printf("EventTeams.jsp: %d bytes, status %s", len(body), resp.Status)

	// Extract the JSON embedded in the initDataGrid JS call.
	// The call looks like: initDataGrid(50, true, "[[...]]", ...)
	// The JSON is a flat 2D array (no deeper nesting) so the first "]]"
	// after the opening "[[" reliably terminates the array.
	// We anchor the search to the initDataGrid call to skip any earlier
	// JSON literals in the page (e.g. rounds or weight lists).
	pageStr := string(body)
	gridIdx := strings.Index(pageStr, "initDataGrid(")
	if gridIdx < 0 {
		return nil, fmt.Errorf("no initDataGrid call found in EventTeams.jsp")
	}
	after := pageStr[gridIdx:]
	start := strings.Index(after, `"[[`)
	if start < 0 {
		return nil, fmt.Errorf("no JSON array found in initDataGrid call in EventTeams.jsp")
	}
	tail := after[start+3:] // content after opening "[["
	end := strings.Index(tail, `]]"`)
	if end < 0 {
		return nil, fmt.Errorf("malformed initDataGrid in EventTeams.jsp: no closing ]]")
	}
	raw := "[[" + tail[:end] + "]]"
	jsonStr := strings.ReplaceAll(raw, `\"`, `"`)

	var grid [][]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &grid); err != nil {
		return nil, fmt.Errorf("unmarshal EventTeams: %w", err)
	}

	teams := make([]teamEntry, 0, len(grid))
	for _, row := range grid {
		if len(row) < 11 {
			continue
		}
		teams = append(teams, teamEntry{
			regularTeamID: str(row[3]), // [3] and [7] are both the regular season teamId
			name:          str(row[1]),
			abbr:          str(row[10]),
		})
	}
	log.Printf("found %d teams in event %s", len(teams), eventID)
	return teams, nil
}

type boutRow []interface{}

// fetchTeamMatches fetches all bout results for one team in a tournament.
func fetchTeamMatches(ss *sessionState, eventID, teamID string) ([]boutRow, error) {
	ajaxURL := fmt.Sprintf(
		"%s/AjaxFunctions.jsp?TIM=%s&twSessionId=%s&function=getEventMatchesJSP&eventId=%s&teamId=%s",
		twSeasonsBase, tim(), ss.sessionID, eventID, teamID,
	)
	req, _ := http.NewRequest("GET", ajaxURL, nil)
	req.Header.Set("Referer", ss.mfURL)
	addBrowserHeaders(req)
	resp, err := ss.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getEventMatchesJSP: %w", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	trimmed := strings.TrimSpace(string(body))
	if trimmed == "null" || len(trimmed) == 0 {
		return nil, nil
	}
	if trimmed[0] != '[' {
		return nil, fmt.Errorf("unexpected response: %s", trimmed[:min(100, len(trimmed))])
	}

	var grid [][]interface{}
	if err := json.Unmarshal([]byte(trimmed), &grid); err != nil {
		return nil, fmt.Errorf("unmarshal matches: %w", err)
	}
	result := make([]boutRow, len(grid))
	for i, row := range grid {
		result[i] = row
	}
	return result, nil
}

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

type eventMeta struct {
	EventName string
	EventDate string // YYYY-MM-DD
}

func writeCSV(w io.Writer, matches map[string]boutRow, meta eventMeta, eventID string, seasonYear int) error {
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{
		"season_year", "event_name", "event_date", "event_type", "event_id",
		"weight_label", "round_name",
		"wrestler_a_name", "wrestler_a_school_slug",
		"wrestler_b_name", "wrestler_b_school_slug",
		"winner_name", "result_method",
		"score_winner", "score_loser", "match_time", "source_match_id",
	})

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
		sourceMatchID := str(row[0])

		_ = cw.Write([]string{
			strconv.Itoa(seasonYear),
			meta.EventName,
			meta.EventDate,
			"tournament",
			eventID,
			weight,
			roundName,
			w1Name, w1School,
			w2Name, w2School,
			winnerName,
			method,
			strconv.Itoa(scoreWinner),
			strconv.Itoa(scoreLoser),
			matchTime,
			sourceMatchID,
		})
	}

	cw.Flush()
	return cw.Error()
}

func main() {
	eventID := flag.String("event", "", "TrackWrestling event ID (required)")
	seasonYear := flag.Int("season", 2026, "Season end year (e.g. 2026 for 2025-26)")
	outPath := flag.String("out", "", "Output CSV path (default: stdout)")
	seasonID := flag.String("seasonid", defaultSeasonID, "TrackWrestling internal season ID")
	gbID := flag.String("gbid", defaultGbID, "TrackWrestling governing body ID")
	eventName := flag.String("event-name", "", "Event name override")
	eventDate := flag.String("event-date", "", "Event date override YYYY-MM-DD")
	flag.Parse()

	if *eventID == "" {
		log.Fatal("missing required -event argument")
	}

	// Air Force's regular-season teamId is used to bootstrap the /tw/seasons/
	// session; any valid D1 season teamId works here.
	const bootstrapTeamID = "758758150"

	ss, err := establishSession(*seasonID, *gbID, bootstrapTeamID)
	if err != nil {
		log.Fatalf("session: %v", err)
	}

	teams, err := fetchEventTeams(ss, *eventID)
	if err != nil {
		log.Fatalf("EventTeams: %v", err)
	}

	name := *eventName
	if name == "" {
		name = fmt.Sprintf("Tournament %s", *eventID)
	}
	meta := eventMeta{EventName: name, EventDate: *eventDate}

	// Collect all matches, deduplicating by source match ID since the same
	// match appears in each team's result set.
	allMatches := make(map[string]boutRow)
	for i, team := range teams {
		rows, err := fetchTeamMatches(ss, *eventID, team.regularTeamID)
		if err != nil {
			log.Printf("team %s (%s): %v — skipping", team.name, team.regularTeamID, err)
			continue
		}
		added := 0
		for _, row := range rows {
			if len(row) == 0 {
				continue
			}
			matchID := str(row[0])
			if _, exists := allMatches[matchID]; !exists {
				allMatches[matchID] = row
				added++
			}
		}
		log.Printf("[%d/%d] %-30s %3d bouts, %3d new (unique total: %d)",
			i+1, len(teams), team.name, len(rows), added, len(allMatches))

		time.Sleep(150 * time.Millisecond)
	}

	var w io.Writer = os.Stdout
	if *outPath != "" {
		f, err := os.Create(*outPath)
		if err != nil {
			log.Fatalf("create output: %v", err)
		}
		defer f.Close()
		w = f
	}

	if err := writeCSV(w, allMatches, meta, *eventID, *seasonYear); err != nil {
		log.Fatalf("write csv: %v", err)
	}

	fmt.Fprintf(os.Stderr, "wrote %d unique bouts\n", len(allMatches))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
