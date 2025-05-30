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

// Let's set up a mock handler that is rate limited in a certain way but pretend
// we don't know about the rate limit. We'll let the load balancer figure it out
// on its own. We're also going to count the number of times it's called so we
// can double check the figures afterwards.
type myHandler struct {
	rateLimiter *rate.Limiter
	numCalls    *atomic.Int32
}

func NewHandler(quota int) *myHandler {
	return &myHandler{
		rateLimiter: rate.NewLimiter(rate.Limit(quota), 1),
		numCalls:    &atomic.Int32{},
	}
}

func (h *myHandler) Call(ctx context.Context, param int) (int, error) {
	err := h.rateLimiter.Wait(ctx)
	if err != nil {
		return 0, err
	}
	h.numCalls.Add(1)
	return param * 2, nil
}

func Example_basicRateLimit() {
	slowHandler := NewHandler(1)
	fastHandler := NewHandler(4)
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
