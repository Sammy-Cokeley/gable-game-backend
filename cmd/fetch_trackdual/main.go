// fetch_trackdual fetches bout results for a TrackWrestling dual meet and
// writes a CSV in the format expected by the ingestion pipeline.
//
// Session flow (all under trackwrestling.com/seasons/):
//  1. GET index.jsp               — obtain server-generated twSessionId
//  2. GET LoadBalance.jsp         — obtain MainFrame.jsp action URL
//  3. POST MainFrame.jsp          — establish season context in server session
//  4. GET AjaxFunctions.jsp       — fetch bout data as JSON (no teamId required)
//
// Usage:
//
//	go run ./cmd/fetch_trackdual -dual 8717428132 -season 2026 -out results.csv
//	go run ./cmd/fetch_trackdual -dual 8717428132 -event-name "Oklahoma State vs Iowa" -event-date 2026-02-22
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

// Governing body ID for College Men on TrackWrestling.
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

func main() {
	dualID := flag.String("dual", "", "TrackWrestling dual meet ID (required)")
	seasonYear := flag.Int("season", 2026, "Season end year (e.g. 2026 for 2025-26)")
	outPath := flag.String("out", "", "Output CSV path (default: stdout)")
	seasonID := flag.String("seasonid", defaultSeasonID, "TrackWrestling internal season ID")
	gbID := flag.String("gbid", defaultGbID, "TrackWrestling governing body ID")
	eventName := flag.String("event-name", "", "Event name override (e.g. 'Oklahoma State vs Iowa')")
	eventDate := flag.String("event-date", "", "Event date override YYYY-MM-DD")
	flag.Parse()

	if *dualID == "" {
		log.Fatal("missing required -dual argument")
	}

	rows, err := fetchDual(*dualID, *seasonID, *gbID)
	if err != nil {
		log.Fatalf("fetch: %v", err)
	}

	// Derive event name from the data if not overridden.
	name := *eventName
	if name == "" && len(rows) > 0 {
		team1 := str(rows[0][23])
		team2 := str(rows[0][27])
		name = team1 + " vs " + team2
	}

	meta := dualMeta{EventName: name, EventDate: *eventDate}

	var w io.Writer = os.Stdout
	if *outPath != "" {
		f, err := os.Create(*outPath)
		if err != nil {
			log.Fatalf("create output: %v", err)
		}
		defer f.Close()
		w = f
	}

	if err := writeCSV(w, rows, meta, *dualID, *seasonYear); err != nil {
		log.Fatalf("write csv: %v", err)
	}

	fmt.Fprintf(os.Stderr, "wrote %d bouts\n", len(rows))
}

type boutRow []interface{}

type dualMeta struct {
	EventName string
	EventDate string // YYYY-MM-DD
}

func addBrowserHeaders(req *http.Request) {
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Connection", "keep-alive")
}

var twSessionRe = regexp.MustCompile(`twSessionId=([a-zA-Z0-9]+)`)
var mainFrameActionRe = regexp.MustCompile(`action\s*=\s*"\./MainFrame\.jsp\?([^"]+)"`)

func fetchDual(dualID, seasonID, gbID string) ([]boutRow, error) {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 30 * time.Second}

	// Step 1: GET index.jsp — obtain the server-generated twSessionId.
	indexURL := fmt.Sprintf("https://www.trackwrestling.com/seasons/index.jsp?TIM=%s", tim())
	req1, _ := http.NewRequest("GET", indexURL, nil)
	addBrowserHeaders(req1)
	resp1, err := client.Do(req1)
	if err != nil {
		return nil, fmt.Errorf("get index: %w", err)
	}
	indexBody, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()

	m := twSessionRe.FindSubmatch(indexBody)
	if len(m) < 2 {
		return nil, fmt.Errorf("could not extract twSessionId from index page")
	}
	sessionID := string(m[1])
	log.Printf("session: %s", sessionID)

	// Step 2: GET LoadBalance.jsp — returns the MainFrame.jsp action URL.
	lbURL := fmt.Sprintf(
		"https://www.trackwrestling.com/seasons/LoadBalance.jsp?TIM=%s&twSessionId=%s&seasonId=%s&gbId=%s&uname=&pword=",
		tim(), sessionID, seasonID, gbID,
	)
	req2, _ := http.NewRequest("GET", lbURL, nil)
	req2.Header.Set("Referer", indexURL)
	addBrowserHeaders(req2)
	resp2, err := client.Do(req2)
	if err != nil {
		return nil, fmt.Errorf("get loadbalance: %w", err)
	}
	lbBody, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()

	mfM := mainFrameActionRe.FindSubmatch(lbBody)
	if len(mfM) < 2 {
		return nil, fmt.Errorf("could not find MainFrame.jsp action in LoadBalance response")
	}
	mfURL := "https://www.trackwrestling.com/seasons/MainFrame.jsp?" + string(mfM[1])

	// Step 3: POST to MainFrame.jsp — establishes season context in the server session.
	formVals := "seasonId=" + seasonID + "&gbId=" + gbID +
		"&uname=&pword=&myTrackId=&myTrackPW=&pageName=&hideFrame=&wrestlerId=&dualId=&teamId=&passcode=&eventId="
	req3, _ := http.NewRequest("POST", mfURL, strings.NewReader(formVals))
	req3.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req3.Header.Set("Referer", lbURL)
	addBrowserHeaders(req3)
	resp3, err := client.Do(req3)
	if err != nil {
		return nil, fmt.Errorf("post mainframe: %w", err)
	}
	io.Copy(io.Discard, resp3.Body)
	resp3.Body.Close()

	// Step 4: GET AjaxFunctions.jsp — returns bout data as a JSON array.
	// No teamId is required; dualId is sufficient to identify the meet.
	ajaxURL := fmt.Sprintf(
		"https://www.trackwrestling.com/seasons/AjaxFunctions.jsp?TIM=%s&twSessionId=%s&function=getDualMatchesJSP&dualId=%s",
		tim(), sessionID, dualID,
	)
	req4, _ := http.NewRequest("GET", ajaxURL, nil)
	req4.Header.Set("Referer", mfURL)
	addBrowserHeaders(req4)
	resp4, err := client.Do(req4)
	if err != nil {
		return nil, fmt.Errorf("get ajax: %w", err)
	}
	ajaxBody, err := io.ReadAll(resp4.Body)
	resp4.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("read ajax body: %w", err)
	}
	log.Printf("ajax: %s %d bytes", resp4.Status, len(ajaxBody))

	return parseJSON(ajaxBody)
}

func parseJSON(data []byte) ([]boutRow, error) {
	// Trim whitespace / BOM that TrackWrestling sometimes prepends.
	data = []byte(strings.TrimSpace(string(data)))
	if len(data) == 0 || data[0] != '[' {
		return nil, fmt.Errorf("unexpected response (want JSON array): %q", string(data[:min(200, len(data))]))
	}

	var grid [][]interface{}
	if err := json.Unmarshal(data, &grid); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	result := make([]boutRow, len(grid))
	for i, row := range grid {
		result[i] = row
	}
	return result, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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

func writeCSV(w io.Writer, rows []boutRow, meta dualMeta, dualID string, seasonYear int) error {
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{
		"season_year", "event_name", "event_date", "event_type", "dual_id",
		"weight_label", "bout_number",
		"wrestler_a_name", "wrestler_a_school_slug",
		"wrestler_b_name", "wrestler_b_school_slug",
		"winner_name", "result_method",
		"score_winner", "score_loser", "match_time", "source_match_id",
	})

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
			sourceMatchID = fmt.Sprintf("TW-%s-%s", dualID, weight)
		}

		_ = cw.Write([]string{
			strconv.Itoa(seasonYear),
			meta.EventName,
			meta.EventDate,
			"dual",
			dualID,
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

	cw.Flush()
	return cw.Error()
}
