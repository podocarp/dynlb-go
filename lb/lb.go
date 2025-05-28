package lb

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

type Handler[T any, U any] struct {
	// Estimated capacity of this handler, units of tasks per second
	EstCap float64
	// Dispatch function called when this handler is chosen
	Dispatch func(context.Context, T) (U, error)
}

type Config struct {
	// Exponential backoff highest exponent. Defaults to 10. This means a
	// max of 2^10 * Unit.
	BackoffMaxExponent int
	// Exponential backoff unit. Defaults to 100ms. With the default backoff
	// this gives a max backoff of 1.7 minutes.
	BackoffUnit time.Duration

	// Interval in which weights are refreshed. Defaults to one second.
	// Higher intervals make the weights converge faster but also cause a
	// little more latency from mutex locks
	UpdateInterval time.Duration

	// A smoothing factor in [0, 1], defaults to 0.8. Larger values means
	// new estimates are valued more than old values, which causes updates
	// to be faster but may cause fluctuations/jitter.
	SmoothingFactor float64
}

type LoadBalancer[T any, U any] struct {
	Config

	*weightedRoundRobin

	dispatch []func(context.Context, T) (U, error)
	calls    []atomic.Int32 // counter of tasks run successfully each tick
	caps     []float64      // estimated capacity of each handler, units of tasks per second
	totalCap float64        // sum of all caps

	mut  sync.Mutex
	done chan struct{}
}

func NewLoadBalancer[T any, U any](handlers ...Handler[T, U]) *LoadBalancer[T, U] {
	n := len(handlers)
	lb := LoadBalancer[T, U]{
		dispatch: make([]func(context.Context, T) (U, error), n),
		calls:    make([]atomic.Int32, n),
		caps:     make([]float64, n),
		totalCap: 0,
		mut:      sync.Mutex{},
		done:     make(chan struct{}, 2),
		weightedRoundRobin: &weightedRoundRobin{
			weights: make([]int, n),
		},
		Config: Config{
			BackoffMaxExponent: 10,
			BackoffUnit:        100 * time.Millisecond,
			UpdateInterval:     time.Second,
			SmoothingFactor:    0.8,
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
		if calls == 0 {
			continue
		}
		estCap := float64(calls) / l.UpdateInterval.Seconds()
		l.caps[i] = l.SmoothingFactor*estCap + (1-l.SmoothingFactor)*l.caps[i]
		l.calls[i].Store(0)
	}
}

// After updating any of the capacities, call this function to rebalance the
// other variables.
func (l *LoadBalancer[T, U]) updateWeights() {
	l.totalCap = 0
	for _, c := range l.caps {
		l.totalCap += c
	}
	maxWeight := 0
	for i, c := range l.caps {
		weight := int(c / l.totalCap * 100)
		l.weights[i] = weight
		if weight > maxWeight {
			maxWeight = weight
		}
	}
	l.rounds = maxWeight
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
	index := l.weightedRoundRobin.Dispatch()
	l.mut.Unlock()

	return l.tryDispatch(ctx, param, index)
}

// Returns the currently used weights. Doesn't really mean much, but useful for
// testing/debugging.
func (l *LoadBalancer[T, U]) GetWeights() []int {
	return l.weights
}
