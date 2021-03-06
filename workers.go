//Licensed under the Apache License, Version 2.0 (the "License");
//you may not use this file except in compliance with the License.
//You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
//Unless required by applicable law or agreed to in writing, software
//distributed under the License is distributed on an "AS IS" BASIS,
//WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//See the License for the specific language governing permissions and
//limitations under the License.
package main

import (
	"fmt"
	"regexp"
	"sort"
	"time"

	"github.com/simonpasquier/http_log_console/pkg/atime"
)

// the following code comes from [1] because Golang has no built-in function
// for sorting maps by value.
// [1] https://groups.google.com/d/msg/golang-nuts/FT7cjmcL7gw/Gj4_aEsE_IsJ
//
// A data structure to hold a key/value pair.
type Pair struct {
	Key   string
	Value int
}

// A slice of Pairs that implements sort.Interface to sort by Value.
type PairList []Pair

func (p PairList) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p PairList) Len() int           { return len(p) }
func (p PairList) Less(i, j int) bool { return p[i].Value < p[j].Value }

// A function to turn a map into a PairList, then sort and return it.
func sortMapByValue(m map[string]int) PairList {
	p := make(PairList, len(m))
	i := 0
	for k, v := range m {
		p[i] = Pair{k, v}
	}
	sort.Sort(p)
	return p
}

// StatsWorker aggregates the number of hits over the given interval
type StatsWorker struct {
	// total number of hits
	totalHits int
	// the number of hits broken down by status code
	statusHits []int
	// the number of hits broken down by section
	sectionHits map[string]int
	// interval (in seconds) at which statistics are emitted
	interval int
	// channel for receiving the Hit values
	in chan *Hit
	// channel for sending out the statistics
	out chan []string
	// channel indicating that the application is done
	done   <-chan struct{}
	logger Logger
}

// Returns a new instance of StatsWorker
func NewStatsWorker(interval int, done <-chan struct{}, logger Logger) *StatsWorker {
	in := make(chan *Hit)
	out := make(chan []string)

	s := StatsWorker{
		logger:      logger,
		sectionHits: make(map[string]int),
		statusHits:  make([]int, 6),
		interval:    interval,
		in:          in,
		out:         out,
		done:        done,
	}

	go func() {
		defer close(out)
		ticker := time.NewTicker(time.Second * time.Duration(s.interval))
		defer ticker.Stop()
		section := regexp.MustCompile("^(?:/([^/]+)/)")
		for {
			select {
			case hit := <-s.in:
				skey := "/"
				if match := section.FindStringSubmatch(hit.uri); match != nil {
					skey = match[1]
				}
				s.sectionHits[skey] += 1
				status := hit.status / 100
				if status >= 0 && status < len(s.statusHits) {
					s.statusHits[status] += 1
				}
				s.totalHits += 1
			case <-ticker.C:
				stats := make([]string, 0)
				for _, p := range sortMapByValue(s.sectionHits) {
					stats = append(stats, fmt.Sprintf("'%s' section: %d hits", p.Key, p.Value))
					delete(s.sectionHits, p.Key)
				}
				for i, v := range s.statusHits[1:] {
					stats = append(stats, fmt.Sprintf("'%dxx': %d hits", i+1, v))
					s.statusHits[i] = 0
				}
				stats = append(stats, fmt.Sprintf("'other': %d hits", s.statusHits[0]))
				s.statusHits[0] = 0
				stats = append(stats, fmt.Sprintf("total: %d hits (%.02f/sec)", s.totalHits, float64(s.totalHits)/float64(s.interval)))
				s.totalHits = 0
				out <- stats
			case <-done:
				s.logger.Println("Exiting StatsWorker")
				return
			}
		}
	}()

	return &s
}

type Clocker interface {
	Now() uint64
}

// Implements a monitonic clock
type MonoClocker struct{}

func (MonoClocker) Now() uint64 {
	return atime.NanoTime()
}

// CircularCounter counts values by 1-second buckets
type CircularCounter struct {
	buckets      []int
	currentIndex int
	currentTime  uint64
	clocker      Clocker
}

func NewCircularCounter(window int, clocker Clocker) *CircularCounter {
	if clocker == nil {
		clocker = MonoClocker{}
	}
	return &CircularCounter{
		clocker:      clocker,
		buckets:      make([]int, window),
		currentIndex: 0,
		currentTime:  clocker.Now(),
	}
}

// Move the index forward to the current time
func (c *CircularCounter) Forward() {
	now := c.clocker.Now()
	steps := int((now - c.currentTime) / uint64(time.Second))
	if steps <= 0 {
		return
	}

	if steps > len(c.buckets) {
		steps = len(c.buckets)
	}

	start := c.currentIndex + 1
	for i := start; i <= start+steps; i++ {
		c.buckets[i%len(c.buckets)] = 0
	}
	c.currentIndex = (c.currentIndex + steps) % len(c.buckets)
	c.currentTime = now
}

// Add a value
func (c *CircularCounter) Add(v int) {
	c.Forward()
	c.buckets[c.currentIndex] += v
}

// Sum all values
func (c *CircularCounter) Sum() int {
	c.Forward()
	sum := 0
	for _, v := range c.buckets {
		sum += v
	}
	return sum
}

// AlertWorker detects if the hits count crosses a predefined threshold and
// emits alerts when it is the case
type AlarmWorker struct {
	// stores the number of hits per second
	counter *CircularCounter
	// threshold value
	threshold int
	// whether the alert has been triggered or not
	triggered bool
	// channel for receiving the Hit values
	in chan *Hit
	// channel for sending out the alerts
	out chan string
	// channel indicating that the application is done
	done   <-chan struct{}
	logger Logger
}

// Returns a new instance of AlarmWorker
func NewAlarmWorker(window int, threshold int, done <-chan struct{}, logger Logger) *AlarmWorker {
	in := make(chan *Hit)
	out := make(chan string)

	a := AlarmWorker{
		logger:    logger,
		counter:   NewCircularCounter(window, nil),
		threshold: threshold,
		triggered: false,
		in:        in,
		out:       out,
		done:      done,
	}

	go func() {
		defer close(out)
		ticker := time.NewTicker(time.Second * time.Duration(1))
		defer ticker.Stop()
		for {
			select {
			case <-a.in:
				a.counter.Add(1)
			case <-ticker.C:
				// clean up buffer
				sum := a.counter.Sum()
				if !a.triggered && sum >= a.threshold {
					out <- fmt.Sprintf(
						"High traffic generated an alert - hits = %d, triggered at %s",
						sum,
						time.Now().Format(time.RFC3339))
					a.triggered = true
				} else if a.triggered && sum < a.threshold {
					out <- fmt.Sprintf(
						"Traffic went back to normal - hits = %d, triggered at %s",
						sum,
						time.Now().Format(time.RFC3339))
					a.triggered = false
				}
			case <-done:
				a.logger.Println("Exiting AlarmWorker")
				return
			}
		}
	}()

	return &a
}
