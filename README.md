# Rate Limiter Algorithms in Go (Golang) with Gin

Learn how API rate limiting works by reading and running the five classic **rate limiting algorithms**: **token bucket**, **leaky bucket**, **fixed window counter**, **sliding window log**, and **sliding window counter**. Each one is implemented from scratch in Go as [Gin](https://github.com/gin-gonic/gin) middleware, and a small load tester lets you watch them reject traffic with `HTTP 429 Too Many Requests`.

I built this while learning Go. Every implementation is short, uses only the standard library plus Gin, and is meant to be read. It's useful if you are:

- learning **Go concurrency**: mutexes, goroutines, channels, `select`, tickers
- preparing for a **system design interview** ("design a rate limiter")
- choosing a **rate limiting algorithm** for your own API

## Contents

- [What is rate limiting?](#what-is-rate-limiting)
- [The algorithms at a glance](#the-algorithms-at-a-glance)
- [How each algorithm works](#how-each-algorithm-works)
- [Which rate limiting algorithm should I use?](#which-rate-limiting-algorithm-should-i-use)
- [Getting started](#getting-started)
- [Load testing the rate limiters](#load-testing-the-rate-limiters)
- [Lessons learned: Go concurrency pitfalls](#lessons-learned-go-concurrency-pitfalls)
- [Limitations and next steps](#limitations-and-next-steps)
- [FAQ](#faq)

## What is rate limiting?

A rate limiter caps how many requests a client can make in a given period of time. Requests over the limit are rejected, usually with status code `429 Too Many Requests`. APIs use rate limiting to protect servers from overload and abuse, to keep usage fair between clients, and to control costs.

All rate limiting algorithms answer the same question: *should this request be allowed right now?* They differ in how they count requests, which changes three things:

- how they handle **bursts** of traffic
- how **smooth** the traffic they let through is
- how much **memory** they need

## The algorithms at a glance

| Algorithm | Source | Route | Settings in this repo | Bursts | Memory |
|---|---|---|---|---|---|
| Token bucket | [tokenBucket.go](rate-limiters/tokenBucket.go) | `/tokenBucket/:num` | 300 tokens, refills 120 per minute | Allowed, up to the bucket size | O(1) |
| Leaky bucket | [leakingBucket.go](rate-limiters/leakingBucket.go) | `/leakingBucket/:num` | Queue of 100, lets 1 request out every 100ms | Smoothed into a steady rate (requests wait) | O(queue size) |
| Fixed window counter | [FixedWindowCounter.go](rate-limiters/FixedWindowCounter.go) | `/FixedWindowSize/:num` | 5 requests per 100ms window | Up to 2× the limit at window edges | O(1) |
| Sliding window log | [SlidingWindowLog.go](rate-limiters/SlidingWindowLog.go) | `/SlidingWindowLog/:num` | 10 requests per rolling 100ms | None, the limit is exact | O(limit) |
| Sliding window counter | [SlidingWindowCounter.go](rate-limiters/SlidingWindowCounter.go) | `/SlidingWindowCounter/:num` | 5 requests per rolling 100ms (estimated) | Small estimation error | O(1) |
| 30/70 split (experiment) | [CustomSlidingWindowCounter.go](rate-limiters/CustomSlidingWindowCounter.go) | `/CustomSlidingWindowCounter/:num` | 5 requests per 100ms | Up to 2× the limit ([why](#experiment-the-3070-split-and-why-it-fails)) | O(1) |

Every route runs the same handler, which computes the `num`-th Fibonacci number with naive recursion. That makes `num` a dial for how much CPU each request uses: `/tokenBucket/20` returns instantly, `/tokenBucket/40` takes a noticeable moment.

Each limiter in this repo is **global**: one counter shared by every client. Real APIs usually keep one per client; see [Limitations and next steps](#limitations-and-next-steps).

## How each algorithm works

### Token bucket

Picture a bucket that holds up to 300 tokens. Each request takes one token. If the bucket is empty, the request is rejected. Tokens are added back at a fixed rate (here, 120 every minute), but never beyond the bucket's capacity.

- **Strength:** allows short bursts (up to 300 requests at once) while enforcing an average rate over time. It's the most widely used algorithm, and Go's own [`golang.org/x/time/rate`](https://pkg.go.dev/golang.org/x/time/rate) package is a token bucket.
- **Tradeoff:** two numbers to tune (bucket size and refill rate).

**In Go:** a `sync.Mutex` protects the token count, and a background goroutine adds tokens once a minute, capped with the built-in `min`. Note that the lock is released *before* `c.Next()`, so requests are handled in parallel:

```go
func (TB *TokenBucket) Middleware(c *gin.Context) {
	TB.m.Lock()
	if TB.Bucket > 0 {
		TB.Bucket--
		TB.m.Unlock() // release before running the handler
		c.Next()
	} else {
		TB.m.Unlock()
		c.JSON(http.StatusTooManyRequests, gin.H{"response": "Too Many Request"})
		c.Abort()
	}
}
```

### Leaky bucket

Requests pour into a bucket (a queue that holds up to 100) and leak out of a hole in the bottom at a constant rate: here one every 100ms, or 10 requests per second. If the bucket is full when a request arrives, that request is rejected.

- **Strength:** the outgoing rate is perfectly steady, which protects a downstream service that can only handle a fixed rate.
- **Tradeoff:** requests wait in the queue, which adds latency. A burst fills the queue, and newer requests get rejected while older ones wait.

**In Go:** this one uses channels and no mutex. A buffered channel is the queue. Each request puts its own `ready` channel into it, and a goroutine with a `time.Ticker` closes one `ready` channel per tick, which lets that request through:

```go
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
```

The middleware uses `select` with `default` to join the queue without blocking (a full queue means 429). It also watches `c.Request.Context().Done()`, so a client that gives up stops waiting.

> **Common mistake:** counting in-flight requests and giving the slot back when the handler finishes is a *concurrency limiter*, not a leaky bucket. It has no notion of time, so fast requests are never limited at all.

### Fixed window counter

Divide time into fixed windows (100ms here) and allow up to 5 requests in each one. The counter resets at the start of every window.

- **Strength:** the simplest algorithm, with O(1) memory.
- **Tradeoff:** bursts at window edges. A client can send 5 requests at the end of one window and 5 at the start of the next:

```
        window 1 (0–100ms)        window 2 (100–200ms)
   ...................|||||  |||||...................
                      95ms   101ms
```

That's 10 requests in about 6ms, twice the limit, and both windows count it as allowed.

**In Go:** a goroutine with a `time.Ticker` resets the counter every 100ms. An alternative with no goroutine is to work out the current window from the clock with `time.Now().Truncate(windowSize)`. That also lines windows up across multiple servers.

### Sliding window log

Keep a timestamp for every allowed request. When a new request arrives, drop the timestamps older than 100ms. If fewer than 10 remain, allow the request and record its timestamp.

- **Strength:** exact. There are no fixed windows, so there are no edge bursts. Any 100ms span contains at most 10 allowed requests.
- **Tradeoff:** memory grows with the limit, one timestamp per allowed request. That's fine for 10, but expensive for 10,000 per minute across a million users.

**In Go:** the log is a **circular buffer**: a fixed-size slice plus `head` and `size` indexes. Adding and removing are both O(1), and nothing is allocated per request. Timestamps are recorded while holding the lock, so they are always in order, and cleanup can stop at the first one that's still fresh:

```go
cutoff := time.Now().Add(-SWL_WINDOW_SIZE)
for swl.size > 0 {
	if swl.window[swl.head].Before(cutoff) {
		swl.head = (swl.head + 1) % swl.limit
		swl.size--
	} else {
		break
	}
}
```

### Sliding window counter

Keep just two counters: requests in the previous fixed window and in the current one. Then estimate how many requests fall in the last 100ms:

```
estimate = prevCount × (1 − elapsed / windowSize) + curCount
```

Example with a limit of 5: the previous window had 5 requests, you're 30ms into the current window, and it has 1 request so far. 70% of the previous window still falls inside the last 100ms, so `5 × 0.7 + 1 = 4.5` → allowed. The next request gives `5 × 0.7 + 2 = 5.5` → rejected.

- **Strength:** O(1) memory like the fixed window, without its edge bursts.
- **Tradeoff:** it's an estimate. It assumes the previous window's requests were spread evenly, which is close enough in practice.

**In Go:** roll the window forward first (comparing window starts made with `Truncate`), then compute the estimate. Divide durations as `float64`. Dividing one `time.Duration` by another gives an integer, so the fraction would round down to 0.

### Experiment: the 30/70 split (and why it fails)

Before using the weighted formula, I tried splitting each window into a 30% part and a 70% part, and counting only the parts that fall inside the last 100ms. It's kept in the repo as a lesson. With two large, uneven parts, some 100ms spans are miscounted. For example: 5 requests at 95ms of one window, then 5 more at 40ms of the next, gives 10 requests in 45ms.

Splitting each window into many small, equal parts (say 10 × 10ms) does make this approach accurate, and it's a real technique. The weighted formula gets close with just two counters.

## Which rate limiting algorithm should I use?

| You need | Use |
|---|---|
| An average rate, with short bursts allowed (most APIs) | Token bucket |
| A perfectly steady outgoing rate | Leaky bucket |
| Exact limits, and limits are small | Sliding window log |
| Accurate limits for many clients with little memory | Sliding window counter |
| The simplest thing, and edge bursts are acceptable | Fixed window counter |

## Getting started

**Prerequisites:** Go 1.26 or newer.

```bash
git clone https://github.com/yash-dhingra/golang-ratelimiters.git
cd golang-ratelimiters/rate-limiters
go run .
```

Use `go run .`, not `go run main.go`. The limiters live in separate files of the same package, and `go run main.go` compiles only that one file, which fails with `undefined: TokenBucket`.

The server listens on port 8080:

```bash
curl http://localhost:8080/health
# Healthy!

curl http://localhost:8080/tokenBucket/20
# Output: 10946
```

When a limiter rejects a request, the response is:

```
HTTP/1.1 429 Too Many Requests
{"response":"Too Many Request"}
```

## Project structure

```
golang-ratelimiters/
├── rate-limiters/                     # Gin server, one route per algorithm
│   ├── main.go                        # routes and the Fibonacci handler
│   ├── tokenBucket.go
│   ├── leakingBucket.go
│   ├── FixedWindowCounter.go
│   ├── SlidingWindowLog.go
│   ├── SlidingWindowCounter.go
│   └── CustomSlidingWindowCounter.go  # the 30/70 experiment
└── load-applicator/                   # sends concurrent requests at a limiter
    └── main.go
```

## Load testing the rate limiters

With the server running, open a second terminal:

```bash
cd golang-ratelimiters/load-applicator
go run .
```

The load tester starts 150 goroutines that each send one request to `/SlidingWindowLog/40`, and prints the status of every response: `200 OK` when the limiter allows it, `429 Too Many Requests` when it rejects it. Edit the URL and the request count in [load-applicator/main.go](load-applicator/main.go) to test the other algorithms.

Tip: use a small `num` such as `20` to test the limiter itself. With `40`, the CPU cost of the handler dominates.

## Lessons learned: Go concurrency pitfalls

These are the bugs I actually hit while building this. They're worth knowing before you write your own middleware.

1. **Don't hold a mutex across `c.Next()`.** `defer mu.Unlock()` at the top of a middleware keeps the lock held while the whole handler runs, so the server can only handle one request at a time. Unlock before calling `c.Next()`, or move the locked part into a small helper that returns `bool`.
2. **Cap the token bucket.** Without `min(tokens+refill, capacity)`, an idle server builds up thousands of tokens and lets a huge burst through later.
3. **Compare `time.Time` with methods.** `t1 < t2` doesn't compile, because `time.Time` is a struct. Use `Before`, `After` and `Equal`. Avoid `==` on times too: it also compares the time zone and the monotonic clock reading.
4. **Divide durations as floats.** `elapsed / windowSize` with two `time.Duration` values is integer division. Use `float64(elapsed) / float64(windowSize)`.
5. **`connection reset by peer` under load isn't your rate limiter.** When hundreds of goroutines connect at once, the operating system's queue of waiting connections overflows (128 by default on macOS; check with `sysctl kern.ipc.somaxconn`). Limit how many requests the client sends at a time with a buffered channel used as a semaphore:

   ```go
   sem := make(chan struct{}, 50) // at most 50 requests in flight
   for i := 0; i < 400; i++ {
   	wg.Add(1)
   	sem <- struct{}{}
   	go func() {
   		defer wg.Done()
   		defer func() { <-sem }()
   		// send the request
   	}()
   }
   ```
6. **A concurrency limiter is not a leaky bucket.** A rate limiter needs a notion of time.
7. **Fixed windows allow 2× bursts at the edges.** That's why the sliding window algorithms exist.

## Limitations and next steps

This is a learning project, not a production library. Ideas for extending it:

- **Per-client limits:** keep one limiter per client, keyed by `c.ClientIP()` or an API key, in a `map[string]*Limiter`, and remove idle entries.
- **Distributed rate limiting:** these limiters live in one server's memory. With several servers behind a load balancer, keep the counters in a shared store such as Redis.
- **Rate limit headers:** tell clients where they stand with `X-RateLimit-Limit`, `X-RateLimit-Remaining` and `Retry-After`.
- **Lazy refill:** compute the token bucket's tokens from the time elapsed since the last request, instead of running a refill goroutine.
- **Constructors:** `NewTokenBucket(...)`-style functions, so a limiter can't be used before it's set up.
- **Unit tests with a fake clock:** inject `now func() time.Time` so edge cases like window boundaries can be tested exactly.

## FAQ

**What is the difference between token bucket and leaky bucket?**
A token bucket decides immediately, allowing or rejecting each request, and permits bursts up to the bucket size. A leaky bucket queues requests and releases them at a constant rate, so traffic comes out smooth, but requests may wait.

**What is the difference between a sliding window log and a sliding window counter?**
The log stores a timestamp per request and is exact. The counter stores two numbers and estimates the count, trading a little accuracy for constant memory.

**What HTTP status code should a rate limiter return?**
`429 Too Many Requests`, ideally with a `Retry-After` header telling the client when to try again.

**Does Go have a built-in rate limiter?**
Not in the standard library, but [`golang.org/x/time/rate`](https://pkg.go.dev/golang.org/x/time/rate) provides a well-tested token bucket (`rate.NewLimiter`). Use it in production; this repo builds the algorithms from scratch to learn how they work.

## Further reading

- [`golang.org/x/time/rate`](https://pkg.go.dev/golang.org/x/time/rate): Go's token bucket rate limiter
- [Gin web framework](https://github.com/gin-gonic/gin)
- *System Design Interview – An Insider's Guide* (Vol. 1) by Alex Xu, the chapter "Design a Rate Limiter"

## Contributing

Found a bug, or want to add something (per-IP limiting, a Redis-backed limiter, tests)? Issues and pull requests are welcome.
