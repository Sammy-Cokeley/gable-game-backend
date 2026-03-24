// ingest_season fetches all Division I dual meet results for a TrackWrestling
// season and writes them to a single CSV in the format expected by the
// ingestion pipeline.
//
// Session flow (all under trackwrestling.com/seasons/):
//  1. GET index.jsp               — obtain server-generated twSessionId
//  2. GET LoadBalance.jsp         — obtain MainFrame.jsp action URL
//  3. POST MainFrame.jsp          — establish season context in server session
//  4. GET AjaxFunctions.jsp?function=getTeams        — list all D1 teams
//  5. GET AjaxFunctions.jsp?function=getTeamSchedule — per-team dual IDs
//  6. GET AjaxFunctions.jsp?function=getDualMatchesJSP — per-dual bout data
//
// Usage:
//
//	go run ./cmd/ingest_season -out season_2026.csv
//	go run ./cmd/ingest_season -seasonid 841725138 -season 2025 -out season_2025.csv
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
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// TrackWrestling season IDs for NCAA College Men — update each season.
// 2025-26: 1560238138  |  2024-25: 841725138
const defaultSeasonID = "1560238138"

// Governing body ID for NCAA College Men on TrackWrestling.
const defaultGbID = "2026076"

const browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

var schoolSlugOverrides = map[string]string{
	"OKST":  "oklahoma-state",
	"IOWA":  "iowa",
	"PSU":   "penn-state",
	"OSU":   "ohio-state",
	"NEB":   "nebraska",
	"MINN":  "minnesota",
	"MICH":  "michigan",
	"NWTN":  "northwestern",
	"ILL":   "illinois",
	"IND":   "indiana",
	"PURD":  "purdue",
	"MSU":   "michigan-state",
	"WISC":  "wisconsin",
	"UNI":   "northern-iowa",
	"ISU":   "iowa-state",
	"MIZZ":  "missouri",
	"OKLA":  "oklahoma",
	"WVU":   "west-virginia",
	"PITT":  "pittsburgh",
	"NCST":  "nc-state",
	"VT":    "virginia-tech",
	"UVA":   "virginia",
	"DUKE":  "duke",
	"UNC":   "north-carolina",
	"CLEM":  "clemson",
	"PRIN":  "princeton",
	"CORN":  "cornell",
	"LEHG":  "lehigh",
	"HOFP":  "hofstra",
	"APPST": "app-state",
	"ARIZ":  "arizona-state",
	"CAL":   "cal-poly",
	"CLEV":  "cleveland-state",
	"CSUB":  "csub",
	"SDSU":  "south-dakota-state",
	"WYOM":  "wyoming",
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

// dualInfo holds metadata about a dual meet from a team's schedule.
type dualInfo struct {
	ID        string // TrackWrestling dual/event ID
	Name      string // e.g. "Iowa, IA @ Oklahoma State, OK"
	Date      string // YYYY-MM-DD derived from YYYYMMDD
}

type boutRow []interface{}

func main() {
	seasonID := flag.String("seasonid", defaultSeasonID, "TrackWrestling internal season ID")
	gbID := flag.String("gbid", defaultGbID, "TrackWrestling governing body ID")
	seasonYear := flag.Int("season", 2026, "Season end year (e.g. 2026 for 2025-26)")
	outPath := flag.String("out", "season.csv", "Output CSV path")
	delayMs := flag.Int("delay", 300, "Milliseconds to wait between API requests")
	flag.Parse()

	delay := time.Duration(*delayMs) * time.Millisecond

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 30 * time.Second}

	log.Println("establishing session...")
	sessionID, mfURL, err := setupSession(client, *seasonID, *gbID)
	if err != nil {
		log.Fatalf("session: %v", err)
	}

	log.Println("fetching D1 team list...")
	time.Sleep(delay)
	teams, err := getD1Teams(client, sessionID, *seasonID, mfURL)
	if err != nil {
		log.Fatalf("get teams: %v", err)
	}
	log.Printf("found %d D1 teams", len(teams))

	// Collect unique dual IDs from every team's schedule.
	duals := map[string]dualInfo{}
	for i, team := range teams {
		teamID, _ := team[0].(string)
		time.Sleep(delay)
		schedule, err := getTeamSchedule(client, sessionID, teamID, *seasonID, mfURL)
		if err != nil {
			log.Printf("schedule for team %s: %v (skipping)", teamID, err)
			continue
		}
		for _, d := range schedule {
			if _, seen := duals[d.ID]; !seen {
				duals[d.ID] = d
			}
		}
		log.Printf("[%d/%d] team %s (%s): %d duals in schedule, %d unique so far",
			i+1, len(teams), teamID, team[1], len(schedule), len(duals))
	}
	log.Printf("total unique dual IDs: %d", len(duals))

	// Open output file.
	f, err := os.Create(*outPath)
	if err != nil {
		log.Fatalf("create output: %v", err)
	}
	defer f.Close()
	cw := csv.NewWriter(f)
	_ = cw.Write([]string{
		"season_year", "event_name", "event_date", "event_type", "dual_id",
		"weight_label", "bout_number",
		"wrestler_a_name", "wrestler_a_school_slug",
		"wrestler_b_name", "wrestler_b_school_slug",
		"winner_name", "result_method",
		"score_winner", "score_loser", "match_time", "source_match_id",
	})

	totalBouts := 0
	i := 0
	for _, dual := range duals {
		i++
		time.Sleep(delay)
		rows, err := getDualBouts(client, sessionID, dual.ID, mfURL)
		if err != nil {
			log.Printf("[%d/%d] dual %s: %v (skipping)", i, len(duals), dual.ID, err)
			continue
		}
		if len(rows) == 0 {
			log.Printf("[%d/%d] dual %s (%s): no bouts recorded", i, len(duals), dual.ID, dual.Name)
			continue
		}
		writeBouts(cw, rows, dual, *seasonYear)
		totalBouts += len(rows)
		log.Printf("[%d/%d] dual %s (%s %s): %d bouts", i, len(duals), dual.ID, dual.Date, dual.Name, len(rows))
	}

	cw.Flush()
	if err := cw.Error(); err != nil {
		log.Fatalf("csv flush: %v", err)
	}

	log.Printf("done — wrote %d bouts across %d duals to %s", totalBouts, len(duals), *outPath)
}

// --- Session setup ---

var twSessionRe = regexp.MustCompile(`twSessionId=([a-zA-Z0-9]+)`)
var mainFrameActionRe = regexp.MustCompile(`action\s*=\s*"\./MainFrame\.jsp\?([^"]+)"`)

func addBrowserHeaders(req *http.Request) {
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Connection", "keep-alive")
}

func doGet(client *http.Client, url, referer string) ([]byte, error) {
	req, _ := http.NewRequest("GET", url, nil)
	addBrowserHeaders(req)
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	b, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	return b, err
}

func setupSession(client *http.Client, seasonID, gbID string) (sessionID, mfURL string, err error) {
	indexURL := "https://www.trackwrestling.com/seasons/index.jsp?TIM=" + tim()
	indexBody, err := doGet(client, indexURL, "")
	if err != nil {
		return "", "", fmt.Errorf("get index: %w", err)
	}
	m := twSessionRe.FindSubmatch(indexBody)
	if len(m) < 2 {
		return "", "", fmt.Errorf("could not extract twSessionId from index page")
	}
	sessionID = string(m[1])

	lbURL := fmt.Sprintf(
		"https://www.trackwrestling.com/seasons/LoadBalance.jsp?TIM=%s&twSessionId=%s&seasonId=%s&gbId=%s&uname=&pword=",
		tim(), sessionID, seasonID, gbID,
	)
	lbBody, err := doGet(client, lbURL, indexURL)
	if err != nil {
		return "", "", fmt.Errorf("get loadbalance: %w", err)
	}
	mfM := mainFrameActionRe.FindSubmatch(lbBody)
	if len(mfM) < 2 {
		return "", "", fmt.Errorf("could not find MainFrame.jsp action in LoadBalance response")
	}
	mfURL = "https://www.trackwrestling.com/seasons/MainFrame.jsp?" + string(mfM[1])

	formVals := "seasonId=" + seasonID + "&gbId=" + gbID +
		"&uname=&pword=&myTrackId=&myTrackPW=&pageName=&hideFrame=&wrestlerId=&dualId=&teamId=&passcode=&eventId="
	req, _ := http.NewRequest("POST", mfURL, strings.NewReader(formVals))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", lbURL)
	addBrowserHeaders(req)
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("post mainframe: %w", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	log.Printf("session: %s", sessionID)
	return sessionID, mfURL, nil
}

// --- Team list ---

// getD1Teams fetches all NCAA Division I teams for the season.
// Each entry is a raw JSON array row; [0]=teamId, [1]=name.
func getD1Teams(client *http.Client, sessionID, seasonID, referer string) ([][]interface{}, error) {
	u := fmt.Sprintf(
		"https://www.trackwrestling.com/seasons/AjaxFunctions.jsp?TIM=%s&twSessionId=%s&function=getTeams&seasonId=%s",
		tim(), sessionID, seasonID,
	)
	body, err := doGet(client, u, referer)
	if err != nil {
		return nil, err
	}

	var all [][]interface{}
	if err := json.Unmarshal(trimJSON(body), &all); err != nil {
		return nil, fmt.Errorf("unmarshal teams: %w", err)
	}

	// Filter: gbId field [6]=="3" (NCAA) and division field [5] contains "DI "
	// but not "DII" or "DIII". We check for the literal " DI " or starts with "DI -".
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

// --- Team schedule ---

// getTeamSchedule returns all dual meets (type=0) from a team's schedule.
func getTeamSchedule(client *http.Client, sessionID, teamID, seasonID, referer string) ([]dualInfo, error) {
	u := fmt.Sprintf(
		"https://www.trackwrestling.com/seasons/AjaxFunctions.jsp?TIM=%s&twSessionId=%s&function=getTeamSchedule&teamId=%s&seasonId=%s",
		tim(), sessionID, teamID, seasonID,
	)
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

	var duals []dualInfo
	for _, row := range rows {
		if len(row) < 4 {
			continue
		}
		eventType, _ := row[1].(string)
		if eventType != "0" { // 0 = dual meet, 1 = tournament
			continue
		}
		id, _ := row[0].(string)
		name, _ := row[2].(string)
		dateRaw, _ := row[3].(string) // YYYYMMDD
		date := ""
		if len(dateRaw) == 8 {
			date = dateRaw[0:4] + "-" + dateRaw[4:6] + "-" + dateRaw[6:8]
		}
		duals = append(duals, dualInfo{ID: id, Name: name, Date: date})
	}
	return duals, nil
}

// --- Bout data ---

func getDualBouts(client *http.Client, sessionID, dualID, referer string) ([]boutRow, error) {
	u := fmt.Sprintf(
		"https://www.trackwrestling.com/seasons/AjaxFunctions.jsp?TIM=%s&twSessionId=%s&function=getDualMatchesJSP&dualId=%s",
		tim(), sessionID, dualID,
	)
	body, err := doGet(client, u, referer)
	if err != nil {
		return nil, err
	}

	data := trimJSON(body)
	if len(data) == 0 || data[0] != '[' {
		return nil, nil
	}

	var grid [][]interface{}
	if err := json.Unmarshal(data, &grid); err != nil {
		return nil, fmt.Errorf("unmarshal bouts: %w", err)
	}
	result := make([]boutRow, len(grid))
	for i, row := range grid {
		result[i] = row
	}
	return result, nil
}

// --- CSV output ---

func writeBouts(cw *csv.Writer, rows []boutRow, dual dualInfo, seasonYear int) {
	for i, row := range rows {
		if len(row) < 34 {
			continue
		}
		weight := str(row[2])
		w1Name := strings.TrimSpace(str(row[21]) + " " + str(row[22]))
		w2Name := strings.TrimSpace(str(row[25]) + " " + str(row[26]))
		w1School := schoolSlug(str(row[24]), str(row[23]))
		w2School := schoolSlug(str(row[28]), str(row[27]))

		// row[9] = "1" means w1 won; "2" means w2 won.
		// row[10] = w1 score, row[13] = w2 score (no swapping needed).
		winnerIdx := str(row[9])
		winnerName := w2Name
		if winnerIdx == "1" {
			winnerName = w1Name
		}
		w1Score, w2Score := intVal(row[10]), intVal(row[13])
		scoreWinner, scoreLoser := w2Score, w1Score
		if winnerIdx == "1" {
			scoreWinner, scoreLoser = w1Score, w2Score
		}

		matchTime := formatTime(intVal(row[16]))
		method := strings.ToUpper(strings.TrimSuffix(str(row[5]), "."))

		sourceMatchID := str(row[0])
		if sourceMatchID == "" {
			sourceMatchID = fmt.Sprintf("TW-%s-%s", dual.ID, weight)
		}

		_ = cw.Write([]string{
			strconv.Itoa(seasonYear),
			dual.Name,
			dual.Date,
			"dual",
			dual.ID,
			weight,
			strconv.Itoa(i + 1),
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
}

// --- Helpers ---

func trimJSON(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
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
