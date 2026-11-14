# DBM — Database Management CLI

A CLI for managing throwaway development databases as Docker containers.
Supports MySQL and PostgreSQL with a single consistent interface.

```
dbm create mydb --provider postgres   # container + volume + ready in seconds
dbm connect mydb                      # connection string when you need it
dbm remove mydb                       # everything gone, volumes included
```

## Why

Spinning up a database for local development means remembering image names,
port mappings, environment variables, volume flags, and the right `docker run`
incantation for each engine. DBM does it in one command and keeps track of what
it created: containers, named volumes, ports, and credentials live in
`~/.dbm/databases/<name>.json`, so `dbm` commands work by name instead of
container ID.

## Features

- **One-command create** — pulls the image, creates a named volume, starts the
  container, waits for real protocol-level readiness, and provisions a dev user.
- **MySQL and PostgreSQL** — provider abstraction handles engine differences
  (images, ports, env vars, user grants, connection strings).
- **Interactive or scripted** — run `dbm create` with no arguments for guided
  setup, or pass flags for automation. Passwords are generated with `crypto/rand`.
- **Lifecycle by name** — `start`, `stop`, `remove`, `logs`, `status`.
- **Groups** — organize databases per project or organization. A database is
  `global` by default; assign one at creation with `-g work`, then filter
  views with `dbm list -g work`.
- **SQL access** — `dbm exec --sql "..."` with table output, plus
  `dbm table list` / `dbm table describe`.
- **Backup and restore** — `mysqldump`/`pg_dump` runs inside the container; the
  dump is written to a local file. Restore streams the file through stdin, so
  large dumps work.
- **Connection strings** — `dbm connect <name> [--format string|json|env]`.

## Installation

Requires Go 1.25+ and a running Docker-compatible daemon (Docker, Podman with
the `docker` alias, etc.).

```sh
git clone https://github.com/iqbalmu/dbm
cd dbm
make install          # builds and copies to /usr/local/bin
```

Or without make:

```sh
go build -o dbm ./cmd/dbm && sudo mv dbm /usr/local/bin/
```

## Quick start

```sh
# Guided setup
dbm create

# Scripted: postgres on a custom port
dbm create appdb --provider postgres --port 55432

# Grab credentials / connection string
dbm connect appdb
dbm connect appdb --format env      # DB_HOST=... DB_PORT=... for .env files
dbm connect appdb --format json     # for scripts

# Inspect and query
dbm table list appdb
dbm table describe appdb users
dbm exec appdb --sql "SELECT version();"

# Backup, restore
dbm backup appdb -o ./dump.sql
dbm restore appdb ./dump.sql
```

## Commands

| Command | Description |
|---|---|
| `dbm create [name] [-p provider] [-v version] [--port N] [-g group] [-i] [--volume=true]` | Create a database (interactive when no name given); `-g` assigns a group, default is global |
| `dbm list [-g group\|global]` | List all managed databases, optionally filtered by group |
| `dbm status [-g group\|global]` | Live status of all containers, optionally filtered by group |
| `dbm start <name>` | Start a stopped database |
| `dbm stop <name>` | Stop a running database |
| `dbm remove <name> [-f] [--keep-data]` | Remove container and data volume (`--keep-data` keeps the volume) |
| `dbm info <name>` | Detailed database information |
| `dbm connect <name> [--format string\|json\|env]` | Show connection information |
| `dbm exec <name> --sql "..." / --file f.sql` | Execute SQL with table output |
| `dbm logs <name> [-n N] [-f]` | Container logs |
| `dbm backup <name> [-o file]` | Dump the database to a local `.sql` file |
| `dbm restore <name> <file> [-f]` | Restore from a `.sql` file (prompts before overwriting) |
| `dbm table list <name>` / `dbm table describe <name> <table>` | Inspect tables |
| `dbm provider list` | Show supported providers and defaults |
| `dbm config init/show/path` | Manage `~/.dbm/config.yaml` |
| `dbm version` | Version information |

## Design notes

- **State**: one JSON file per database in `~/.dbm/databases/`, written with
  `0600` permissions (credentials included).
- **Ports bind to `127.0.0.1` only** — your dev database is not reachable from
  the network. Create refuses ports already used by another managed database or
  any local process.
- **Named volumes** (`dbm_<name>_data`) survive container restarts; `dbm remove`
  deletes them unless `--keep-data` is passed. Creating a database with the same
  name as an existing volume reuses it (you'll see a warning).
- **Readiness**: `dbm create` waits until the server completes a real protocol
  handshake before provisioning users — no fixed sleeps, so it works on slow
  machines and rootless container runtimes alike.
- **User provisioning**: the dev user (`dev_user`) gets full privileges on its
  database. On PostgreSQL 15+ the user is made database owner, which restores
  `CREATE` on schema `public` (revoked for non-owners since PG15).
- **SQL identifiers and literals** supplied by users are escaped before being
  interpolated into DDL.

## Development

```sh
make build    # build to bin/
make test     # go test ./...
make fmt      # go fmt
go vet ./...
```

## License

[MIT](LICENSE)
