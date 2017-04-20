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

// StatsWorker aggregates the number of hits over the given interval
type StatsWorker struct {
	logger Logger
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
	out chan string
	// channel indicating that the application is done
	done <-chan struct{}
}

// the following code comes from https://groups.google.com/d/msg/golang-nuts/FT7cjmcL7gw/Gj4_aEsE_IsJ
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

func NewStatsWorker(interval int, done <-chan struct{}, logger Logger) *StatsWorker {
	in := make(chan *Hit)
	out := make(chan string)

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
				for _, p := range sortMapByValue(s.sectionHits) {
					out <- fmt.Sprintf("'%s' section: %d hits", p.Key, p.Value)
					delete(s.sectionHits, p.Key)
				}
				for i, v := range s.statusHits[1:] {
					out <- fmt.Sprintf("'%dxx': %d hits", i, v)
					s.statusHits[i] = 0
				}
				out <- fmt.Sprintf("'other': %d hits", s.statusHits[0])
				s.statusHits[0] = 0
				out <- fmt.Sprintf("total: %d hits (%.02f/sec)", s.totalHits, float64(s.totalHits/s.interval))
				s.totalHits = 0
			case <-done:
				s.logger.Println("Exiting StatsWorker")
				break
			}
		}
	}()

	return &s
}

// CircularCounter counts values over a period in 1-second buckets
type CircularCounter struct {
	buckets      []int
	currentIndex int
	currentTime  uint64
}

func NewCircularCounter(window int) *CircularCounter {
	return &CircularCounter{
		buckets:      make([]int, window),
		currentIndex: 0,
		currentTime:  atime.NanoTime(),
	}
}

// Move the index forward to the current time
func (c *CircularCounter) Forward() {
	now := atime.NanoTime()
	steps := int((now - c.currentTime) / uint64(time.Second))
	if steps <= 0 {
		return
	}

	if steps > len(c.buckets) {
		steps = len(c.buckets)
	}

	newIndex := (c.currentIndex + steps) % len(c.buckets)
	for i := c.currentIndex + 1; i <= newIndex; i++ {
		c.buckets[i%len(c.buckets)] = 0
	}
	c.currentIndex = newIndex
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

// AlertWorker detects if the hits count reaches a predefined threshold
type AlarmWorker struct {
	logger Logger
	// the number of hits per second
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
	done <-chan struct{}
}

func NewAlarmWorker(window int, threshold int, done <-chan struct{}, logger Logger) *AlarmWorker {
	in := make(chan *Hit)
	out := make(chan string)

	a := AlarmWorker{
		logger:    logger,
		counter:   NewCircularCounter(window),
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
					out <- fmt.Sprintf("High traffic generated an alert - hits = %d, triggered at %s", sum, time.Now())
					a.triggered = true
				} else if a.triggered && sum < a.threshold {
					out <- fmt.Sprintf("Traffic went back to normal - hits = %d, triggered at %s", sum, time.Now())
					a.triggered = false
				}
			case <-done:
				a.logger.Println("Exiting AlarmWorker")
				break
			}
		}
	}()

	return &a
}
