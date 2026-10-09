package main

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const SWL_LIMIT int = 10
const SWL_WINDOW_SIZE time.Duration = 100 * time.Millisecond

type SlidingWindowLog struct {
	m      sync.Mutex
	window []time.Time
	head   int
	size   int
	limit  int
}

func (swl *SlidingWindowLog) initialize() {
	swl.m.Lock()
	defer swl.m.Unlock()
	swl.window = make([]time.Time, SWL_LIMIT)
	swl.head = 0
	swl.size = 0
	swl.limit = SWL_LIMIT
}

func (swl *SlidingWindowLog) Middleware(c *gin.Context) {
	swl.m.Lock()

	// clean up old timestamps
	cutoff := time.Now().Add(-SWL_WINDOW_SIZE)
	for swl.size > 0 {
		if swl.window[swl.head].Before(cutoff) {
			swl.window[swl.head] = time.Time{}
			swl.head = (swl.head + 1) % swl.limit
			swl.size--
		} else {
			break
		}
	}
	// Check if there's size
	tail := (swl.head + swl.size) % swl.limit

	if swl.size >= swl.limit {
		swl.m.Unlock()
		c.JSON(http.StatusTooManyRequests,
			gin.H{"response": "Too Many Request"})
		c.Abort()
	} else {
		swl.window[tail] = time.Now()
		swl.size++
		swl.m.Unlock()
		c.Next()
	}

}
