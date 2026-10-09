package main

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type LeakyBucket struct {
	queue chan chan struct{}
}

func (lb *LeakyBucket) NewLeakyBucket(capacity int, leakEvery time.Duration) {
	lb.queue = make(chan chan struct{}, capacity)
	go lb.leak(leakEvery)
}

func (lb *LeakyBucket) leak(every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for range ticker.C {
		select {
		case ready := <-lb.queue:
			close(ready) // let one waiting request through
		default: // bucket empty: this tick is wasted, so idle time doesn't build up a burst
		}
	}
}

func (lb *LeakyBucket) Middleware(c *gin.Context) {
	ready := make(chan struct{})
	select {
	case lb.queue <- ready: // got a place in the bucket
	default:
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"response": "Too Many Requests"})
		return
	}

	select {
	case <-ready:
		c.Next()
	case <-c.Request.Context().Done(): // client gave up while waiting
		c.Abort()
	}
}
