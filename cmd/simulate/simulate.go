package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/url"
	"sort"
	"sync"
	"time"

	"holosam/appengine/demo/pkg/util"
)

const (
	cycleLength = 2 * time.Minute
)

var (
	rnd = rand.New(rand.NewSource(time.Now().Unix()))
)

type reqType int

const (
	USER reqType = iota
	PUBLISH
	FOLLOW
)

var reqTypes = []reqType{USER, PUBLISH, FOLLOW}

func (t reqType) createURL(baseURL string, args ...string) string {
	switch t {
	case USER:
		return baseURL + fmt.Sprintf("user/%s", args[0])
	case PUBLISH:
		return baseURL + fmt.Sprintf("publish?user=%s&text=%s", args[0], url.QueryEscape(args[1]))
	case FOLLOW:
		return baseURL + fmt.Sprintf("follow?src=%s&dst=%s", args[0], args[1])
	default:
		log.Fatalf("Unrecognized request type %d", t)
		return ""
	}
}

type Simulation struct {
	client *util.HttpClient
	params SimParams

	mu      sync.RWMutex
	metrics []map[reqType]*reqMetrics
	rnd     *rand.Rand
}

type SimParams struct {
	BaseURL      string
	Concurrency  int
	MaxUserIndex int
	Length       time.Duration
}

type reqMetrics struct {
	avgLatencyMs int
	requests     int
	errors       int
}

func NewSimulation(params SimParams) *Simulation {
	return &Simulation{
		client:  util.NewHttpClient(),
		params:  params,
		metrics: make([]map[reqType]*reqMetrics, params.MaxUserIndex),
		rnd:     rand.New(rand.NewSource(time.Now().Unix())),
	}
}

// Run until the context ends.
func (s *Simulation) RunFlat(ctx context.Context) {
	pool := util.NewThreadPool(s.params.Concurrency)

	iteration := 0
	for {
		userIndex := iteration
		if err := pool.Run(ctx, func() error {
			s.executeEvent(userIndex % s.params.MaxUserIndex)
			return nil
		}); err != nil {
			log.Printf("Context ended: %v", err)
			break
		}
		iteration++
	}

	// Context is ended but still calls "Wait()"
	pool.Join(ctx)
}

// Run a cyclical traffic pattern that goes up and down.
func (s *Simulation) RunCyclical(ctx context.Context) {
	iteration := 0
	threadsToUse := 1
	ascending := true

outer:
	for {
		// Quick non-blocking check to see if the entire simulation is over.
		select {
		case <-ctx.Done():
			log.Printf("Context ended: %v", ctx.Err())
			break outer
		default:
		}

		pool := util.NewThreadPool(threadsToUse)
		cycleCtx, cancel := context.WithTimeout(ctx, cycleLength)

		for {
			userIndex := iteration
			if err := pool.Run(cycleCtx, func() error {
				s.executeEvent(userIndex % s.params.MaxUserIndex)
				return nil
			}); err != nil {
				cancel()
				log.Printf("Cycle %d ended: %v", threadsToUse, err)
				break
			}
			iteration++
		}
		pool.Join(cycleCtx)

		if ascending && threadsToUse == s.params.Concurrency {
			ascending = false
		} else if !ascending && threadsToUse == 1 {
			ascending = true
		}

		if ascending {
			threadsToUse++
		} else {
			threadsToUse--
		}
	}
}

// Run a bursty traffic pattern that switches between min and max traffic at
// intervals.
func (s *Simulation) RunBursty(ctx context.Context) {
	iteration := 0
	threadsToUse := 1
	timeout := cycleLength

outer:
	for {
		select {
		case <-ctx.Done():
			log.Printf("Context ended: %v", ctx.Err())
			break outer
		default:
			// Don't block
		}

		pool := util.NewThreadPool(threadsToUse)
		cycleCtx, cancel := context.WithTimeout(ctx, timeout)

		for {
			userIndex := iteration
			if err := pool.Run(cycleCtx, func() error {
				s.executeEvent(userIndex % s.params.MaxUserIndex)
				return nil
			}); err != nil {
				cancel()
				log.Printf("Cycle %d ended: %v", threadsToUse, err)
				break
			}
			iteration++
		}
		pool.Join(cycleCtx)

		if threadsToUse == 1 {
			threadsToUse = s.params.Concurrency
			timeout = cycleLength
		} else {
			threadsToUse = 1
			timeout = cycleLength * time.Duration(rnd.Intn(4)+1)
		}
	}
}

func (s *Simulation) PrintStats() {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sort.Slice(s.metrics, func(i, j int) bool {
		return s.metrics[i][USER].avgLatencyMs < s.metrics[j][USER].avgLatencyMs
	})

	totals := make(map[reqType]*reqMetrics, len(reqTypes))
	for _, reqMap := range s.metrics {
		for reqType, metrics := range reqMap {
			// fmt.Printf("User #%d - req %d: %+v\n", userIndex, reqType, metrics)

			if _, ok := totals[reqType]; !ok {
				totals[reqType] = &reqMetrics{}
			}
			rm := totals[reqType]
			rm.requests += metrics.requests
			rm.errors += metrics.errors
			rm.avgLatencyMs = ((rm.avgLatencyMs * rm.requests) + (metrics.avgLatencyMs * metrics.requests)) / (rm.requests + metrics.requests)
		}
	}

	for reqType, metrics := range totals {
		fmt.Printf("Total - req %d: %+v\n", reqType, metrics)
	}
}

func (s *Simulation) executeEvent(userIndex int) {
	user := username(userIndex)

	s.recordRequest(USER, userIndex, USER.createURL(s.params.BaseURL, user))
	s.recordRequest(PUBLISH, userIndex, PUBLISH.createURL(s.params.BaseURL, user, "non randomized doc for now"))

	// Pick a user to follow.
	toFollow := -1
	s.mu.RLock()
	rndIndex := s.rnd.Intn(s.params.MaxUserIndex)
	for i := 0; i < s.params.MaxUserIndex; i++ {
		if len(s.metrics[(i+rndIndex)%s.params.MaxUserIndex]) != 0 {
			toFollow = i
			break
		}
	}
	s.mu.RUnlock()
	if toFollow == -1 {
		return
	}

	s.recordRequest(FOLLOW, userIndex, FOLLOW.createURL(s.params.BaseURL, user, username(toFollow)))
}

func (s *Simulation) recordRequest(t reqType, userIndex int, url string) {
	startTime := time.Now()
	_, err := s.client.Send(util.ReqOpts{
		Method: "GET",
		Url:    url,
	})
	elapsedTime := int(time.Since(startTime).Milliseconds())

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.metrics[userIndex]) == 0 {
		s.metrics[userIndex] = make(map[reqType]*reqMetrics, len(reqTypes))
	}
	if _, ok := s.metrics[userIndex][t]; !ok {
		s.metrics[userIndex][t] = &reqMetrics{}
	}

	rm := s.metrics[userIndex][t]
	rm.avgLatencyMs = ((rm.avgLatencyMs * rm.requests) + elapsedTime) / (rm.requests + 1)
	rm.requests++
	if err != nil {
		rm.errors++
	}
}

func username(userIndex int) string {
	return fmt.Sprintf("user%d", userIndex)
}
