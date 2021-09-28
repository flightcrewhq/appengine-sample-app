package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"
)

func main() {
	simulateCmd := flag.NewFlagSet("simulate", flag.ExitOnError)
	simFeedURL := simulateCmd.String("url", "https://feed-dot-fifth-marker-318421.uc.r.appspot.com/", "Base url for the web app.")
	simMinutes := simulateCmd.Int("m", 30, "How many minutes to run for.")
	simConcurrency := simulateCmd.Int("c", 5, "How many threads to use.")
	simUsers := simulateCmd.Int("u", 100, "Number of users to hit.")

	if len(os.Args) < 2 {
		log.Fatal("Need a subcommand.")
	}

	switch os.Args[1] {
	case simulateCmd.Name():
		simulateCmd.Parse(os.Args[2:])

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*simMinutes)*time.Minute)
		defer cancel()

		sim := NewSimulation(SimParams{
			BaseURL:      *simFeedURL,
			Concurrency:  *simConcurrency,
			MaxUserIndex: *simUsers,
		})

		sim.Run(ctx)

	default:
		log.Fatal("Not a recognized command.")
	}

	log.Printf("Exited successfully")
}
