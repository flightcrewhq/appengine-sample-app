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
	pool   *util.ThreadPool
	params SimParams

	mu      sync.RWMutex
	metrics []map[reqType]*reqMetrics
	rnd     *rand.Rand
}

type SimParams struct {
	BaseURL      string
	Concurrency  int
	MaxUserIndex int
}

type reqMetrics struct {
	avgLatencyMs int
	requests     int
	errors       int
}

func NewSimulation(params SimParams) *Simulation {
	return &Simulation{
		client:  util.NewHttpClient(),
		pool:    util.NewThreadPool(params.Concurrency),
		params:  params,
		metrics: make([]map[reqType]*reqMetrics, params.MaxUserIndex),
		rnd:     rand.New(rand.NewSource(time.Now().Unix())),
	}
}

func (s *Simulation) Run(ctx context.Context) {
	log.Printf("Beginning simulation")
	i := 0
	for {
		x := i
		if err := s.pool.Run(ctx, func() error {
			s.executeEvent(x % s.params.MaxUserIndex)
			return nil
		}); err != nil {
			log.Printf("Run error: %v", err)
			break
		}
		i++
	}

	_ = s.pool.Join(ctx)
	s.printStats()
}

func (s *Simulation) printStats() {
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
