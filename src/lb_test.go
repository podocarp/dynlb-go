package dynlb_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	dynlb "github.com/podocarp/dynlb/src"
	"github.com/stretchr/testify/assert"
	"golang.org/x/time/rate"
)

// Tests that the estimated weights slowly converge to the actual rate.
func TestEstWeightsImprove(t *testing.T) {
	// put target rates into this array, too high a rate may lead to
	// inaccurate results...
	rates := []int{1, 5, 2}
	// time to run the test for, the longer the more accurate it is
	secondsToRun := 5
	acceptableDelta := 10.0 // percentage points

	downstreams := NewDownstreams(rates...)
	lb := dynlb.NewLoadBalancer(downstreams...)
	lb.Start()

	ctx, cancel := context.WithCancel(context.Background())
	timer := time.NewTimer(time.Duration(secondsToRun) * time.Second)
	var wg sync.WaitGroup

L:
	for {
		select {
		case <-timer.C:
			cancel()
			lb.Destroy()
			wg.Wait()
			break L
		default:
			wg.Add(1)
			go func() {
				defer wg.Done()
				lb.Dispatch(ctx, 1)
			}()
			time.Sleep(time.Millisecond)
		}
	}

	wg.Wait()
	total := 0
	for _, r := range rates {
		total += r
	}
	fmt.Println(lb.GetWeights())
	for i, w := range lb.GetWeights() {
		weight := float64(rates[i]) / float64(total) * 100
		assert.InDelta(t, weight, w, acceptableDelta, "handler %d", i)
	}
}

func NewDownstreamsThatError(rates ...int) []dynlb.Handler[int, int] {
	downstreams := make([]dynlb.Handler[int, int], len(rates))
	for i, r := range rates {
		rateLimit := rate.NewLimiter(rate.Limit(r), 1)
		downstreams[i] = dynlb.Handler[int, int]{
			EstCap: 0,
			Dispatch: func(ctx context.Context, param int) (int, error) {
				if !rateLimit.Allow() {
					return 0, dynlb.ErrExceedCap
				}
				err := rateLimit.Wait(ctx)
				if err != nil {
					return 0, err
				}
				return param, nil
			},
		}
	}

	return downstreams
}

// In this test the handlers'  rate limiting doesn't block but instead return an
// error when the rate limit is exceeded. In this case we can still learn the
// approximate weights but slower.
func TestErrBackoff(t *testing.T) {
	// put target rates into this array, too high a rate may lead to
	// inaccurate results...
	rates := []int{2, 1, 10}
	// time to run the test for, the longer the more accurate it is
	secondsToRun := 5
	acceptableDelta := 20.0 // percentage points

	downstreams := NewDownstreamsThatError(rates...)
	lb := dynlb.NewLoadBalancer(downstreams...)
	lb.BackoffUnit = 10 * time.Millisecond
	lb.BackoffMaxExponent = 5
	lb.UpdateInterval = 1000 * time.Millisecond
	lb.Start()

	ctx, cancel := context.WithCancel(context.Background())
	timer := time.NewTimer(time.Duration(secondsToRun) * time.Second)
	var wg sync.WaitGroup
L:
	for {
		select {
		case <-timer.C:
			cancel()
			lb.Destroy()
			wg.Wait()
			break L
		default:
			wg.Add(1)
			go func() {
				defer wg.Done()
				lb.Dispatch(ctx, 1)
			}()
			time.Sleep(time.Millisecond)
		}
	}

	wg.Wait()
	total := 0
	for _, r := range rates {
		total += r
	}
	fmt.Println(lb.GetWeights())
	for i, w := range lb.GetWeights() {
		weight := float64(rates[i]) / float64(total) * 100
		assert.InDelta(t, weight, w, acceptableDelta, "handler %d", i)
	}
}
