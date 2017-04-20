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
	"log"
	"os"
	"os/signal"
	"time"
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

type Logger interface {
	Printf(string, ...interface{})
	Println(...interface{})
	Fatalln(...interface{})
}

func main() {
	var (
		filename  = flag.String("f", "", "HTTP log file to monitor")
		interval  = flag.Int("i", 10, "Interval at which statistics should be emitted")
		window    = flag.Int("w", 120, "Window of time")
		threshold = flag.Int("t", 1, "Hits threshold")
		logger    = log.New(os.Stderr, "", log.LstdFlags)
	)
	flag.Parse()
	if *filename == "" {
		log.Fatalln("-f argument is missing")
	}

	done := make(chan struct{})
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)
	go func() {
		sig := <-sigs
		logger.Println("Caught signal ", sig)
		// closing the done channel will tear down all the goroutines
		close(done)
	}()

	logProcessor, err := NewLogProcessor(*filename, logger)
	if err != nil {
		logger.Fatalln(err)
	}

	// kick off the processing of the logs
	hits := make(chan *Hit)
	go logProcessor.Run(hits, done)

	// dispatch the hits to all the workers
	statsWorker := NewStatsWorker(*interval, done, logger)
	alarmWorker := NewAlarmWorker(*window, *threshold, done, logger)
	go func() {
		for hit := range hits {
			statsWorker.in <- hit
			alarmWorker.in <- hit
		}
	}()

	// and finally collect and display the statistics
	go func() {
		for out := range statsWorker.out {
			logger.Println(out)
		}
	}()
	go func() {
		for out := range alarmWorker.out {
			logger.Println(out)
		}
	}()

	// wait forever
	<-done
}
