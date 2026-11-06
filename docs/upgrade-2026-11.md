# November 2026 Upgrade Record

This record describes the implementation approved on 2026-11-06. It is not a
production deployment record. Tests use temporary databases and local tile
fixtures; no user database or existing downloads are used.

## Delivery Status

| Batch | State |
| --- | --- |
| Portable rules, dependency review, main completion and handoff | Committed: `da33be6` |
| Download configuration and format correctness | In validation |
| Automatic recovery, queue slots and storage performance | Pending |
| Dependency/toolchain upgrade | Pending |
| Cesium terrain generation and offline preview | Pending |
| Source selection, paginated history and responsive interactions | Pending |
| Manuals, reproducible images, native packages and final main checks | Pending |

## Download Configuration Regressions

The new `download_policy_regression_test.go` reproduced all four backend defects
on the previous implementation: three TCP connections instead of one; configured
404/503 retry decisions reversed; explicit zero retries changed to two; real
WebP rejected as an unknown image format. The same four tests pass after repair.

Default HTTP requests now reuse connections. A source may explicitly set a
`Connection: close` header when it actually needs one. Source `retry_statuses`
controls HTTP retry decisions in both attempts and extra passes; transport
failures retain their normal retry behavior. `max_retries = 0` disables extra
attempts, while `retry_passes` remains a separate setting. WebP is validated and
preserved without image recompression.

The source catalogue exposes only concurrency and minimum-delay values under
`fetch_policy`; source headers and proxy credentials are not included.

Validation is recorded in each bilingual commit. Full upgrade acceptance,
benchmarks and platform checks remain pending until their batches finish.
