# Velocity comparative benchmarks

This independent Go module deliberately contains Gin, Echo, and Fiber only as
benchmark dependencies. The Velocity framework module one directory above has
no third-party dependencies.

Pinned competitors:

- Gin `v1.12.0`
- Echo `v4.15.4`
- Fiber `v3.5.0`

Run after fetching dependencies:

```sh
go test -run '^$' -bench . -benchmem -count=5
```

Benchmarks cover a static route, parameter route, three no-op middleware layers,
and strict JSON decode/respond behavior. Gin, Echo, and Velocity use their native
`net/http` handlers with the same in-memory request/response objects. Fiber's
official `App.Test` adapter is used because Fiber natively uses `fasthttp`, not
`net/http`; its numbers are useful, but are reported separately from the direct
`net/http` comparison rather than treated as an identical transport path.

Run the suite on a quiet target machine and compare medians from several runs.
Do not use one machine's output as a universal performance claim.

The most recent measured results are recorded in `../BENCHMARKS.md`.
