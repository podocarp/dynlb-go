# What's this?

This is a simple adaptive weighted round robin load balancer for golang.
It tries to guess the actual weights of the handlers you throw at it instead of you having to configure statically pre-known values.
Right now this does not provide rate limiting despite knowing the actual rates. This is a planned feature.

A super simple demo is available that shows the rebalancing between two handlers with different rate limits:
```
$ go run ./cmd/demo/
curr weights [50 50]
curr weights [39 60]
curr weights [35 64]
curr weights [39 60]
curr weights [35 64]
curr weights [26 73]
curr weights [31 68]
curr weights [26 73]
^Cslow handler called 9 times, fast handler called 21 times
```

## Getting started

```
go get github.com/podocarp/dynlb-go
```

First you might want to define a few handlers.
```
	handlers := make([]lb.Handler[int, int], n)

  handlers[i] = lb.Handler[int, int]{
    EstCap:   1,
    Dispatch: func(ctx context.Context, param int) {
      // some long winded computation/fetch your apis/whatever
    },
  }
```
Then register your handlers with the load balancer and start the lb.
```
lb := lb.NewLoadBalancer(handlers...)
lb.Start()
```
Dispatch calls to any of the handlers with `lb.Dispatch`.
After you're done, you can clean up with `lb.Destroy`.
