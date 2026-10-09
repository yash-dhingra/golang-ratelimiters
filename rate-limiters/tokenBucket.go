package main

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const BUCKET_SIZE int = 300
const REFILL_RATE_PER_MINUTE int = 120

type TokenBucket struct {
	m      sync.Mutex
	Bucket int
}

func (TB *TokenBucket) initialize() {
	go TB.filler()
	TB.m.Lock()
	TB.Bucket = BUCKET_SIZE
	TB.m.Unlock()
}

func (TB *TokenBucket) filler() {
	for {
		time.Sleep(1 * time.Minute)
		TB.m.Lock()
		TB.Bucket = min(TB.Bucket+REFILL_RATE_PER_MINUTE, BUCKET_SIZE)
		TB.m.Unlock()
	}
}

func (TB *TokenBucket) Middleware(c *gin.Context) {
	TB.m.Lock()
	if TB.Bucket > 0 {
		TB.Bucket--
		TB.m.Unlock()
		c.Next()
	} else {
		TB.m.Unlock()
		c.JSON(http.StatusTooManyRequests,
			gin.H{"response": "Too Many Request"})
		c.Abort()
	}

}
