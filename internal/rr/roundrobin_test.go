package rr_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/podocarp/dynlb-go/internal/rr"
	"github.com/podocarp/dynlb-go/internal/utils"
	"github.com/stretchr/testify/assert"
)

func TestGenericWeightedRoundRobin(t *testing.T) {
	// put target rates into this array, too high a rate may lead to
	// inaccurate results...
	rates := []int{1, 5, 1}
	// time to run the test for, the longer the more accurate it is
	secondsToRun := 5

	roundRobin := rr.NewWeightedRoundRobin(rates...)

	downstreams := utils.NewRateLimitedDownstreams(rates...)
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
			i := roundRobin.Dispatch()
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
