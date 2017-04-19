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
	"flag"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/hpcloud/tail"
)

// Hit represents a user's hit
type Hit struct {
	// timestamp as reported in the log file
	timestamp time.Time
	// HTTP method (eg 'GET, 'POST', ...)
	method string
	// Request URI
	uri string
	// HTTP status code (eg 200, 404, ...)
	status int
}

// LogProcessor watches a file stream
type LogProcessor struct {
	stream *tail.Tail
}

// Returns a new instance of LogProcessor
func NewLogProcessor(filename string) (*LogProcessor, error) {
	// Skip directly to the end of the file to avoid processing old lines
	tailConfig := tail.Config{
		Follow: true,
		Logger: tail.DiscardingLogger,
		Location: &tail.SeekInfo{Offset: 0, Whence: os.SEEK_END},
		MustExist: true,
	}
	stream, err := tail.TailFile(filename, tailConfig)
	if err != nil {
		return nil, err
	}
	return &LogProcessor{stream: stream,}, nil
}

// Reads the HTTP log lines and sends Hit values to the out channel
func (l *LogProcessor) Run(out chan<- *Hit, done <-chan struct{}) error {
	defer l.stream.Cleanup()
	clf := regexp.MustCompile("\\[(?P<timestamp>[^]]+)\\] \"(?P<method>\\S+) (?P<uri>\\S+) [^\"]+\" (?P<status>\\d+)")

	for {
		select {
		case line := <-l.stream.Lines:
			matches := clf.FindStringSubmatch(line.Text)
			if matches == nil {
				fmt.Printf("no match found for %s", line.Text)
				continue
			}
			// TODO: configurable time format
			status, _ := strconv.Atoi(matches[4])
			timestamp, err := time.Parse("02/Jan/2006:15:04:05 -0700", matches[1])
			if err != nil {
				fmt.Println(err)
				continue
			}
			out <- &Hit{
				timestamp: timestamp,
				uri: matches[3],
				method: matches[2],
				status: status,
			}
		case <-done:
			break
		}
	}

	return nil
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
	out chan string
	// channel indicating that the application is done
	done <-chan struct{}
}

// the following code comes from https://groups.google.com/d/msg/golang-nuts/FT7cjmcL7gw/Gj4_aEsE_IsJ
// A data structure to hold a key/value pair.
type Pair struct {
  Key string
  Value int
}

// A slice of Pairs that implements sort.Interface to sort by Value.
type PairList []Pair
func (p PairList) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p PairList) Len() int { return len(p) }
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

func computeRate(v int, interval int) {

}

func NewStatsWorker(interval int, done <-chan struct{}) (*StatsWorker) {
	in := make(chan *Hit)
	out := make(chan string)

	s := StatsWorker{
		sectionHits: make(map[string]int),
		statusHits: make([]int, 6),
		interval: interval,
		in: in,
		out: out,
		done: done,
	}

	go func(){
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
				out <- fmt.Sprintf("total: %d hits (%.02f/sec)", s.totalHits, float64(s.totalHits / s.interval))
				s.totalHits = 0
			case <-done:
				break
			}
		}
	}()

	return &s
}

func main() {
	var (
		filename = flag.String("f", "", "HTTP log file to monitor")
		interval = flag.Int("i", 10, "Interval at which statistics should be emitted")
	)
	flag.Parse()
	if *filename == "" {
		fmt.Println("-f argument is missing")
		return
	}

	done := make(chan struct{})
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)
	go func() {
		sig := <-sigs
		fmt.Println("")
		fmt.Println("Caught signal ", sig)
		// closing the done channel will tear down all the goroutines
		close(done)
	}()

	logProcessor, err := NewLogProcessor(*filename)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// kick off the processing of the logs
	hits := make(chan *Hit)
	go logProcessor.Run(hits, done)

	// dispatch the hits to all the workers
	statsWorker := NewStatsWorker(*interval, done)
	go func() {
		for hit := range hits {
			statsWorker.in <- hit
		}
	}()

	// and finally collect and display the statistics
	go func() {
		for out := range statsWorker.out {
			fmt.Println(out)
		}
	}()

	// wait forever
	<-done
}
