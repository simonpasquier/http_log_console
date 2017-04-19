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
// TODO: add counters to track the number of ok/failed lines that have been processed
type LogProcessor struct {
	stream *tail.Tail
}

// Returns a new instance of LogProcessor
func NewLogProcessor(filename string) (l *LogProcessor, err error) {
	// Skip directly to the end of the file to avoid processing old lines
	tail_config := tail.Config{
		Follow: true,
		Logger: tail.DiscardingLogger,
		Location: &tail.SeekInfo{Offset: 0, Whence: os.SEEK_END},
		MustExist: true,
	}
	stream, err := tail.TailFile(filename, tail_config)
	if err != nil {
		return nil, err
	}
	l = &LogProcessor{
		stream: stream,
	}
	return
}

// Reads the HTTP log lines and sends Hit values to the out channel
func (l *LogProcessor) Run(out chan<- *Hit, done <-chan struct{}) (err error) {
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
			hit := &Hit{method: matches[2], uri: matches[3]}
			// TODO: configurable time format
			hit.status, err = strconv.Atoi(matches[4])
			hit.timestamp, err = time.Parse("02/Jan/2006:15:04:05 -0700", matches[1])
			if err != nil {
				fmt.Println(err)
				continue
			}
			out <- hit
		case <-done:
			break
		}
	}

	return
}

// StatsWorker aggregates the number of hits over the given interval
type StatsWorker struct {
	// total number of hits
	hits int
	// interval (in seconds) at which statistics are emitted
	interval int
	// channel for receiving the Hit values
	in chan *Hit
	// channel for sending out the statistics
	out chan string
	// channel indicating that the application is done
	done <-chan struct{}
}

func NewStatsWorker(interval int, done <-chan struct{}) (*StatsWorker) {
	in := make(chan *Hit)
	out := make(chan string)

	s := StatsWorker{interval: interval, in: in, out: out, done: done}

	go func(){
		ticker := time.NewTicker(time.Second * time.Duration(s.interval))
		for {
			select {
			case <-s.in:
				s.hits += 1
			case <-ticker.C:
				out <- fmt.Sprintf("%d hits", s.hits)
				s.hits = 0
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
		return
	}

	// kick off the processing of the logs
	hits := make(chan *Hit)
	go logProcessor.Run(hits, done)

	// kick off the processing of the hits
	statsWorker := NewStatsWorker(5, done)
	go func() {
		for hit := range hits {
			statsWorker.in <- hit
		}
	}()

	// and finally collect the statistics
	go func() {
		for out := range statsWorker.out {
			fmt.Println(out)
		}
	}()

	// wait forever
	<-done
}
