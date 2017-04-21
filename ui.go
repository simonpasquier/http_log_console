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
	ui "github.com/gizak/termui"
)

func DrawUi(stat chan []string, alert chan string, done chan struct{}, logger Logger) error {
	if err := ui.Init(); err != nil {
		return err
	}
	defer ui.Close()

	sl := ui.NewList()
	sl.Items = []string{}
	sl.BorderLabel = "Statistics"
	sl.Height = 40
	sl.Width = 40
	sl.X = 0
	sl.Y = 0

	al := ui.NewList()
	al.Items = []string{}
	al.BorderLabel = "Alerts"
	al.Height = 40
	al.Width = 120
	al.X = 41
	al.Y = 0

	ui.Handle("/sys/kbd/q", func(ui.Event) {
		// tear down all goroutines
		close(done)
		ui.StopLoop()
	})

	ui.Render(sl, al)

	update := func() {
		for {
			select {
			case stats := <-stat:
				sl.Items = stats
				ui.Render(sl, al)
			case alert := <-alert:
				al.Items = append([]string{alert}, al.Items...)
				ui.Render(sl, al)
			case <-done:
				return
			}
		}
	}

	go update()
	ui.Loop()

	return nil
}
