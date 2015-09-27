package osthrpool

import (
	"fmt"
	"time"
)

func Example() {
	// Make a new pool of 5 goroutines that are locked to an OS thread.
	// These goroutines are automatically unlocked after 500ms if there
	// are no more tasks to execute.
	p := New(5, 500*time.Millisecond)

	// Execute 10 tasks.
	for i := 0; i < 10; i++ {
		fmt.Println("index:", i)

		p.Execute(func() {
			fmt.Println("task: ", i)
		})
	}

	// Output:
	// index: 0
	// task:  0
	// index: 1
	// task:  1
	// index: 2
	// task:  2
	// index: 3
	// task:  3
	// index: 4
	// task:  4
	// index: 5
	// task:  5
	// index: 6
	// task:  6
	// index: 7
	// task:  7
	// index: 8
	// task:  8
	// index: 9
	// task:  9
}
