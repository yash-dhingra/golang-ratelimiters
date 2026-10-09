package main

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const WINDOW_REQUEST_LIMTIT int = 5
const WINDOW_SIZE time.Duration = 100 * time.Millisecond

type FixedWindowSize struct {
	m           sync.Mutex
	WindowLimit int
}

func (TB *FixedWindowSize) initialize() {
	go TB.filler()
	TB.m.Lock()
	TB.WindowLimit = WINDOW_REQUEST_LIMTIT
	TB.m.Unlock()
}

func (TB *FixedWindowSize) filler() {
	ticker := time.NewTicker(WINDOW_SIZE)
	defer ticker.Stop()
	for range ticker.C {
		TB.m.Lock()
		TB.WindowLimit = WINDOW_REQUEST_LIMTIT
		TB.m.Unlock()
	}
}

func (TB *FixedWindowSize) Middleware(c *gin.Context) {
	TB.m.Lock()
	if TB.WindowLimit > 0 {
		TB.WindowLimit--
		TB.m.Unlock()
		c.Next()
	} else {
		TB.m.Unlock()
		c.JSON(http.StatusTooManyRequests,
			gin.H{"response": "Too Many Request"})
		c.Abort()
	}

}
