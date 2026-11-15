# xm-db: Database Management CLI

A CLI for managing throwaway development databases as Docker containers.
Supports MySQL and PostgreSQL with a single consistent interface.

```
xmdb create mydb --provider postgres   # container + volume + ready in seconds
xmdb connect mydb                      # connection string when you need it
xmdb remove mydb                       # everything gone, volumes included
```

## Why

Spinning up a database for local development means remembering image names,
port mappings, environment variables, volume flags, and the right `docker run`
incantation for each engine. xm-db does it in one command and keeps track of what
it created: containers, named volumes, ports, and credentials live in
`~/.xmdb/databases/<name>.json`, so `xmdb` commands work by name instead of
container ID.

## Features

- Interactive wizard: run `xmdb` with no arguments for a menu to pick an
  action and a database; the action runs once and the program exits.
- One-command create: pulls the image, creates a named volume, starts the
  container, waits for real protocol-level readiness, and provisions a dev user.
- MySQL and PostgreSQL: provider abstraction handles engine differences
  (images, ports, env vars, user grants, connection strings).
- Interactive or scripted: run `xmdb create` with no arguments for guided
  setup, or pass flags for automation. Passwords are generated with `crypto/rand`.
  The guided flow lists the ports already in use, suggests the next free one,
  offers the known Docker tags per engine (with manual entry for anything
  else), and every prompt backs out with Esc.
- Lifecycle by name: `start`, `stop`, `remove`, `logs`, `status`.
- Groups: organize databases per project or organization. A database is
  `global` by default; assign one at creation with `-g work`, then filter
  views with `xmdb list -g work`.
- SQL access: `xmdb exec --sql "..."` with table output, plus
  `xmdb table list` and `xmdb table describe`. `xmdb exec --file <script>`
  streams a multi-statement script through the engine's own client inside the
  container, which supports them on both engines.
- Backup and restore: `mysqldump`/`pg_dump` runs inside the container and
  streams the dump straight to a local file; restore streams the file back
  through stdin. Either direction handles dumps far larger than memory.
- Connection strings: `xmdb connect <name> [--format string|json|env]`.

## Installation

Requires a Docker-compatible daemon (Docker, Podman with the `docker` alias,
etc.).

### One-liner (curl)

```sh
curl -fsSL https://raw.githubusercontent.com/iqbalmu/xm-db/main/install.sh | sh
```

Detects your OS/arch, downloads the matching release binary, verifies its
sha256 checksum, and installs it. Defaults to `/usr/local/bin` (falls back to
`~/.local/bin` without sudo access); both are overridable:

```sh
XMDB_INSTALL_DIR=~/.local/bin VERSION=v1.0.0 sh install.sh
```

Re-running the installer (or running `xmdb update`) upgrades to the latest release.

### Prebuilt binaries

Grab a binary for your platform from
[GitHub Releases](https://github.com/iqbalmu/xm-db/releases), unpack it, and put
`xmdb` somewhere in your `PATH`:

```sh
tar -xzf xm-db_*_linux_amd64.tar.gz
sudo install ./xmdb /usr/local/bin/
```

### go install

```sh
go install github.com/iqbalmu/xm-db/cmd/xmdb@latest
```

The binary lands in `$(go env GOPATH)/bin` (usually `~/go/bin`), so make sure
that directory is on your `PATH`.

### From source

Requires Go 1.25+:

```sh
git clone https://github.com/iqbalmu/xm-db
cd xmdb
make install          # builds and copies to /usr/local/bin
```

Or without make:

```sh
go build -o xmdb ./cmd/xmdb && sudo mv xmdb /usr/local/bin/
```

## Quick start

```sh
# Guided setup
xmdb create

# Scripted: postgres on a custom port
xmdb create appdb --provider postgres --port 55432

# Grab credentials / connection string
xmdb connect appdb
xmdb connect appdb --format env      # DB_HOST=... DB_PORT=... for .env files
xmdb connect appdb --format json     # for scripts

# Inspect and query
xmdb table list appdb
xmdb table describe appdb users
xmdb exec appdb --sql "SELECT version();"

# Backup, restore
xmdb backup appdb -o ./dump.sql
xmdb restore appdb ./dump.sql
```

## Commands

| Command | Description |
|---|---|
| `xmdb` | Interactive wizard menu: pick an action, pick a database |
| `xmdb create [name] [-p provider] [-v version] [--port N] [-g group] [-i] [--volume=true]` | Create a database (interactive when no name given); `-g` assigns a group, default is global |
| `xmdb list [-g group\|global]` | List all managed databases, optionally filtered by group |
| `xmdb status [-g group\|global]` | Live status of all containers, optionally filtered by group |
| `xmdb start <name>` | Start a stopped database |
| `xmdb stop <name>` | Stop a running database |
| `xmdb remove <name> [-f] [--keep-data]` | Remove container and data volume (`--keep-data` keeps the volume) |
| `xmdb info <name>` | Detailed database information |
| `xmdb connect <name> [--format string\|json\|env]` | Show connection information |
| `xmdb exec <name> --sql "..." / --file f.sql` | Execute SQL with table output |
| `xmdb logs <name> [-n N] [-f]` | Container logs |
| `xmdb backup <name> [-o file]` | Dump the database to a local `.sql` file |
| `xmdb restore <name> <file> [-f]` | Restore from a `.sql` file (prompts before overwriting) |
| `xmdb table list <name>` / `xmdb table describe <name> <table>` | Inspect tables |
| `xmdb provider list` | Show supported providers and defaults |
| `xmdb config init/show/path` | Manage `~/.xmdb/config.yaml` |
| `xmdb update` | Update `xmdb` to the latest release |
| `xmdb version` | Version information |

## Design notes

- Works with Docker and Podman: on every connect `xmdb` probes candidate
  daemons in order (`$DOCKER_HOST`, `/var/run/docker.sock`, then the rootless
  and rootful Podman sockets; on Windows, the Docker Desktop and podman named
  pipes). The first one that answers wins; pin a side with
  `--runtime docker|podman`.
- State: one JSON file per database in `~/.xmdb/databases/`, written with
  `0600` permissions (credentials included).
- Ports bind to `127.0.0.1` only, so your dev database is not reachable from
  the network. Create refuses ports already used by another managed database or
  any local process.
- Named volumes (`xmdb_<name>_data`) survive container restarts; `xmdb remove`
  deletes them unless `--keep-data` is passed. Creating a database when a
  volume with that name still exists fails fast with instructions, because
  container images skip init on an existing volume (new credentials would not
  apply).
- Readiness: `xmdb create` waits until the server completes a real protocol
  handshake before provisioning users (no fixed sleeps), so it works on slow
  machines and rootless container runtimes alike.
- User provisioning: the dev user (`dev_user`) gets full privileges on its
  database. On PostgreSQL 15+ the user is made database owner, which restores
  `CREATE` on schema `public` (revoked for non-owners since PG15).
- SQL identifiers and literals supplied by users are escaped before being
  interpolated into DDL.

## Development

```sh
make build    # build to bin/
make test     # go test ./...
make fmt      # go fmt
go vet ./...
```

### Releasing

Releases are cut by pushing a semver tag; GitHub Actions builds binaries for
linux/darwin/windows (amd64 + arm64) and publishes them to GitHub Releases.

```sh
git tag -a v1.0.1 -m "v1.0.1"
git push origin v1.0.1
```

Test a release build locally without uploading:

```sh
make release-snapshot
```

## License

[MIT](LICENSE)
