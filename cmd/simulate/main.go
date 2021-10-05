package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"
)

// Switch to https://github.com/spf13/cobra.
func main() {
	simulateCmd := flag.NewFlagSet("simulate", flag.ExitOnError)
	simFeedURL := simulateCmd.String("url", "https://feed-dot-fifth-marker-318421.uc.r.appspot.com/", "Base url for the web app.")
	simDuration := simulateCmd.Duration("d", 30*time.Minute, "How long to run for.")
	simConcurrency := simulateCmd.Int("c", 5, "Max threads to use.")
	simUsers := simulateCmd.Int("u", 100, "Number of users to hit.")
	simType := simulateCmd.String("t", "max", "Type of traffic to simulate, options: [ max, cyclical, bursty ]")

	if len(os.Args) < 2 {
		log.Fatal("Need a subcommand.")
	}

	switch os.Args[1] {
	case simulateCmd.Name():
		simulateCmd.Parse(os.Args[2:])

		ctx, cancel := context.WithTimeout(context.Background(), *simDuration)
		defer cancel()

		sim := NewSimulation(SimParams{
			BaseURL:      *simFeedURL,
			Concurrency:  *simConcurrency,
			MaxUserIndex: *simUsers,
			Length:       *simDuration,
		})

		switch *simType {
		case "max":
			sim.RunFlat(ctx)
		case "cyclical":
			sim.RunCyclical(ctx)
		case "bursty":
			sim.RunBursty(ctx)
		default:
			log.Fatalf("Unrecognized type: %s", *simType)
		}

		sim.PrintStats()

	default:
		log.Fatalf("Unrecognized command: %s", os.Args[1])
	}

	log.Printf("Exited successfully")
}
