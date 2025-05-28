package dynlb_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dynlb "github.com/podocarp/dynlb/src"
	"github.com/stretchr/testify/assert"
	"golang.org/x/time/rate"
)

func NewDownstreams(rates ...int) []dynlb.Handler[int, int] {
	downstreams := make([]dynlb.Handler[int, int], len(rates))
	for i, r := range rates {
		rateLimit := rate.NewLimiter(rate.Limit(r), 1)
		downstreams[i] = dynlb.Handler[int, int]{
			EstCap: 0,
			Dispatch: func(ctx context.Context, param int) (int, error) {
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

func TestGenericWeightedRoundRobin(t *testing.T) {
	// put target rates into this array, too high a rate may lead to
	// inaccurate results...
	rates := []int{1, 5, 1}
	// time to run the test for, the longer the more accurate it is
	secondsToRun := 5

	rr := dynlb.NewWeightedRoundRobin(rates...)

	downstreams := NewDownstreams(rates...)
	completions := make([]*atomic.Int32, len(rates))
	for i := range rates {
		completions[i] = &atomic.Int32{}
	}

	ctx, cancel := context.WithCancel(context.Background())
	timer := time.NewTimer(time.Duration(secondsToRun) * time.Second)
	var wg sync.WaitGroup
L:
	for {
		select {
		case <-timer.C:
			cancel()
			wg.Wait()
			break L
		default:
			wg.Add(1)
			i := rr.Dispatch()
			go func() {
				defer wg.Done()
				if _, err := downstreams[i].Dispatch(ctx, i); err == nil {
					completions[i].Add(1)
				}
			}()
			time.Sleep(time.Millisecond)
		}
	}

	for i := range rates {
		actualRate := float64(completions[i].Load()) / float64(secondsToRun)
		assert.InDelta(t, rates[i], actualRate, 1, "hander %d")
	}
}
