# Signal: monitor impact, chapter 1

A learning prototype: paste a Git diff, paste or upload a JSON monitor inventory, and inspect potential monitoring impacts. React handles the interface; Go validates inputs and calls OpenAI. Nothing changes your monitors.

## Run locally

Requires Node.js 20.19+ (or 22.12+) and Go 1.22+.

```sh
cd web
npm install
npm run build
cd ..
go run ./server
```

Open http://127.0.0.1:8080. Click **Load example**, then **Analyze monitor impact** in Demo mode. Demo is an exact-string heuristic, not an AI model. An unrelated monitor returns insufficient context because the heuristic cannot reason about it.

For development, run `go run ./server` at the project root and `npm run dev` in `web` in separate terminals. Vite proxies `/api` to Go.

## Enable real AI

Set `OPENAI_API_KEY` and `OPENAI_MODEL` in your server's environment before starting it. Select a model available to your API project that supports the Responses API and Structured Outputs. Keep the key in the server environment; never paste it into this chat, the UI, or source files. The app does not load `.env` automatically.

Select **AI** in the interface. That mode sends the supplied diff and monitor inventory to OpenAI. Demo mode stays local. There is no automatic fallback: an AI error stays an error.

The implementation uses the [official Structured Outputs contract](https://developers.openai.com/api/docs/guides/structured-outputs): Responses API `text.format`, JSON Schema, and `strict: true`. `store: false` disables response storage; it is not a promise of zero data retention.

## Input contract

The uploaded file is an array, not a Datadog API export wrapper:

```json
[
  {"name": "Coffee Brew Success", "query": "service:coffee-machine \"Coffee_Brew: Complete\""},
  {"name": "Cleaning Errors", "query": "service:coffee-machine status:error"}
]
```

Names must be unique. Maximum: 100 monitors, a 100 KB diff, and a 200 KB request body. Smaller batches may be needed to fit model output limits.

## Your first lesson: follow one request

1. `web/src/main.jsx` turns your form into `POST /api/analyze`. The UI never calls the model directly.
2. `server/main.go` checks the request before spending model tokens. A token is a chunk of text the model processes; more input and output generally means more work and cost.
3. `server/prompt.txt` defines the review task, statuses, evidence rules, and uncertainty behavior. Your diff and queries are input data, not trusted instructions.
4. `modelReport` sends the prompt, inputs, and response schema to OpenAI. A schema is a contract for the shape of the answer.
5. `validate` checks monitor identity, coverage, status, explanations, and literal diff evidence. This catches some bad answers; a real quote can still support an incorrect interpretation.
6. React renders the result. You verify it against actual code, logs, and query behavior before acting.

**Try this exercise:** analyze the sample in demo mode. Then add another `+` line that still logs `Coffee_Brew: Complete`. Analyze again. Why does the warning disappear? The heuristic checks added lines, but it does not understand execution paths. That limitation motivates model reasoning and, later, evaluations.

## What to learn next

Before changing prompts, write down expected outcomes for: a renamed log message, an unrelated code change, an unchanged message moved between lines, a structured-field rename, and a severity change. Run the same cases after every prompt revision. Track missed impacts and false alarms separately. A plausible explanation is not a passing evaluation.

We deliberately have no embeddings, retrieval, database, or autonomous agent yet. For small inventories, the whole inventory fits in one request. Add retrieval only when input size or evaluation results justify it.

## Validation

`go test ./...` checks demo behavior, rejects fabricated evidence and altered monitor identities, checks invalid inputs, and tests the model HTTP contract with a fake server. `cd web && npm run build` checks the production frontend build. The fake server does not measure model quality. Real-model behavior requires your API credentials and a labeled evaluation set.

This is a localhost prototype, with no authentication, persistence, GitHub integration, or Datadog integration. It is not prepared for public hosting.

## Chapter 2: GitHub pull-request automation

The repository is https://github.com/kishanrajacfa/monitor-impact.

The same analyzer now works without a browser or web server:

```sh
go run ./server -diff examples/brew.diff -monitors monitors.json -mode demo -output report.json -summary report.md
```

For a real model request, configure `OPENAI_API_KEY` and `OPENAI_MODEL` in the shell and replace `-mode demo` with `-mode ai`. CLI mode defaults to AI and does not start an HTTP server. Findings remain advisory; a successful analysis exits zero even if it finds potential impacts. Invalid inputs, unavailable credentials, API errors, and invalid reports exit nonzero.

### Enable GitHub Actions

1. In GitHub, open this repository's **Settings → Secrets and variables → Actions**.
2. Under **Secrets**, create a repository secret named `OPENAI_API_KEY` using your replacement API key. Do not commit the key or paste it into chat.
3. Optionally, under **Variables**, add `OPENAI_MODEL`. The workflow defaults to `gpt-4.1-mini`.
4. Open **Actions → Monitor impact → Run workflow** on `main`. This manual run analyzes `examples/brew.diff`, making it a quick integration check before opening a PR.
5. Inspect the run summary and download `monitor-impact-report`. Confirm its JSON has `mode: ai` and the expected model. Manual runs make real API requests too.
6. Create a branch, change the log message in `examples/brewing.go` from `Coffee_Brew: Complete` to `Your coffee is ready`, commit, push, and open a PR to `main`.
7. Open the PR's **Checks** and select **Monitor impact** to read the run summary. Updating the PR reruns the analysis. Reports are retained for seven days; API errors fail the check, while findings themselves do not.

Replace `monitors.json` with your real inventory before expecting useful coverage. The workflow reads this file from the PR's base commit, so changes to monitor definitions take effect after merging them. All supplied diff text and monitor queries are sent to OpenAI.

### How the workflow works

`.github/workflows/monitor-impact.yml` responds to PR changes or a manual run. It checks out the base commit's analyzer and inventory, collects the three-dot Git diff from the merge base to the PR head, and invokes the CLI with the API key only in the analysis step. No PR code is built or executed. The JSON and readable report are uploaded as run artifacts, and the readable report appears in the Actions summary. Model/input text is HTML-escaped before rendering.

The first version processes non-draft PRs from branches in the same repository. Fork and Dependabot PRs are skipped because they do not receive the model secret. Do not enable secret access for fork workflows to work around that limitation. Only trusted collaborators should have write access to this learning repository. We use `pull_request`, not `pull_request_target`, and only read permission for the GitHub token. There is no PR-comment bot or automatic query update yet.

Diffs over 100 KB are rejected rather than silently truncated. This avoids presenting a partial review as complete. Later we can add explicit batching and coverage reporting.

Model reports are validated before use. If a completed response fails evidence or monitor-identity validation, the analyzer sends the failed report and validation feedback for one repair attempt. A second invalid answer fails the run; the checks are never relaxed. A repair can add one model call and its associated latency and cost. This improves structural reliability, not proof that the model's interpretation is correct.

## Coffee-machine demo story

Imagine a coffee machine with two monitors. **Coffee Brew Success** counts logs containing `Coffee_Brew: Complete`. **Cleaning Errors** searches for `service:coffee-machine status:error`.

The sample PR makes three changes:

- Rename the brew-complete message to `Your coffee is ready`. The success monitor may stop matching.
- Lower a failed cleaning cycle from ERROR to WARN. The error monitor may miss it, depending on how severity maps to `status`.
- Change the coffee menu label. That changes display text without changing a log event.

Demo narration: “The machine still makes coffee, but our dashboard may stop counting it because the log text changed. The reviewer compares the code change with our monitor queries and suggests what to verify.” These are fictional examples.

For small diffs (up to 199 distinct changed lines), the model selects an evidence line ID from an enumeration. The server expands the ID to the exact changed line, preserving tabs, quotes, and patch markers. Larger diffs retain free-text evidence with the same literal-substring validation and bounded repair. Schema constraints ground the quote; they do not prove the interpretation.
