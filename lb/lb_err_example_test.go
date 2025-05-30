package lb_test

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/podocarp/dynlb-go/lb"
	"golang.org/x/time/rate"
)

// This is another kind of handler that is rate limited but instead of making
// you wait it just errors back at you (public APIs, etc.).
type myHandlerThatErrs struct {
	rateLimiter *rate.Limiter
	numCalls    *atomic.Int32
}

func NewHandlerThatErrs(quota int) *myHandlerThatErrs {
	return &myHandlerThatErrs{
		rateLimiter: rate.NewLimiter(rate.Limit(quota), 1),
		numCalls:    &atomic.Int32{},
	}
}

func (h *myHandlerThatErrs) Call(ctx context.Context, param int) (int, error) {
	if !h.rateLimiter.Allow() {
		// Throw the special lb.ErrExceedCap to let the load balancer
		// know you've been rate limited and need to retry! Otherwise it
		// will think it's a regular error and bubble it back to the
		// caller.
		return 0, lb.ErrExceedCap
	}
	return param * 2, nil
}

func Example_errRateLimit() {
	slowHandler := NewHandlerThatErrs(1)
	fastHandler := NewHandlerThatErrs(5)
	lb := lb.NewLoadBalancer(
		lb.Handler[int, int]{
			EstCap:   0,
			Dispatch: slowHandler.Call,
		},
		lb.Handler[int, int]{
			EstCap:   0,
			Dispatch: fastHandler.Call,
		})

	// start the load balancer
	lb.Start()

	ctx := context.TODO()
	ticker := time.NewTicker(time.Second)   // inspect weights every second
	timer := time.NewTimer(5 * time.Second) // stop after 5s

	for {
		select {
		case <-timer.C:
			fmt.Printf(
				"slow handler called %d times, fast handler called %d times",
				slowHandler.numCalls.Load(),
				fastHandler.numCalls.Load(),
			)
			os.Exit(0)
		case <-ticker.C:
			fmt.Println("curr weights", lb.GetWeights())
		default:
			go lb.Dispatch(ctx, 1)
			time.Sleep(200 * time.Millisecond)
		}
	}
}
