package lb

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/podocarp/dynlb-go/internal/rr"
)

type HandlerFunc[T any, U any] func(context.Context, T) (U, error)

// A handler (or downstream) for the load balancer. When [LoadBalancer.Dispatch]
// is called, it will choose an appropriate handler and call the the supplied
// Dispatch function.
type Handler[T any, U any] struct {
	// Estimated capacity of this handler, units of tasks per second
	EstCap float64
	// Dispatch function called when this handler is chosen
	Dispatch HandlerFunc[T, U]
}

// Configuration for the load balancer. Should not be changed after you call
// [LoadBalancer.Start], it will cause data races.
type Config struct {
	BackoffMaxExponent int
	BackoffUnit        time.Duration
	UpdateInterval     time.Duration
	SmoothingFactor    float64

	// Exploration rate for ε-greedy algorithm
	ExplorationRate float64
	// Additive increase amount for AIMD
	AIMDIncrease float64
	// Multiplicative decrease factor for AIMD
	AIMDDecreaseFactor float64
}

type LoadBalancer[T any, U any] struct {
	Config

	*rr.WeightedRoundRobin

	dispatch   []HandlerFunc[T, U]
	calls      []atomic.Int32 // counter of tasks run successfully each tick
	rejections []atomic.Int32 // counter of ErrExceedCap each tick
	caps       []float64      // estimated capacity of each handler, units of tasks per second
	totalCap   float64        // sum of all caps

	mut  sync.Mutex
	done chan struct{}
}

func NewLoadBalancer[T any, U any](handlers ...Handler[T, U]) *LoadBalancer[T, U] {
	n := len(handlers)
	lb := LoadBalancer[T, U]{
		dispatch:           make([]HandlerFunc[T, U], n),
		calls:              make([]atomic.Int32, n),
		rejections:         make([]atomic.Int32, n),
		caps:               make([]float64, n),
		totalCap:           0,
		mut:                sync.Mutex{},
		done:               make(chan struct{}, 2),
		WeightedRoundRobin: rr.NewWeightedRoundRobin(make([]int, n)),
		Config: Config{
			BackoffMaxExponent: 10,
			BackoffUnit:        100 * time.Millisecond,
			UpdateInterval:     time.Second,
			SmoothingFactor:    0.5,
			ExplorationRate:    0.1,
			AIMDIncrease:       0.1,
			AIMDDecreaseFactor: 0.9,
		},
	}

	for i, ds := range handlers {
		lb.dispatch[i] = ds.Dispatch
		lb.caps[i] = max(ds.EstCap, 1)
	}

	lb.updateWeights()

	return &lb
}

func (l *LoadBalancer[T, U]) spin() {
	ticker := time.NewTicker(l.UpdateInterval)
	for {
		select {
		case <-ticker.C:
			l.mut.Lock()
			l.updateLoads()
			l.updateWeights()
			l.mut.Unlock()
		case <-l.done:
			ticker.Stop()
			return
		}
	}
}

// Starts the auto weight adjustment behavior. Without this it's just a dumb
// round robin scheduler.
func (l *LoadBalancer[T, U]) Start() {
	go l.spin()
}

// Stops the load balancer. You can still call `Dispatch` afterwards but the
// weights will stop updating, and there is no guarantee on its behavior. Don't
// do that!
func (l *LoadBalancer[T, U]) Destroy() {
	l.done <- struct{}{}
}

// Average the current loads into the existing capacities, and reset the load
// counters.
func (l *LoadBalancer[T, U]) updateLoads() {
	for i := range l.calls {
		calls := l.calls[i].Load()
		rejects := l.rejections[i].Load()

		// AIMD: additive increase for successes
		if calls > 0 {
			l.caps[i] += l.AIMDIncrease
		}

		// AIMD: multiplicative decrease for rejections
		if rejects > 0 {
			l.caps[i] *= l.AIMDDecreaseFactor
		}

		// Exponential smoothing for observed rate
		if calls > 0 || rejects > 0 {
			estCap := float64(calls) / l.UpdateInterval.Seconds()
			l.caps[i] = l.SmoothingFactor*estCap + (1-l.SmoothingFactor)*l.caps[i]
		}

		// Decay for idle handlers to prevent starvation
		if calls == 0 && rejects == 0 {
			l.caps[i] *= 0.99
		}

		l.caps[i] = max(l.caps[i], 0.1)
		l.calls[i].Store(0)
		l.rejections[i].Store(0)
	}
}

// After updating any of the capacities, call this function to rebalance the
// other variables.
func (l *LoadBalancer[T, U]) updateWeights() {
	l.totalCap = 0
	for _, c := range l.caps {
		l.totalCap += c
	}
	newWeights := make([]int, len(l.dispatch))
	for i, c := range l.caps {
		weight := int(c / l.totalCap * 100)
		newWeights[i] = weight
	}
	l.UpdateWeights(newWeights)
}

// Return this error to signal that the function has been called too quickly,
// triggers an exponential backoff to start.
var ErrExceedCap = errors.New("lb exceed capacity")

func (l *LoadBalancer[T, U]) backoff(i int) {
	exp := min(l.BackoffMaxExponent, i)
	time.Sleep(l.BackoffUnit * 1 << exp)
}

func (l *LoadBalancer[T, U]) tryDispatch(ctx context.Context, param T, index int) (U, error) {
	var res U
	var err error
	attempts := 0
L:
	for {
		select {
		case <-ctx.Done():
			return res, ctx.Err()
		default:
			res, err = l.dispatch[index](ctx, param)
			if !errors.Is(err, ErrExceedCap) {
				break L
			}
			l.rejections[index].Add(1)
			l.backoff(attempts)
			attempts++
		}
	}

	l.calls[index].Add(1)

	return res, err
}

// Tries to call one of the available handlers.
func (l *LoadBalancer[T, U]) Dispatch(ctx context.Context, param T) (U, error) {
	l.mut.Lock()
	var index int
	if l.ExplorationRate > 0 && len(l.dispatch) > 1 && rand.Float64() < l.ExplorationRate {
		index = rand.Intn(len(l.dispatch))
	} else {
		index = l.WeightedRoundRobin.Dispatch()
	}
	l.mut.Unlock()

	return l.tryDispatch(ctx, param, index)
}

// Returns the currently used weights. Doesn't really mean much, but useful for
// testing/debugging.
func (l *LoadBalancer[T, U]) GetWeights() []int {
	return l.WeightedRoundRobin.GetWeights()
}
