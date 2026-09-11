# hsdebug — home server debugger agent

`hsdebug` is a single Go binary that both **runs a local service** (`hsdebug server`)
and acts as its **client** (all other subcommands), following the Vault pattern.
It registers locally running home-server services, checks their health, and uses
the [opencode](https://opencode.ai) agent to debug the unhealthy ones.

## Commands

| Command | Purpose |
|---|---|
| `hsdebug server` | Start the local REST API + web UI (loopback by default). |
| `hsdebug register` | Register a loopback service (name, port, health path, optional prompt). |
| `hsdebug scan` | Auto-detect common home-server services on loopback (`--add` to register). |
| `hsdebug health [service]` | Check health of one or all services (green tick / red cross). |
| `hsdebug agent ...` | Configure the opencode agent (model, provider, credentials, probe). |
| `hsdebug debug [service]` | Debug unhealthy services (preflight gate + agent run). |
| `hsdebug doctor` | Self-diagnostics for hsdebug itself. |

### Global output flags (all commands)

- `--json` — JSON output
- `--markdown` — Markdown output
- `--jq=<expr>` — apply a jq filter (implies `--json`)
- `--no-redact` — disable output redaction for this command

### Redaction

Output secrets are masked according to a profile (config `redact.profile` or env
`HSDEBUG_REDACT`):

- `strict` (default) — known secret fields, declared-sensitive values, and
  name/shape heuristics.
- `known` — only known secret fields and declared-sensitive values.
- `off` — no masking (also `--no-redact` per command).

## Agent configuration

hsdebug drives opencode through an **isolated** opencode config at
`~/.config/hsdebug/opencode.json` (via `OPENCODE_CONFIG`), so it never disturbs
your own opencode setup.

```sh
# set the model (provider inferred from provider/model)
hsdebug agent model anthropic/claude-sonnet-4-5

# non-interactive credential "bypass" — stored 0600 and referenced via {file:}
hsdebug agent provider anthropic --key sk-...
# or reference an env var / existing file
hsdebug agent provider anthropic --key-env ANTHROPIC_API_KEY
hsdebug agent provider anthropic --key-file /path/to/key

# verify end-to-end that the AI actually responds (dry-run)
hsdebug agent probe
```

### AI dry-run (configurable)

Before any real debugging, `hsdebug debug` runs a **preflight gate**:

1. agent configured (model + credential),
2. opencode present,
3. **AI dry-run** — sends a side-effect-free prompt and confirms the model echoes
   a sentinel token.

The dry-run is configurable in `config.json`:

```json
"agent": {
  "probe": { "enabled": true, "sentinel": "HSDEBUG_OK", "timeoutSeconds": 60 }
}
```

## Docker

```sh
docker build -t hsdebug .
docker run -p 7654:7654 -v hsdebug-data:/data hsdebug
```

## Development

```sh
go build ./...
go test ./...
go test -race -cover ./...
```
