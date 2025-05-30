package utils

import (
	"context"

	"github.com/podocarp/dynlb-go/lb"
	"golang.org/x/time/rate"
)

func NewRateLimitedDownstreams(rates ...int) []lb.Handler[int, int] {
	downstreams := make([]lb.Handler[int, int], len(rates))
	for i, r := range rates {
		rateLimit := rate.NewLimiter(rate.Limit(r), 1)
		downstreams[i] = lb.Handler[int, int]{
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

func NewDownstreamsThatError(rates ...int) []lb.Handler[int, int] {
	downstreams := make([]lb.Handler[int, int], len(rates))
	for i, r := range rates {
		rateLimit := rate.NewLimiter(rate.Limit(r), 1)
		downstreams[i] = lb.Handler[int, int]{
			EstCap: 0,
			Dispatch: func(ctx context.Context, param int) (int, error) {
				if !rateLimit.Allow() {
					return 0, lb.ErrExceedCap
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
