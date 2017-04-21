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
		window    = flag.Int("w", 120, "Alarm evaluation period")
		threshold = flag.Int("t", 100, "Alarm threshold")
		logger    = log.New(os.Stderr, "", log.LstdFlags)
	)
	flag.Parse()
	if *filename == "" {
		log.Fatalln("-f argument is missing")
	}

	done := make(chan struct{})

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

	// finally display the UI
	go DrawUi(statsWorker.out, alarmWorker.out, done, logger)

	// wait forever
	<-done
}
