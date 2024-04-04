package main

import (
	"context"
	"flag"
	"log"
	"os"
	"strconv"
	"time"
)

var (
	envThreads = "SIM_THREADS"
	// Type of traffic to simulate, options: [ max, cyclical, bursty ]
	envTrafficType = "SIM_TRAFFIC_TYPE"
)

var (
	feedURL     = flag.String("url", "", "Base url for the web app.")
	simDuration = flag.Duration("d", 30*time.Minute, "How long to run for.")
)

func main() {
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if *feedURL == "" {
		log.Fatalf("No input URL to hit.")
	}

	threadsStr := os.Getenv(envThreads)
	threads, err := strconv.Atoi(threadsStr)
	if err != nil {
		threads = 5
	}

	sim := NewSimulation(SimParams{
		BaseURL:      *feedURL,
		Concurrency:  threads,
		MaxUserIndex: 100,
		Length:       *simDuration,
	})

	switch os.Getenv(envTrafficType) {
	case "cyclical":
		sim.RunCyclical(ctx)
	case "bursty":
		sim.RunBursty(ctx)
	case "max":
		fallthrough
	default:
		sim.RunFlat(ctx)
	}

	sim.PrintStats()

	log.Printf("Exited successfully")
}
