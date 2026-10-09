package main

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const SWC_LIMIT int = 5
const SWC_WINDOW time.Duration = 100 * time.Millisecond

type SlidingWindowCounter struct {
	m          sync.Mutex
	CurWindowL time.Time
	PrevCount  int
	CurCount   int
}

func (SWC *SlidingWindowCounter) initialize() {
	SWC.m.Lock()
	SWC.CurWindowL = time.Now().Truncate(SWC_WINDOW)
	SWC.PrevCount = 0
	SWC.CurCount = 0
	SWC.m.Unlock()
}

func (SWC *SlidingWindowCounter) Middleware(c *gin.Context) {
	SWC.m.Lock()

	now := time.Now()
	windowStart := now.Truncate(SWC_WINDOW)

	if windowStart.Equal(SWC.CurWindowL.Add(SWC_WINDOW)) {
		SWC.PrevCount = SWC.CurCount
		SWC.CurCount = 0
		SWC.CurWindowL = windowStart
	} else if !windowStart.Equal(SWC.CurWindowL) {
		// A whole window passed with no requests, so the previous window counts as empty.
		SWC.PrevCount = 0
		SWC.CurCount = 0
		SWC.CurWindowL = windowStart
	}

	elapsed := now.Sub(SWC.CurWindowL)
	prevWeight := 1 - float64(elapsed)/float64(SWC_WINDOW)
	estimate := float64(SWC.PrevCount)*prevWeight + float64(SWC.CurCount)

	if estimate < float64(SWC_LIMIT) {
		SWC.CurCount++
		SWC.m.Unlock()
		c.Next()
	} else {
		SWC.m.Unlock()
		c.JSON(http.StatusTooManyRequests,
			gin.H{"response": "Too Many Request"})
		c.Abort()
	}
}
