# k6 loader for coderunner

This folder contains the first k6-based load test harness for the coderunner `POST /run` endpoint only. It does not include direct gRPC tests.

## Scripts

- `smoke.js`: low-risk validation against the `short` workload profile.
- `mixed-ramp.js`: staged ramp test for finding the latency knee under a more realistic command mix.
- `mixed-soak.js`: steady-state mixed workload for checking sustainable capacity with LLM-like commands.
- `long-form.js`: 5-minute constant-VU CPU stress test where each request runs for bounded 5 to 25 second compute work.

## Request contract

The runner endpoint expects this JSON body:

```json
{
  "code": "printf 'short-ok\\n'",
  "language": "bash"
}
```

The current agent executes the `code` field through `bash -c`, so the built-in workload profiles send shell commands. The mixed profiles use shell commands that invoke Python inside the container because the default agent image already includes Python and that keeps the workload self-contained.

## Default workload mix

- `short`: quick shell response
- `explore`: create a small directory tree, enumerate files, and read a subset back
- `json`: create a JSON file, read it, parse it, and aggregate values
- `cpu`: lighter compute-only Python loop for some worst-case coverage
- `network`: optional HTTP request profile, disabled by default because network availability can vary by deployment

Default weighted mix for the mixed scenarios:

- `short`: 70
- `explore`: 20
- `json`: 8
- `cpu`: 2
- `network`: 0

This keeps the mixed scenarios biased toward the kinds of short-lived file, directory, and data-manipulation commands that LLMs are more likely to generate when using the Go `/run` path. The dedicated `long-form.js` scenario remains the explicit CPU-bound stress suite.

## Usage

Run from the repository root:

```bash
k6 run load-test/loader/smoke.js
k6 run load-test/loader/mixed-ramp.js
k6 run load-test/loader/mixed-soak.js
k6 run load-test/loader/long-form.js
```

Target a remote coderunner instance:

```bash
CODERUNNER_BASE_URL=http://machine-1.local:8080 k6 run load-test/loader/mixed-ramp.js
```

## Useful environment variables

- `CODERUNNER_BASE_URL`: base URL for coderunner, default `http://localhost:8080`
- `CODERUNNER_RUN_PATH`: request path, default `/run`
- `CODERUNNER_REQUEST_TIMEOUT`: k6 request timeout, default `35s`
- `THINK_TIME_SECONDS`: optional sleep after each request, default `0`
- `EXPLORE_FILE_COUNT`: number of files created for the `explore` profile, default `48`
- `EXPLORE_DIR_COUNT`: number of top-level directories used for the `explore` profile, default `6`
- `EXPLORE_READ_COUNT`: number of files read back for the `explore` profile, default `8`
- `JSON_RECORD_COUNT`: number of JSON records created and parsed for the `json` profile, default `300`
- `CPU_PYTHON_ITERATIONS`: compute intensity for the `cpu` profile, default `750000`
- `NETWORK_URL`: target URL for the optional `network` profile, default `https://example.com`
- `NETWORK_TIMEOUT_SECONDS`: timeout for the optional `network` profile, default `5`
- `LONG_FORM_MIN_SECONDS`: minimum command duration for `long-form.js`, default `5`
- `LONG_FORM_MAX_SECONDS`: maximum command duration for `long-form.js`, default `25`
- `MIX_SHORT_WEIGHT`: short profile weight, default `70`
- `MIX_EXPLORE_WEIGHT`: explore profile weight, default `20`
- `MIX_JSON_WEIGHT`: json profile weight, default `8`
- `MIX_CPU_WEIGHT`: cpu profile weight, default `2`
- `MIX_NETWORK_WEIGHT`: network profile weight, default `0`

Ramp scenario tuning:

- `RAMP_START_RATE`
- `RAMP_PRE_ALLOCATED_VUS`
- `RAMP_MAX_VUS`
- `RAMP_TIME_UNIT`
- `RAMP_STAGE_1_TARGET` through `RAMP_STAGE_6_TARGET`
- `RAMP_STAGE_1_DURATION` through `RAMP_STAGE_6_DURATION`

Soak scenario tuning:

- `SOAK_RATE`
- `SOAK_TIME_UNIT`
- `SOAK_DURATION`
- `SOAK_PRE_ALLOCATED_VUS`
- `SOAK_MAX_VUS`

Smoke scenario tuning:

- `SMOKE_VUS`
- `SMOKE_DURATION`

Long-form scenario tuning:

- `LONG_FORM_VUS`
- `LONG_FORM_DURATION`
- `LONG_FORM_MIN_SECONDS`
- `LONG_FORM_MAX_SECONDS`

The long-form workload uses a time-bounded compute loop instead of `sleep`, so it is still the better choice for exposing CPU bottlenecks on the agent machine.

## Current checks

Each script validates:

- HTTP status is `200`
- response body is JSON
- response includes an `output` field
- response output includes the expected marker for the selected workload profile

The scripts also publish these k6 custom rates:

- `invalid_json_responses`
- `missing_output_responses`
- `command_error_responses`
