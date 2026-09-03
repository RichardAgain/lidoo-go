# Profile Database Operations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add profile-aware `init`, `update`, and `drop` commands that run click-odoo-contrib inside the selected Odoo container.

**Architecture:** Keep Go as the host-side lifecycle wrapper and Docker labels as profile discovery. Install `click-odoo-contrib==1.23.1` in each Odoo image, then use `docker exec` with a small shell adapter that exports the container's `HOST`, `PORT`, `POSTGRES_USER`, and `POSTGRES_PASSWORD` as `PGHOST`, `PGPORT`, `PGUSER`, and `PGPASSWORD`. Pass the database name explicitly so one profile can own many databases.

**Tech Stack:** Go 1.26, Docker CLI, Odoo 18 official image, Python `click-odoo-contrib==1.23.1`, `click-odoo` transitive dependency, PostgreSQL 17.

**Spec:** `docs/superpowers/specs/2026-09-02-database-operations-design.md`

## Global Constraints

- Keep the project as lightweight, fast, and clean as possible.
- Treat a profile as the runtime unit; one profile may expose multiple Odoo databases.
- Continue using `io.lidoo.name` for runtime profile discovery.
- Install click-odoo-contrib in the Docker image, never in the host environment.
- Do not add a resident process or a profile file for this database-command change.
- Require an explicit `--yes` before `drop` executes.
- Do not stop a profile automatically because it may serve multiple databases.

---

### Task 1: Add database operation validation and command construction tests

**Files:**
- Create: `internal/docker/database_test.go`
- Create: `internal/docker/database.go`

**Interfaces:**
- `validateDatabaseName(name string) error` accepts safe PostgreSQL/Odoo names up to 63 characters and rejects empty, whitespace, shell punctuation, and names beginning with punctuation.
- `databaseCommand(container string, operation string, database string, modules string, updateAll bool) []string` returns the exact `docker exec` argument list for the selected operation without embedding user input in a shell script.

- [x] **Step 1: Write failing tests**

Test valid names (`testing_db`, `demo-18`, `client.test`) and invalid names
(``, `with space`, `;drop`, `-demo`). Test that init includes
`click-odoo-initdb`, `--new-database` data, and modules; update includes
`click-odoo-update` and optionally `--update-all`; drop includes
`click-odoo-dropdb` and the database argument.

- [x] **Step 2: Run the focused tests and verify the expected failure**

Run:

```bash
go test ./internal/docker -run 'Test(DatabaseName|DatabaseCommand)' -v
```

Expected: compilation fails because the validation and command-builder
functions do not exist.

- [x] **Step 3: Implement the minimal pure helpers**

Add the database-name regexp and deterministic command builders. Use Docker
exec positional arguments for the profile and database values. Keep the
credential-to-Odoo-option adapter as fixed shell text with positional values,
not string interpolation.

- [x] **Step 4: Run the focused tests and verify they pass**

Run the same focused test command. Expected: all database helper tests pass.

### Task 2: Add profile-aware init, update, and drop commands

**Files:**
- Modify: `cmd/main.go`
- Modify: `internal/docker/common.go`
- Modify: `internal/docker/database.go`

**Interfaces:**
- `Init(args []string) error` parses `--name`, `--database`, and `--modules` (default `base`), validates the profile/database, and runs click-odoo-initdb inside the selected running container.
- `Update(args []string) error` parses `--name`, `--database`, and `--update-all`, then runs click-odoo-update inside the selected running container.
- `Drop(args []string) error` parses `--name`, `--database`, and `--yes`, refusing to call Docker without `--yes`, then runs click-odoo-dropdb inside the selected running container.
- `requireRunningProfile(name string) (string, error)` returns `lidoo-<name>` only when the label-discovered profile exists and is running.

- [x] **Step 1: Add tests for command safety**

Extend `database_test.go` with tests that missing `--name`, missing
`--database`, stopped/missing profile errors, and missing `--yes` are returned
before operation execution. Keep Docker execution behind one helper so the
pure validation remains testable.

- [x] **Step 2: Run the focused tests and verify they fail for missing commands**

Run:

```bash
go test ./internal/docker -run 'Test(Database|Profile)' -v
```

Expected: the new command tests fail to compile or report the missing command
implementation.

- [x] **Step 3: Implement the command wrappers**

Follow the existing flag/error patterns in `up`, `stop`, `restart`, and
`remove`. Use `docker exec <container> sh -c <fixed script> <database> [modules]`
where the fixed script exports `PGHOST`, `PGPORT`, `PGUSER`, and `PGPASSWORD`
from the container environment before invoking the selected click-odoo binary.
These commands do not accept Odoo's `--db_host`-style options.
Forward `--update-all` only when requested. Register `init`, `update`, and
`drop` in `cmd/main.go` and update usage text.

- [x] **Step 4: Run all Go tests**

Run `go test ./...`; expected result is all packages passing.

### Task 3: Install click-odoo-contrib in the Odoo image

**Files:**
- Modify: `docker/Dockerfile.18`

**Interfaces:**
- New Odoo 18 images contain `click-odoo-initdb`, `click-odoo-update`, and
  `click-odoo-dropdb` on `PATH`.

- [x] **Step 1: Update the Dockerfile**

Add the installation as root during image build and restore the Odoo runtime
user afterward:

```dockerfile
USER root
RUN pip3 install --no-cache-dir --break-system-packages click-odoo-contrib==1.23.1
USER odoo
```

Keep the package pinned and avoid leaving pip's download cache in the image.

- [x] **Step 2: Build and smoke-test the image**

Run:

```bash
docker build --file docker/Dockerfile.18 --tag lidoo-odoo:18 .
docker run --rm --entrypoint sh lidoo-odoo:18 -c 'command -v click-odoo-initdb && command -v click-odoo-update && command -v click-odoo-dropdb'
```

Expected: the image builds and all three command paths are printed.

### Task 4: Document the complete database workflow

**Files:**
- Modify: `README.md`
- Modify: `docs/superpowers/specs/2026-09-02-database-operations-design.md`

**Interfaces:**
- README explains image rebuild, profile/database distinction, all commands,
  module selection, drop confirmation, stopped-profile behavior, and current
  addon-path limitation.

- [x] **Step 1: Add documented examples**

Document:

```bash
docker compose up -d
go run ./cmd up --name testing --version 18
go run ./cmd init --name testing --database testing_db --modules base,sale
go run ./cmd update --name testing --database testing_db
go run ./cmd drop --name testing --database testing_db --yes
```

Explain that `--name` selects the profile and `--database` selects one DB
inside it; `drop` is irreversible and custom add-ons must already be visible
inside the container's configured addons path.

- [x] **Step 2: Run documentation consistency checks**

Run `git diff --check` and search for every command name in README and the
design spec. Expected: no whitespace errors and no undocumented operation.

### Task 5: Verify runtime behavior

**Files:**
- No source changes.

- [x] **Step 1: Rebuild the profile image and recreate only disposable profiles**

Run the image build and recreate an empty test profile so the new package is
present. Do not remove a populated legacy profile without a backup.

- [x] **Step 2: Verify command help inside the container**

Run `docker exec lidoo-testing click-odoo-initdb --help`,
`docker exec lidoo-testing click-odoo-update --help`, and
`docker exec lidoo-testing click-odoo-dropdb --help`; each must exit zero and
show its expected command.

- [x] **Step 3: Verify Go quality gates**

### Task 6: Clean builder and database operation logs

**Files:**
- Modify: `docker/Dockerfile.18`
- Modify: `internal/docker/common.go`
- Modify: `internal/docker/up.go`
- Modify: `internal/docker/database.go`
- Modify: `internal/docker/remove.go`
- Modify: `internal/docker/stop.go`
- Modify: `internal/docker/restart.go`
- Create: `internal/docker/log_test.go`

- [x] Use Buildx when available and retain a compatible fallback for Docker
  installations without the plugin.
- [x] Suppress successful Docker IDs and builder output while preserving
  failure diagnostics.
- [x] Resolve the Odoo 18 Courier font lookup with the bundled Nimbus Mono
  font instead of adding an OS font dependency.
- [x] Filter only known non-fatal click-odoo/Odoo noise, strip ANSI colors,
  and keep actual errors visible.
- [x] Verify with a disposable profile running `init`, `update`,
  `update --all`, `drop`, and `remove`.

Run `go test ./...`, `go vet ./...`, `go build ./...`, `docker compose config`,
and `git diff --check`.
