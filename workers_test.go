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
	"time"
	"testing"
)

type FakeClocker struct{
	now uint64
}

func (f FakeClocker) Now() uint64 {
	return f.now
}

func (f *FakeClocker) Set(v uint64) {
	f.now = v * uint64(time.Second)
}

func TestCircularCounterSum(t *testing.T) {
	clocker := FakeClocker{}
	counter := NewCircularCounter(120, &clocker)

	v := counter.Sum()
	if v != 0 {
		t.Fatalf("Expected 0 but got %d", v)
	}

	counter.Add(1)
	v = counter.Sum()
	if v != 1 {
		t.Fatalf("Expected 1 but got %d", v)
	}

	clocker.Set(20)
	counter.Add(2)
	v = counter.Sum()
	if v != 3 {
		t.Fatalf("Expected 3 but got %d", v)
	}

	clocker.Set(120)
	counter.Add(3)
	v = counter.Sum()
	if v != 5 {
		t.Fatalf("Expected 5 but got %d", v)
	}

	clocker.Set(200)
	v = counter.Sum()
	if v != 3 {
		t.Fatalf("Expected 3 but got %d", v)
	}

	clocker.Set(1000)
	v = counter.Sum()
	if v != 0 {
		t.Fatalf("Expected 0 but got %d", v)
	}
}
