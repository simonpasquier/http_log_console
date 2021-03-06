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
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/hpcloud/tail"
)

// LogProcessor watches a file stream
type LogProcessor struct {
	logger Logger
	stream *tail.Tail
}

// Returns a new instance of LogProcessor
func NewLogProcessor(filename string, logger Logger) (*LogProcessor, error) {
	// Skip directly to the end of the file to avoid processing old lines
	tailConfig := tail.Config{
		Follow:    true,
		Logger:    tail.DiscardingLogger,
		Location:  &tail.SeekInfo{Offset: 0, Whence: os.SEEK_END},
		MustExist: true,
	}
	stream, err := tail.TailFile(filename, tailConfig)
	if err != nil {
		return nil, err
	}
	return &LogProcessor{stream: stream, logger: logger}, nil
}

// Reads the HTTP log lines and sends Hit values to the out channel
func (l *LogProcessor) Run(out chan<- *Hit, done <-chan struct{}) error {
	defer l.stream.Cleanup()
	clf := regexp.MustCompile("\\[([^]]+)\\] \"(\\S+) (\\S+) [^\"]+\" (\\d+)")

	for {
		select {
		case line := <-l.stream.Lines:
			matches := clf.FindStringSubmatch(line.Text)
			if matches == nil {
				l.logger.Printf("no match found for %s", line.Text)
				continue
			}
			status, _ := strconv.Atoi(matches[4])
			timestamp, err := time.Parse("02/Jan/2006:15:04:05 -0700", matches[1])
			if err != nil {
				l.logger.Println(err)
				continue
			}
			out <- &Hit{
				timestamp: timestamp,
				uri:       matches[3],
				method:    matches[2],
				status:    status,
			}
		case <-done:
			return nil
		}
	}
}
