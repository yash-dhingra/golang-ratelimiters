package main

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const SWC_WINDOW_REQUEST_LIMIT int = 5
const SWC_WINDOW_SIZE time.Duration = 100 * time.Millisecond
const SWC_WINDOW_TRUNCATE = 10 * time.Millisecond

type CustomSlidingWindowCounter struct {
	m           sync.Mutex
	PrevWindowL time.Time
	Prev30C     int
	Prev70C     int
	CurWindowL  time.Time
	Cur30C      int
	Cur70C      int
}

func (SWC *CustomSlidingWindowCounter) initialize() {
	SWC.m.Lock()
	SWC.Prev70C = 0
	SWC.Prev30C = 0
	SWC.CurWindowL = time.Now().Truncate(SWC_WINDOW_SIZE)
	SWC.PrevWindowL = SWC.CurWindowL.Add(-SWC_WINDOW_SIZE)
	SWC.Cur30C = 0
	SWC.Cur70C = 0
	SWC.m.Unlock()
}

func (SWC *CustomSlidingWindowCounter) Middleware(c *gin.Context) {
	SWC.m.Lock()

	now := time.Now()

	if now.Sub(SWC.CurWindowL) < SWC_WINDOW_SIZE {
		// It means the current req is in range
		// check if it's in the first 30%
		if now.Truncate(SWC_WINDOW_TRUNCATE).Sub(SWC.CurWindowL) <= 30*time.Millisecond {
			if SWC.Cur30C+SWC.Prev70C+1 <= SWC_WINDOW_REQUEST_LIMIT {
				SWC.Cur30C++
				SWC.m.Unlock()
				c.Next()
			} else {
				SWC.m.Unlock()
				c.JSON(http.StatusTooManyRequests,
					gin.H{"response": "Too Many Request"})
				c.Abort()
			}
		} else {
			if SWC.Cur30C+SWC.Cur70C+1 <= SWC_WINDOW_REQUEST_LIMIT {
				SWC.Cur70C++
				SWC.m.Unlock()
				c.Next()
			} else {
				SWC.m.Unlock()
				c.JSON(http.StatusTooManyRequests,
					gin.H{"response": "Too Many Request"})
				c.Abort()
			}
		}
	} else {
		// check if the -window size of now is farther away, if yes reset prev, else transfer current
		if now.Add(-SWC_WINDOW_SIZE).After(SWC.CurWindowL.Add(SWC_WINDOW_SIZE)) {
			SWC.Prev30C = 0
			SWC.Prev70C = 0
			SWC.CurWindowL = now.Truncate(SWC_WINDOW_SIZE)
			SWC.PrevWindowL = SWC.CurWindowL.Add(-SWC_WINDOW_SIZE)
			SWC.Cur30C = 0
			SWC.Cur70C = 0
		} else {
			SWC.CurWindowL = SWC.CurWindowL.Add(SWC_WINDOW_SIZE)
			SWC.Prev30C = SWC.Cur30C
			SWC.Prev70C = SWC.Cur70C
			SWC.Cur30C = 0
			SWC.Cur70C = 0
		}

		if now.Truncate(SWC_WINDOW_TRUNCATE).Sub(SWC.CurWindowL) <= 30*time.Millisecond {
			if SWC.Cur30C+SWC.Prev70C+1 <= SWC_WINDOW_REQUEST_LIMIT {
				SWC.Cur30C++
				SWC.m.Unlock()
				c.Next()
			} else {
				SWC.m.Unlock()
				c.JSON(http.StatusTooManyRequests,
					gin.H{"response": "Too Many Request"})
				c.Abort()
			}
		} else {
			if SWC.Cur30C+SWC.Cur70C+1 <= SWC_WINDOW_REQUEST_LIMIT {
				SWC.Cur70C++
				SWC.m.Unlock()
				c.Next()
			} else {
				SWC.m.Unlock()
				c.JSON(http.StatusTooManyRequests,
					gin.H{"response": "Too Many Request"})
				c.Abort()
			}
		}

	}

}
