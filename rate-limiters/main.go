package main

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

func health(c *gin.Context) {
	c.String(http.StatusOK, "Healthy!")
}

func fibonacci(num int) int {
	if num <= 1 {
		return 1
	}
	return fibonacci(num-1) + fibonacci(num-2)
}

func FibonacciCaller(c *gin.Context) {
	num, err := strconv.Atoi(c.Param("num"))
	if err != nil {
		c.String(http.StatusNotAcceptable, "Invalid Input, please use correct input number")
		return
	}

	output := fibonacci(num)

	c.String(http.StatusOK, "Output: %d", output)
}

func main() {
	router := gin.Default()
	fmt.Println("Hello world...Starting Server!!!")

	var TB TokenBucket
	var LB LeakyBucket
	var FWS FixedWindowSize
	var SWL SlidingWindowLog
	var SWC SlidingWindowCounter
	var CSWC CustomSlidingWindowCounter

	TB.initialize()
	FWS.initialize()
	LB.NewLeakyBucket(100, 100*time.Millisecond)
	SWL.initialize()
	SWC.initialize()
	CSWC.initialize()

	router.GET("/health", health)
	router.GET("/tokenBucket/:num", TB.Middleware, FibonacciCaller)
	router.GET("/leakingBucket/:num", LB.Middleware, FibonacciCaller)
	router.GET("/FixedWindowSize/:num", FWS.Middleware, FibonacciCaller)
	router.GET("/SlidingWindowLog/:num", SWL.Middleware, FibonacciCaller)
	router.GET("/SlidingWindowCounter/:num", SWC.Middleware, FibonacciCaller)
	router.GET("/CustomSlidingWindowCounter/:num", CSWC.Middleware, FibonacciCaller)
	router.Run()
}
