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

## Why direct ADT wiring instead of mcp-server-abap

[mcp-server-abap](https://github.com/Hochfrequenz/mcp-server-abap) is a production-quality MCP server that exposes ~50 SAP ADT tools and is also maintained by Hochfrequenz.
The obvious question is: why not use it here instead of implementing ADT calls directly?

**The short answer:** the BTP Cloud Connector makes it impractical.

Three options were evaluated (see [issue #7](../../issues/7) for the full analysis):

**Option A — Direct ADT wiring (current).**
The Go service calls SAP via adtler, routing through BTP's SOCKS5 Connectivity proxy.
Single deployment unit, BTP auth fully wired, tool calls are in-process function calls.
This is what we built.

**Option B — mcp-server-abap as a BTP CF sidecar.**
mcp-server-abap is designed as a trusted local stdio process — it has no built-in network auth.
Exposing it as a CF app requires adding an auth layer.
More critically, BTP's Cloud Connector transport (the SOCKS5 proxy + `Proxy-Authorization` dance) is not built into mcp-server-abap.
Bridging it would require the same custom transport injection we did for adtler (see [adtler PR #61](https://github.com/Hochfrequenz/adtler/pull/61)), but for a separate service — two CF apps to deploy, monitor, and scale.

**Option C — Claude's remote MCP support.**
Claude API supports MCP servers via `mcp_servers`, where Anthropic's infrastructure calls the MCP server directly.
This requires a publicly reachable HTTPS endpoint for the MCP server — which means SAP must be reachable from the public internet.
For on-premise SAP behind a Cloud Connector this is a non-starter.

**Revisit if:** the scope grows to write operations (activate, create object) or the SAP system moves to BTP ABAP Environment / S/4HANA Cloud where the Cloud Connector is no longer in the path.
In that case, Option B or C become viable.

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

## Customisation

| What | Where |
| ---- | ----- |
| Review prompt | `internal/agent/prompts/review_prompt.md` |
| AI model | `reviewModel` constant in `internal/agent/runner.go` |
| Token budget | `reviewMaxTokens` constant in `internal/agent/runner.go` |
| Persistence (swap in-memory store) | implement `reviewstore.JobStore` in `internal/reviewstore/store.go` |

## Open issues / setup required

The following GitHub issues track one-time setup tasks required before the service is usable:

- [#1 Set ANTHROPIC_API_KEY in CF environment](../../issues/1)
- [#2 Configure config.yml with your SAP destination, client, and fork settings](../../issues/2)
- [#3 Customize the ABAP review prompt for your organisation](../../issues/3)
- [#4 Full deployment checklist (ordered, with verification steps)](../../issues/4)

## License

MIT, see [LICENSE](LICENSE).

---

Built on [go-sap-btp-cf-template](https://github.com/Hochfrequenz/go-sap-btp-cf-template) — for BTP deployment, forking, XSUAA auth, Cloud Connector wiring, and everything else about the plumbing, see the template README.
