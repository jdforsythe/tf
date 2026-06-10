# tf

**A prettier, interactive terraform.**

`tf` wraps your existing `terraform` and turns the wall-of-text output of
`plan`, `apply`, and `destroy` into a clean, interactive TUI — live progress
with ETAs, and a collapsible plan tree you can actually read. Everything else
passes straight through.

![demo](demo.gif)

[![ci](https://github.com/jdforsythe/tf/actions/workflows/ci.yml/badge.svg)](https://github.com/jdforsythe/tf/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/jdforsythe/tf)](https://github.com/jdforsythe/tf/releases)
[![license](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

## Why

A `terraform plan` with a few dozen resources scrolls hundreds of lines of
diff past you, and the one line you care about — `Plan: 3 to add, 1 to
change, 2 to destroy` — is buried at the bottom. Applies are worse: long
resource names stream by with no sense of progress.

`tf` reads terraform's machine-readable `-json` UI stream and the structured
plan from `terraform show -json` instead of scraping text, and renders:

- **Live activity view** — during refresh and apply, an active list of
  resources: yellow with a spinner while running, flashing green and
  disappearing when done. Errors stick in red.
- **Progress headline** — progress bar, `done/total`, percent, active count,
  per-resource timing, and a naive ETA from the observed completion rate.
- **Collapsible plan tree** — resources grouped into
  `＋ create / ～ update / ± replace / － destroy`, collapsed to just names by
  default. Expand any resource for an attribute-level diff: `old → new`,
  `(known after apply)`, `(sensitive)`, and `# forces replacement` flags.
  Output changes and detected drift get their own sections.
- **A real approval gate** — for `apply`/`destroy` the plan tree *is* the
  confirmation prompt. Review, then press `a` `y`. Destroys get a red
  warning with the count.

## Install

```sh
brew install jdforsythe/tap/tf
```

or with Go:

```sh
go install github.com/jdforsythe/tf@latest
```

or grab a binary from [releases](https://github.com/jdforsythe/tf/releases),
or build from source with `make install`.

`tf` invokes whatever `terraform` is on your `PATH`. Point it elsewhere with
`TF_BIN` — OpenTofu works too: `TF_BIN=tofu tf plan`.

## Use

Use it exactly like terraform:

```sh
tf plan                    # live refresh → collapsible plan tree
tf plan -out=x.tfplan      # same, and keeps the plan file
tf apply                   # plan → review tree as approval gate → live apply
tf apply x.tfplan          # review a saved plan, then apply it
tf apply -auto-approve     # skip the review screen
tf destroy                 # same flow, with a red warning
tf init / state / fmt / …  # passed through untouched
```

When stdout isn't a terminal (CI, pipes, scripts), `tf` execs terraform
directly with your original arguments — no TUI, no `-json`, identical
behavior and exit codes. It's safe to alias: `alias terraform=tf` if you
like to live dangerously, though the whole point is that `tf` is shorter.

### Keys

| Key | Action |
|---|---|
| `↑`/`↓` or `j`/`k` | move |
| `enter` / `space` | expand / collapse resource |
| `e` / `c` | expand / collapse all |
| `pgup`/`pgdn`, `g`/`G` | page, jump to top/bottom |
| `a` then `y` | apply the plan |
| `q` / `esc` | quit (prints the plan summary) |

During a plan or apply, `q` or `ctrl-c` asks terraform to stop gracefully;
pressing it again force-kills.

### Flag handling

`apply` and `destroy` always plan to a file first (the review tree needs it,
and it's how `-json` mode supports approval at all). Plan-shaping flags —
`-var`, `-var-file`, `-target`, `-replace`, `-destroy`, … — are forwarded to
the plan step; `-parallelism` and `-lock*` go to both steps. `-json`,
`-no-color`, and `-input` are managed by the wrapper. Errors exit `1`.

## How it works

```
tf apply
 ├─ terraform plan -json -out=<tmp>   → streamed events drive the activity view
 ├─ terraform show -json <tmp>        → structured diff builds the review tree
 └─ terraform apply -json <tmp>       → streamed events drive the progress view
```

No text scraping, no PTY tricks — just terraform's
[machine-readable UI](https://developer.hashicorp.com/terraform/internals/machine-readable-ui)
and [JSON plan representation](https://developer.hashicorp.com/terraform/internals/json-format).

## Development

```sh
make build            # build ./tf
cd demo && terraform init && ../tf apply    # play with the demo stack
vhs demo.tape         # re-record demo.gif (brew install vhs)
```

The `demo/` stack uses only the builtin `terraform_data` resource, so it
needs no providers, no network, and no cloud account.

## License

[MIT](LICENSE)
