# AI ABAP Code Review Service

An AI-powered code review service for SAP ABAP, running on **SAP BTP Cloud Foundry**.
Users submit a transport request ID via a web UI; a Claude agent autonomously fetches ABAP source objects from the on-premise SAP system via ADT, and returns a structured, printable markdown review.

## Architecture

```mermaid
flowchart LR
    Browser([Browser]) --> AR[SAP Approuter\nJWT login]
    AR -->|GET /| Go[Go backend · Gin\nHTMX UI + review API]
    AR -->|POST /api/reviews| Go
    Go -->|tool calls| Agent[Claude Agent\nclaude-opus-4]
    Agent -->|ADT via adtler| DST[Destination Service]
    DST --> CC[Cloud Connector]
    CC -->|HTTPS + Basic Auth| SAP[On-premise SAP\nABAPADT]

    subgraph SAP BTP Cloud Foundry
        AR
        Go
        DST
    end
```

## Why direct ADT wiring, not an MCP server

We considered using [aibap.mcp](https://github.com/Hochfrequenz/aibap.mcp) (also by Hochfrequenz) as the SAP integration layer.
We decided against it because aibap.mcp is a local stdio process: it cannot receive the SOCKS5 proxy configuration that BTP injects at runtime into CF apps, so it has no path through the Cloud Connector to the on-premise SAP system.
Integrating it would mean a second CF app and the same SOCKS5 transport-injection work — with no meaningful gain for the read-only scope we need.

Direct wiring via [adtler](https://github.com/Hochfrequenz/adtler) (the Go ADT client library) keeps everything in a single CF app with BTP auth fully wired.
See [issue #7](../../issues/7) for the full analysis; revisit if the SAP system moves to the cloud or write operations become in scope.

## Quick start

1. **If deploying your own instance:** fill in `config.yml` (`app.name`, `app.module`, `cf.*`, `examples.destination_name`, `examples.sap_client`) and run `go run ./cmd/apply-config` — see [#2](../../issues/2) for the full field list
2. Set `ANTHROPIC_API_KEY` in your CF environment: `cf set-env <app-name> ANTHROPIC_API_KEY sk-ant-...`
3. Customize the review prompt: edit `internal/agent/prompts/review_prompt.md`
4. Run `go run ./cmd/apply-config` to rewrite the tree for your fork
5. Cross-compile the binary (`make build-linux` or `.\scripts\build.ps1`), then `cf push --vars-file vars.yml`
6. Open `https://<app-name>-web.<domain>/` and enter a transport request number

## Local development

**The server cannot run locally without BTP.**
`cmd/server/main.go` calls `btp.LoadEnv()` on startup, which reads `VCAP_SERVICES` and `VCAP_APPLICATION` — CF-injected environment variables that are absent on a developer laptop.
If they are missing the server refuses to start.
This is intentional: there is no meaningful stub mode for the three-leg BTP dance (XSUAA → Destination → Cloud Connector).

Unit tests (`go test ./...`) run without any BTP or SAP credentials — they use fakes throughout.

For integration tests against a real SAP system see [issue #6](../../issues/6) — the `internal/agent/` tests can connect directly to SAP without the Cloud Connector, so only `SAP_INTEGRATION_*` env vars are needed, not a full BTP stack.

## How it works

1. **Submit** — the user enters a transport request ID (e.g. `DEVK900123`) at `GET /`.
2. **Create job** — `POST /api/reviews` validates the TR ID, creates an async review job, and returns a link to `GET /reviews/:id`.
3. **Agent runs** — a Claude tool-use loop (`internal/agent/runner.go`) calls three ADT tools:
   - `list_tr_objects` — lists all ABAP objects in the transport request
   - `fetch_source` — fetches source for programs, interfaces, and classes
   - `fetch_class_includes` — fetches class definitions, implementations, and test includes
4. **Review ready** — the agent writes a structured markdown review.
   `GET /reviews/:id` polls every 3 s until the job is done, then renders printable HTML via goldmark.

ADT calls travel through the BTP Connectivity SOCKS5 proxy to the on-premise SAP system using the destination configured in `config.yml`.

## Deployed instance (Hochfrequenz)

| | URL |
|---|---|
| **Web UI** (XSUAA login required) | [ai-abap-code-review-service-web.cfapps.eu10.hana.ondemand.com](https://ai-abap-code-review-service-web.cfapps.eu10.hana.ondemand.com/) — on the login page, choose **Default Identity Provider** |
| Health | [/healthz](https://ai-abap-code-review-service.cfapps.eu10.hana.ondemand.com/healthz) |
| Version | [/version](https://ai-abap-code-review-service.cfapps.eu10.hana.ondemand.com/version) |

CI/CD: deployment is triggered by **publishing a GitHub Release** — not by push to `main`.
The workflow (`.github/workflows/deploy.yml`) cross-compiles the binary, runs the full gate (test + lint + fmt), pushes to the `dev` space in `HF Dev Account_hf-cf` on `eu10`, and smoke-tests `/healthz` and `/version`.

## Operations notes (HF deployment)

Findings from first deployment — documented here so the next person doesn't have to rediscover them.

### Finding the SAP technical user

The BTP Destination `HF_S4` authenticates to the on-premise SAP system with a technical username and password (BasicAuthentication).
To see which user that is:

> BTP cockpit → **Connectivity → Destinations → HF_S4** → Authentication section → **User** field

Currently: **`metzej`**.
The technical user must have `SAP_BC_TRANSPORT_ADMINISTRATOR` in SAP (SU01 → Roles tab) to list transport requests via ADT.
Without it, all TR-listing calls return an empty response with HTTP 200 — no error, just no data.

### XSUAA login: choose Default Identity Provider

The login page shows multiple identity providers.
Always choose **Default Identity Provider** (SAP ID Service / accounts.sap.com).
Corporate SSO is listed separately and will not work for this app.

### Transport request suggestions: why SQL instead of the ADT organizer tree

The standard ADT endpoint for listing open TRs (`GET /sap/bc/adt/cts/transportrequests` with Accept `application/vnd.sap.adt.transportorganizertree.v1+xml`) returns an empty `<tm:root/>` on this S/4HANA system.
Root cause: the system classifies its transport requests as `KORRDEV="SYST"` or `"CUST"` instead of the standard `"K"` (workbench).
The organizer tree endpoint silently ignores non-K requests.

Workaround implemented in `internal/adtclient/sqllister.go`: query `E070` and `E07T` directly via `RunQuery` (ADT data preview SQL API).
This returns all request types regardless of KORRDEV.
See [adtler issue #63](https://github.com/Hochfrequenz/adtler/issues/63) for the full root cause analysis.

### `/healthz` returns 503 when `ANTHROPIC_API_KEY` is missing

The health endpoint checks for required env vars at runtime.
If the key is missing it returns `503 {"status":"unhealthy","missing":["ANTHROPIC_API_KEY"]}` — this is intentional.
Set the key via `cf set-env ai-abap-code-review-service ANTHROPIC_API_KEY sk-ant-...` then `cf restage`.

### JWT `user_name` is an email, not a SAP username

The XSUAA JWT claim `user_name` contains the user's BTP email address (e.g. `konstantin.klein@hochfrequenz.de`).
SAP CTS stores usernames as short login IDs (`METZEJ`, `KLEINK`).
These cannot be mapped automatically — do not use `user_name` as a SAP user filter.

## Customisation

| What | Where |
| ---- | ----- |
| Review prompt | `internal/agent/prompts/review_prompt.md` |
| AI model | `reviewModel` constant in `internal/agent/runner.go` |
| Token budget | `reviewMaxTokens` constant in `internal/agent/runner.go` |
| Persistence (swap in-memory store) | implement `reviewstore.JobStore` in `internal/reviewstore/store.go` |

## License

MIT, see [LICENSE](LICENSE).
