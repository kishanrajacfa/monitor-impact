# Signal — Monitor Impact

Find log monitors that may stop matching after a code change.

Signal compares a Git diff with monitor queries and produces an evidence-based review: which monitors may be affected, why, and what to verify next. Use the browser for an individual review, the CLI for scripts, or GitHub Actions to analyze pull requests automatically.

**Status:** Local prototype with advisory reports. Signal does not modify monitors, update queries, or block merges based on findings.

## Example

A coffee machine changes its completion log:

```diff
- slog.Info("Coffee_Brew: Complete", "service", "coffee-machine")
+ slog.Info("Your coffee is ready", "service", "coffee-machine")
```

A monitor still searches for `service:coffee-machine "Coffee_Brew: Complete"`. Signal flags a potential impact, quotes the change, and recommends checking the new logs before updating the query.

The included examples use a fictional coffee machine. They also cover a cleaning failure changing from ERROR to WARN and an unrelated menu-label edit.

## Quick start

### Requirements

- Go 1.22 or later
- Node.js 20.19+ or 22.12+
- An OpenAI API key for AI analysis; no key is needed for Demo mode

### Run locally

```sh
git clone https://github.com/kishanrajacfa/monitor-impact.git
cd monitor-impact
npm --prefix web ci
npm --prefix web run build
go run ./server
```

Open [http://127.0.0.1:8080](http://127.0.0.1:8080). Click **Load example**, keep **Demo** selected, and click **Analyze monitor impact**.

Demo mode runs a local exact-string heuristic. It is useful for trying the interface, but it does not call an AI model or reason about code behavior. Stop the server with **Ctrl+C**.

### Enable AI analysis

Set these environment variables in the terminal that starts the server:

| Variable         | Purpose                                                                             |
| ---------------- | ----------------------------------------------------------------------------------- |
| `OPENAI_API_KEY` | An API key with access to the selected model                                        |
| `OPENAI_MODEL`   | A model supporting the Responses API and Structured Outputs, such as `gpt-4.1-mini` |

Provide the key through your shell or secret manager, then start the server:

```sh
export OPENAI_MODEL=gpt-4.1-mini
go run ./server
```

Restart an existing server after changing its environment. In the browser, select **AI · OpenAI model** before analyzing. Configuring credentials does not change the selected mode automatically.

AI mode sends the supplied diff and monitor queries to OpenAI and may incur API charges. Keep API keys out of source files and browser inputs. The app does not automatically load `.env` files. API failures are reported as errors; they do not fall back to Demo mode.

## Monitor inventory

Supply a JSON array of monitors, each with a unique `name` and a nonempty `query`:

```json
[
  {
    "name": "Coffee Brew Success",
    "query": "service:coffee-machine \"Coffee_Brew: Complete\""
  },
  {
    "name": "Cleaning Errors",
    "query": "service:coffee-machine status:error"
  }
]
```

- **Browser:** Paste or upload the inventory in the form.
- **CLI:** Pass the inventory file with `-monitors`.
- **GitHub Actions:** Edit [`monitors.json`](monitors.json). The workflow reads the inventory from the PR's base commit, so inventory updates take effect after they are merged.

There is no automatic sync with a monitoring platform. Provider-specific exports must be converted to this array format.

## Command-line usage

Run the sample without an API key:

```sh
go run ./server \
  -diff examples/brew.diff \
  -monitors monitors.json \
  -mode demo \
  -output report.json \
  -summary report.md
```

With the API environment variables configured, use `-mode ai` to call the model. CLI mode defaults to AI when `-mode` is omitted. Supplying `-diff` runs the CLI instead of starting the web server.

The command writes a JSON report and a readable Markdown report. It exits successfully when analysis completes, including when potential impacts are found. Invalid input, API errors, and rejected reports produce a nonzero exit code.

## Pull-request automation

The included [GitHub Actions workflow](.github/workflows/monitor-impact.yml) analyzes non-draft PRs from branches in the same repository when they are opened or updated. Fork and Dependabot PRs are skipped.

To enable it in a copy of this repository:

1. Open **Settings → Secrets and variables → Actions**.
2. Add a repository secret named `OPENAI_API_KEY`.
3. Optionally add a repository variable named `OPENAI_MODEL`. The default is `gpt-4.1-mini`.
4. Replace `monitors.json` with the monitor inventory to review against.
5. Open **Actions → Monitor impact → Run workflow** to test the integration using `examples/brew.diff`.

For a PR, open **Checks → Monitor impact** and follow the link to the Actions run. The report appears in the run summary and as a downloadable `monitor-impact-report` artifact, retained for seven days. Findings are advisory; execution or validation errors fail the check.

The workflow runs the analyzer from the base commit and treats the PR checkout as diff data. It does not build or execute the changed application code. The GitHub token has read-only repository-content permission. Only trusted collaborators should have write access to the repository and its workflows.

## Report format and validation

Each monitor receives one assessment:

| Status                 | Meaning                                                                           |
| ---------------------- | --------------------------------------------------------------------------------- |
| `potential_impact`     | A change may affect the monitor; inspect the evidence and verify it.              |
| `no_evidence`          | No impact was identified in the supplied diff. This is not a guarantee of safety. |
| `insufficient_context` | The available inputs do not support a reliable assessment.                        |

Findings include the original monitor name and query, diff evidence, an explanation, a suggested next step, and missing context.

AI responses use [Structured Outputs](https://developers.openai.com/api/docs/guides/structured-outputs). Go independently validates monitor identity, coverage, allowed statuses, explanations, and literal diff evidence. For small diffs, the model selects evidence line IDs that Go expands into exact source text. Larger diffs use free-text evidence with literal-substring validation.

A completed report that fails validation gets one repair attempt with feedback. A second invalid report is rejected. A repair can incur an additional model call. These checks improve report consistency and grounding; they do not establish that an interpretation is correct.

## Development

Run these commands in separate terminals from the repository root:

```sh
# Backend
go run ./server
```

```sh
# Frontend with live updates
npm --prefix web run dev
```

Open the URL printed by Vite. Its development proxy forwards `/api` requests to Go on port 8080.

### Project layout

```text
web/src/main.jsx       React inputs, API request, and report rendering
web/src/style.css      Interface styling
server/main.go         HTTP routes, model integration, and validation
server/prompt.txt      Model review instructions
server/cli.go          File-based analysis and report generation
server/*_test.go       Backend and CLI tests
monitors.json          Monitor inventory for automated analysis
examples/              Fictional coffee-machine fixtures
.github/workflows/     Pull-request workflow
```

### Checks and formatting

```sh
go test ./...
npm --prefix web run build
npm --prefix web run format:check
```

Format supported files with `npm --prefix web run format`. Format Go files with `gofmt`.

The Go tests cover input validation, demo behavior, model-request handling, evidence checks, bounded repair, and report generation. Model calls in those tests use a fake HTTP server; they do not measure real-model accuracy.

## Limits

- Up to 100 monitors, a 100 KB diff, and a 200 KB HTTP request body. Oversized diffs are rejected rather than truncated.
- Each model request has a 90-second timeout; CLI analysis has a 95-second overall timeout. Smaller batches may be needed to fit output limits.
- Reviews only use the supplied diff and queries. Signal cannot confirm deployment state, runtime behavior, or severity-to-status mappings without additional context.
- No authentication, rate limiting, persistence, or monitoring-platform connection. The web server binds to localhost and is not prepared for public hosting.
- Requests set `store: false`; this is not a zero-data-retention guarantee.
