// Command simulate runs a simulation scenario against a live Ripls server.
//
// The server must be running in dev mode (-dev-mode) with simulation clock
// middleware enabled. The command registers users, creates communities and
// gear, then executes a deterministic timeline of multi-user activity.
//
// Usage:
//
//	go run ./server/cmd/simulate -url http://localhost:8080
//	go run ./server/cmd/simulate -url http://localhost:8080 -scenario college-friends -seed 123
//	go run ./server/cmd/simulate -url http://localhost:8080 -list
//	go run ./server/cmd/simulate -url http://localhost:8080 -purge
//	go run ./server/cmd/simulate -url http://localhost:8080 -report /tmp/sim-report.html
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
	"go.ripls.org/ripls/server/simulation"
)

func main() {
	serverURL := flag.String("url", "", "Server base URL (required, e.g. http://localhost:8080)")
	scenarioName := flag.String("scenario", "suburban-neighborhood", "Scenario name to run")
	seed := flag.Int64("seed", 42, "PRNG seed for reproducibility")
	timeout := flag.Duration("timeout", 0, "Maximum run time (0 = no limit)")
	assetsDir := flag.String("assets-dir", "", "Path to simulation image assets (required, e.g. server/simulation/assets)")
	reportPath := flag.String("report", "", "Path to write HTML report (e.g. /tmp/sim-report.html)")
	listScenarios := flag.Bool("list", false, "List available scenarios and exit")
	purge := flag.Bool("purge", false, "Remove all simulation data and exit")
	verbose := flag.Bool("v", false, "Enable debug logging")
	readTraffic := flag.Bool("read-traffic", false, "Enable periodic read actions (feed checks, browsing, search)")
	readMultiplier := flag.Float64("read-multiplier", 1.0, "Scale read frequency (2.0 = double reads)")
	concurrency := flag.Int("concurrency", 0, "Number of concurrent user goroutines (0 = sequential)")

	flag.Parse()

	if *listScenarios {
		printScenarios()
		return
	}

	if *serverURL == "" {
		fmt.Fprintln(os.Stderr, "Error: -url is required")
		fmt.Fprintln(os.Stderr)
		flag.Usage()
		os.Exit(1)
	}

	if *verbose {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})))
	}

	if *purge {
		purgeAllSimulations(*serverURL)
		return
	}

	if *assetsDir == "" {
		fmt.Fprintln(os.Stderr, "Error: -assets-dir is required")
		fmt.Fprintln(os.Stderr)
		flag.Usage()
		os.Exit(1)
	}

	scenario, err := findScenario(*scenarioName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintln(os.Stderr, "Run with -list to see available scenarios.")
		os.Exit(1)
	}

	cfg := simulation.RunConfig{
		ServerURL:      *serverURL,
		Scenario:       scenario,
		Seed:           *seed,
		AssetsDir:      *assetsDir,
		ReadTraffic:    *readTraffic,
		ReadMultiplier: *readMultiplier,
		Concurrency:    *concurrency,
	}

	ctx := context.Background()
	if *timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *timeout)
		defer cancel()
	}

	// Allow graceful shutdown on Ctrl+C.
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	fmt.Fprintf(os.Stderr, "Running scenario %q against %s (seed=%d)\n", scenario.Name, *serverURL, *seed)
	fmt.Fprintf(os.Stderr, "  Communities: %d, Members: %d\n", len(scenario.Communities), totalMembers(scenario))
	fmt.Fprintf(os.Stderr, "  Simulated period: %s to %s\n",
		scenario.StartTime.Format("2006-01-02"),
		scenario.EndTime.Format("2006-01-02"))
	if *concurrency > 0 {
		fmt.Fprintf(os.Stderr, "  Concurrency: %d goroutines\n", *concurrency)
	}
	fmt.Fprintln(os.Stderr)

	result, err := simulation.RunSimulation(ctx, cfg)
	if err != nil {
		log.Fatalf("Simulation failed: %v", err)
	}

	printResult(result, scenario)

	if *reportPath != "" {
		if err := writeReport(*reportPath, result, scenario, *seed); err != nil {
			log.Fatalf("Report generation failed: %v", err)
		}
		fmt.Fprintf(os.Stderr, "Report written to %s\n", *reportPath)
	}
}

// writeReport generates an HTML report file from the simulation results.
func writeReport(path string, result *simulation.RunResult, scenario simulation.Scenario, seed int64) error {
	report := simulation.BuildReport(result, scenario, seed)
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create report file: %w", err)
	}
	defer f.Close()
	if err := simulation.WriteReportHTML(f, report); err != nil {
		return fmt.Errorf("write report HTML: %w", err)
	}
	return nil
}

// findScenario returns the scenario matching the given name.
func findScenario(name string) (simulation.Scenario, error) {
	for _, s := range simulation.AllScenarios() {
		if s.Name == name {
			return s, nil
		}
	}

	var names []string
	for _, s := range simulation.AllScenarios() {
		names = append(names, s.Name)
	}
	return simulation.Scenario{}, fmt.Errorf("unknown scenario %q (available: %s)", name, strings.Join(names, ", "))
}

// printScenarios lists all available scenarios.
func printScenarios() {
	fmt.Println("Available scenarios:")
	fmt.Println()
	for _, s := range simulation.AllScenarios() {
		members := totalMembers(s)
		days := int(s.EndTime.Sub(s.StartTime).Hours() / 24)
		fmt.Printf("  %-25s %d communities, %d members, %d days\n",
			s.Name, len(s.Communities), members, days)
		fmt.Printf("  %s\n\n", s.Description)
	}
}

// totalMembers counts all members across all communities.
func totalMembers(s simulation.Scenario) int {
	n := 0
	for _, c := range s.Communities {
		n += len(c.Members)
	}
	return n
}

// purgeAllSimulations lists all simulation runs and cleans up each one.
func purgeAllSimulations(serverURL string) {
	ctx := context.Background()
	client := apiconnect.NewAdminServiceClient(http.DefaultClient, serverURL)

	listResp, err := client.ListSimulations(ctx, connect.NewRequest(&api.ListSimulationsRequest{}))
	if err != nil {
		log.Fatalf("ListSimulations failed: %v", err)
	}

	sims := listResp.Msg.Simulations
	if len(sims) == 0 {
		fmt.Println("No simulation data found.")
		return
	}

	fmt.Printf("Found %d simulation(s):\n", len(sims))
	for _, s := range sims {
		fmt.Printf("  %-35s %d users, %d communities\n", s.SimulationId, s.UserCount, s.CommunityCount)
	}
	fmt.Println()

	var totalUsers, totalCommunities, totalRecords int64
	for _, s := range sims {
		resp, err := client.CleanupSimulation(ctx, connect.NewRequest(&api.CleanupSimulationRequest{
			SimulationId: s.SimulationId,
		}))
		if err != nil {
			log.Fatalf("CleanupSimulation(%s) failed: %v", s.SimulationId, err)
		}
		fmt.Printf("  Cleaned %s: %d users, %d communities, %d records (%dms)\n",
			s.SimulationId, resp.Msg.UsersDeleted, resp.Msg.CommunitiesDeleted,
			resp.Msg.RecordsDeleted, resp.Msg.DurationMilliseconds)
		totalUsers += resp.Msg.UsersDeleted
		totalCommunities += resp.Msg.CommunitiesDeleted
		totalRecords += resp.Msg.RecordsDeleted
	}

	fmt.Println()
	fmt.Printf("Purge complete: %d users, %d communities, %d total records deleted.\n",
		totalUsers, totalCommunities, totalRecords)
}

// printResult prints a summary of the simulation results and login credentials.
func printResult(r *simulation.RunResult, scenario simulation.Scenario) {
	fmt.Println("Simulation complete!")
	fmt.Println()
	fmt.Printf("  Simulation ID:    %s\n", r.SimulationID)
	fmt.Printf("  Duration:         %s\n", r.Duration.Round(time.Millisecond))
	fmt.Printf("  Timeline steps:   %d\n", r.TimelineSteps)
	fmt.Println()
	fmt.Printf("  Users:            %d\n", r.UsersCreated)
	fmt.Printf("  Communities:      %d\n", r.CommunitiesCreated)
	fmt.Printf("  Gear:             %d\n", r.State.GearCreated)
	fmt.Printf("  Media uploaded:   %d\n", len(r.State.MediaIDs))
	fmt.Println()
	fmt.Println("  Transfers:")
	fmt.Printf("    Started:        %d\n", r.State.TransfersStarted)
	fmt.Printf("    Completed:      %d\n", r.State.TransfersCompleted)
	fmt.Printf("    Cancelled:      %d\n", r.State.TransfersCancelled)
	fmt.Printf("    Interest withdrawn: %d\n", r.State.InterestWithdrawn)
	fmt.Println()
	fmt.Println("  Requests:")
	fmt.Printf("    Submitted:      %d\n", r.State.RequestsSubmitted)
	fmt.Printf("    Fulfilled:      %d\n", r.State.RequestsFulfilled)
	fmt.Printf("    Cancelled:      %d\n", r.State.RequestsCancelled)
	fmt.Printf("    Offers withdrawn: %d\n", r.State.OffersWithdrawn)
	fmt.Println()
	fmt.Println("  Experiences:")
	fmt.Printf("    Created:        %d\n", r.State.ExperiencesCreated)
	fmt.Printf("    Completed:      %d\n", r.State.ExperiencesCompleted)
	fmt.Printf("    Cancelled:      %d\n", r.State.ExperiencesCancelled)
	fmt.Printf("    RSVPs (yes):    %d\n", r.State.RSVPYesCount)
	fmt.Printf("    RSVPs (no):     %d\n", r.State.RSVPNoCount)
	fmt.Println()
	fmt.Printf("  Chat messages:    %d\n", r.State.ChatMessagesSent)
	fmt.Printf("  Read actions:     %d\n", r.State.ReadsExecuted)
	fmt.Println()
	// No passwords: these accounts registered through the emailed one-time code
	// (#2571). Signing in as one means requesting a code for its address and
	// reading it back from RequestEmailCode's dev-mode echo — the mail itself is
	// never sent, since example.com is a reserved test domain.
	fmt.Println("  Simulated accounts (sign in with an emailed code, not a password):")
	fmt.Println()
	for _, comm := range scenario.Communities {
		fmt.Printf("  Community: %s\n", comm.Name)
		for _, m := range comm.Members {
			fmt.Printf("    %-25s %-35s (%s)\n", m.Name, m.Email, m.Persona)
		}
		fmt.Println()
	}
}
