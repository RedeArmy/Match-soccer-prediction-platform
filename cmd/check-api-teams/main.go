// Command check-api-teams fetches all fixture team names from the api-football
// provider for the 2026 FIFA World Cup and compares them against the team names
// stored in the local matches table.
//
// Usage:
//
//	FOOTBALL_API_KEY=<key> go run ./cmd/check-api-teams
//
// Output: a table of provider team names vs DB team names, flagging any mismatch
// that would prevent auto-linking via DailyFixtureSync.
package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/rede/world-cup-quiniela/pkg/footballprovider"
)

const (
	leagueID = 1 // FIFA World Cup
	season   = 2026
)

func main() {
	apiKey := os.Getenv("FOOTBALL_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("WCQ_FOOTBALLPROVIDER_APIKEY")
	}
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "error: set FOOTBALL_API_KEY or WCQ_FOOTBALLPROVIDER_APIKEY")
		os.Exit(1)
	}

	client := footballprovider.NewAPIFootballClient(apiKey, "", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Fetch today + tomorrow to cover evening fixtures under UTC+0.
	now := time.Now().UTC()
	seen := make(map[string]struct{})
	allTeams := []string{}

	for _, d := range []string{
		now.Format("2006-01-02"),
		now.AddDate(0, 0, 1).Format("2006-01-02"),
		now.AddDate(0, 0, -1).Format("2006-01-02"),
	} {
		fixtures, err := client.GetFixturesByDate(ctx, leagueID, season, d)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warn: GetFixturesByDate(%s): %v\n", d, err)
			continue
		}
		for _, f := range fixtures {
			for _, name := range []string{f.HomeTeam, f.AwayTeam} {
				if _, ok := seen[name]; !ok {
					seen[name] = struct{}{}
					allTeams = append(allTeams, name)
				}
			}
		}
	}

	if len(allTeams) == 0 {
		fmt.Println("No fixtures found for today/yesterday/tomorrow. Try querying a specific date.")
		cancel()
		return
	}

	// DB canonical names (from migrations/000167_seed_teams.up.sql + 000168 matches)
	dbNames := []string{
		"Mexico", "South Africa", "South Korea", "Czechia",
		"Canada", "Bosnia and Herzegovina", "Qatar", "Switzerland",
		"Brazil", "Morocco", "Haiti", "Scotland",
		"United States", "Paraguay", "Australia", "Türkiye",
		"Germany", "Curaçao", "Ivory Coast", "Ecuador",
		"Netherlands", "Japan", "Sweden", "Tunisia",
		"Belgium", "Egypt", "Iran", "New Zealand",
		"Spain", "Cape Verde", "Saudi Arabia", "Uruguay",
		"France", "Senegal", "Iraq", "Norway",
		"Argentina", "Algeria", "Austria", "Jordan",
		"Portugal", "DR Congo", "Uzbekistan", "Colombia",
		"England", "Croatia", "Ghana", "Panama",
	}

	dbSet := make(map[string]struct{}, len(dbNames))
	for _, n := range dbNames {
		dbSet[strings.ToLower(n)] = struct{}{}
	}

	sort.Strings(allTeams)

	fmt.Println("=== API Team Names vs DB Canonical Names ===")
	fmt.Println()
	mismatches := 0
	for _, apiName := range allTeams {
		if _, ok := dbSet[strings.ToLower(apiName)]; ok {
			fmt.Printf("  OK       %s\n", apiName)
		} else {
			fmt.Printf("  MISMATCH %s  ← not found in DB (add alias)\n", apiName)
			mismatches++
		}
	}

	fmt.Printf("\nTotal API teams seen today: %d  |  Mismatches: %d\n", len(allTeams), mismatches)
}
