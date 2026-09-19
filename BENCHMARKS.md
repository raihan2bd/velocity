# Benchmark results

The reproducible suite is in [`benchmarks/`](benchmarks/). It pins Gin 1.12.0,
Echo 4.15.4, and Fiber 3.5.0 without adding any dependency to Velocity itself.

Command:

```sh
go test -C benchmarks -run '^$' -bench '^Benchmark(StaticRoute|ParameterRoute|MiddlewareChain|JSONRoundTrip)$' -benchmem -benchtime=700ms -count=5
```

Environment: Windows amd64, Go 1.27.0, AMD Ryzen 7 7700. The table shows the
median of five runs. Lower is better.

| Workload | Velocity | Gin | Echo | Fiber `App.Test` |
| --- | ---: | ---: | ---: | ---: |
| Static route | 24.11 ns, 0 alloc | 25.04 ns, 0 alloc | 23.30 ns, 0 alloc | 6839 ns, 22 alloc |
| Two-parameter route | 46.28 ns, 0 alloc | 40.97 ns, 0 alloc | 50.45 ns, 0 alloc | 7643 ns, 22 alloc |
| Three no-op middleware layers | 31.66 ns, 0 alloc | 34.19 ns, 0 alloc | 62.52 ns, 3 alloc | 7388 ns, 22 alloc |
| Strict JSON decode and JSON response | 1064 ns, 12 alloc | 1095 ns, 14 alloc | 1225 ns, 11 alloc | 16442 ns, 46 alloc |

Gin, Echo, and Velocity are driven through native `net/http` handlers using
the same in-memory request and response objects. Fiber natively uses `fasthttp`;
its official `App.Test` adapter is shown for repeatability but is not a
transport-equivalent comparison.

These results show Velocity is competitive and wins two focused workloads on
this machine, not that it is universally fastest. Run the command on the
deployment CPU with representative routes, middleware, payload sizes, TLS, and
concurrency before selecting a framework.
