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
	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
)

func DrawUi(stat chan []string, alert chan string, done chan struct{}, logger Logger) error {
	if err := ui.Init(); err != nil {
		return err
	}
	defer ui.Close()

	sl := widgets.NewList()
	sl.Rows = []string{}
	sl.Title = "Statistics"
	sl.SetRect(0, 0, 40, 40)

	al := widgets.NewList()
	al.Rows = []string{}
	al.Title = "Alerts"
	al.SetRect(41, 0, 81, 120)

	ui.Render(sl, al)

	go func() {
		for {
			select {
			case stats := <-stat:
				sl.Rows = stats
				ui.Render(sl, al)
			case alert := <-alert:
				al.Rows = append([]string{alert}, al.Rows...)
				ui.Render(sl, al)
			case <-done:
				return
			}
		}
	}()

	uiEvents := ui.PollEvents()
	for {
		e := <-uiEvents
		switch e.ID {
		case "q", "<C-c>":
			close(done)
			return nil
		}
	}
}
