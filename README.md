# What's this?

This is a simple adaptive weighted round robin load balancer for golang.
It tries to guess the actual weights of the handlers you throw at it instead of you having to configure statically pre-known values.

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

## Notes

- If you can't get close to full saturation on your downstreams, it doesn't really
matter. For example if your endpoints in total can handle 1k qps but you're only
sending 10qps, the weights aren't going to change that much. This is like using
a splitter to balance a single belt in Factorio, it doesn't work well unless its
getting backed up.
- There is no rate limiting on top of the load balancer. This is something
that's very easy with golang's `rate` package.
