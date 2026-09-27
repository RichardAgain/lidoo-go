# lidoo

Create the shared database configuration before starting the services:

```sh
cp .env.example .env
docker compose up -d db caddy
go run ./cmd run --name testing --version 18
```

For the developer-friendly `lidoo` command, add the repository wrapper to your
shell path once per checkout:

```sh
export PATH="$(pwd)/bin:$PATH"
lidoo run --name testing --version 18
lidoo backup --name testing --database testing_db
```

The wrapper changes to the repository root before invoking `go run ./cmd`, so
it works even when called from another directory. It is intentionally a
development wrapper and requires Go to be installed.

To list all profiles discovered through Docker labels, including stopped
profiles:

```sh
go run ./cmd list
```

Lidoo generates the shared Caddy configuration for each profile and routes
requests through `lidoo-net` to the container's internal port `8069`. The
CLI generates routes such as `testing.lidoo.localhost`. The `.localhost`
domain resolves to the local machine without DNS or hosts-file configuration.
Then open:

```text
http://testing.lidoo.localhost
```

## Profile configuration

Profile configuration is persisted in `.lidoo.json`, which is the canonical
input for future container creation and recreation. Configure a profile
before its first run, without starting or replacing a container:

```sh
go run ./cmd config --name testing \
  --db-filter-mode profile \
  --admin-passwd 'local-master-password'
```

The default database filter mode is `profile`, including for older state
entries that have no mode; it restricts databases to the escaped profile
prefix (`testing__...`). `disabled` omits Odoo's `--db-filter` and allows
every database visible to that profile, so use it only when that sharing is
intentional. `custom` passes the exact regular expression supplied with
`--db-filter-pattern`; it is powerful and can expose databases if the
expression is too broad:

```sh
go run ./cmd config --name testing \
  --db-filter-mode custom \
  --db-filter-pattern '^testing__(dev|staging)$'
go run ./cmd config --name testing --db-filter-mode disabled
```

`--admin-passwd` is Odoo's master password for database-management actions,
not PostgreSQL's `POSTGRES_PASSWORD` and not the per-database Odoo admin
user's password. The master password is intentionally stored as plaintext
in this ignored local-development state. When configured, Lidoo generates
`.lidoo/<profile>/odoo.conf` with mode `0644` in a mode `0755` directory and
mounts it read-only at Odoo's conventional `/etc/odoo/odoo.conf`. These
permissions are required because the official Odoo entrypoint runs as the
`odoo` user, reads the file through `ODOO_RC` before startup, and needs to
parse its `[options]` section. The generated file is only a runtime artifact.
An empty `--admin-passwd ''` leaves no configured master password for a new
container; an existing runtime file is rewritten without the password so a
stopped container using its old mount can still start. Lidoo does not use a
manually maintained `odoo.conf` as the source of truth.

Changing state does not silently recreate or restart an existing container.
Run `recreate --name <profile>` to apply all persisted creation settings to
an existing profile. Starting a stopped managed container refreshes its
generated config when that container already has the runtime mount; an old
container without that mount must be recreated explicitly.

Caddy must be running before a profile is created or removed. Lidoo validates
and reloads Caddy after each route change; it does not give Caddy access to
the Docker socket. Lidoo does not modify `/etc/hosts`, configure `dnsmasq`, or
implement custom DNS handling.

Profile builds use Docker BuildKit/Buildx when available. On Docker
installations that do not include the Buildx plugin, Lidoo falls back to the
compatible builder and keeps its diagnostic output hidden on successful
builds; build failures still print the complete Docker output.

## Database operations

The Odoo image includes `click-odoo-contrib==1.23.1`, which provides the
database maintenance commands used by Lidoo. Rebuild/recreate the profile
after changing the Dockerfile so the new tools are available. `run` starts an
existing container; it does not replace that container when its image tag has
been rebuilt.

If a database command reports that `click-odoo-contrib` is missing, the
profile was created from an older image. For a disposable profile, recreate
the container and run the operation again:

```sh
go run ./cmd remove --name testing --yes
go run ./cmd run --name testing --version 18
go run ./cmd init --name testing --database testing_db --modules base,sale
```

`remove` does not delete the PostgreSQL Compose volume or the named
`lidoo-filestore-data` volume. For a populated profile, back up its databases,
filestore, and add-ons before destructive work; `remove` deletes the container
but does not delete those volumes.

`--name` always selects the profile/container. `--database` selects one Odoo
database inside that profile; a profile can contain multiple databases. Database
names are qualified with the profile's configured prefix, so logical names stay
separate from physical PostgreSQL names.

Discover and inspect databases through the running profile:

```sh
go run ./cmd db list --name testing
go run ./cmd db info --name testing --database testing_db
go run ./cmd db shell --name testing --database testing_db
LIDOO_ADMIN_PASSWORD='new-admin-password' \
  go run ./cmd db set-password --name testing --database testing_db
```

`db list` shows the databases allowed by the profile's filter mode (all of them
when the mode is `disabled`, the matching pattern when it is `custom`);
click-odoo's internal `cache-*` template databases are hidden. `db shell` is an
interactive `psql` session; its terminal and exit status are passed through.
`db set-password` changes the built-in `admin` user's login password through
Odoo; it takes the password from `LIDOO_ADMIN_PASSWORD` or prompts on a terminal
without echoing, so the secret is never an argument.

Initialize a database with the requested comma-separated modules:

```sh
go run ./cmd init \
  --name testing \
  --database testing_db \
  --modules base,sale
```

Update a database using click-odoo's changed-addon detection, or force a full
update with `--update-all`:

```sh
go run ./cmd update --name testing --database testing_db
go run ./cmd update --name testing --database testing_db --update-all
```

Drop a database and its Odoo filestore. This operation is irreversible and
requires explicit confirmation:

```sh
go run ./cmd drop --name testing --database testing_db --yes
```

Create a backup or restore one using the corresponding `click-odoo-contrib`
options. The source and destination paths are local paths; Lidoo copies them
through Docker because profile containers do not mount the project directory:

```sh
# Saves to backups/testing_db_<yymmdd>_<unix-seconds>.zip
go run ./cmd backup \
  --name testing \
  --database testing_db \
  --format zip \
  --filestore

# A path can override the default destination
go run ./cmd backup \
  --name testing \
  --database testing_db \
  ./backups/testing.zip

go run ./cmd restore \
  --name testing \
  --destination testing_db \
  --force \
  --neutralize \
  ./backups/testing.zip
```

When no destination is supplied, backups are saved under `backups/` using
the name `<db_name>_%y%m%d_%s.zip`. The `backups/` directory must exist. A
final positional path overrides this default. Non-zip formats require an
explicit destination path. `backup` supports `--force`, `--if-exists`,
`--format zip|dump|folder`, and `--filestore`/`--no-filestore`.
`restore` supports `--copy`/`--move`,
`--force`, `--neutralize`, and `--jobs N`. The target database comes from
`--destination` (preferred) or `--database`; when neither is given it is
derived from the dump file name and made unique against existing databases, so
a restore never silently replaces an existing database.

The selected profile must be running. These commands execute
`click-odoo-initdb`, `click-odoo-update`, `click-odoo-dropdb`,
`click-odoo-backupdb`, or `click-odoo-restoredb` inside the profile container,
using its Odoo version, addons path, and PostgreSQL connection. PostgreSQL
credentials are always read from the profile environment
(`POSTGRES_USER` and `POSTGRES_PASSWORD`); Lidoo has no hardcoded database
password fallback. Add-ons are registered before they can be attached. Attach
and detach use the registered checkout or worktree path and report when a
running profile needs recreation:

```sh
go run ./cmd addons add server-tools https://github.com/OCA/server-tools.git
go run ./cmd addons attach --name testing server-tools
go run ./cmd addons attach --name testing server-tools --recreate
go run ./cmd addons detach --name testing server-tools --recreate
```

Use `--recreate` only when replacing the container is acceptable. Without it,
state changes remain pending until the profile is explicitly recreated. Unknown,
missing, non-directory, and duplicate mount paths are rejected before Docker
or state changes.

Database operation logs are intentionally concise. Lidoo removes only the
known non-fatal ReportLab and Odoo 18 `mail` description warnings; actual
command errors and database failures remain visible.

## Runtime operations

Inspect state-only profiles and Docker containers without changing them:

```sh
go run ./cmd status
go run ./cmd status --name testing
go run ./cmd logs --name testing --tail 100
go run ./cmd logs --name testing --follow
```

Wait is opt-in. It checks the container, PostgreSQL, and the Odoo HTTP route;
plain `run` remains non-blocking:

```sh
go run ./cmd run --name testing --version 18
go run ./cmd run --name testing --wait --timeout 2m
go run ./cmd wait --name testing --timeout 30s
```

If startup exits or times out, inspect `lidoo logs --name testing`.

## Interactive TUI

`lidoo tui` (or `go run ./cmd tui`) opens the dashboard. Focus moves with
`tab`/`←`/`→`; profiles, databases, and add-ons are separate panels.

Profiles:

- `n` (or `c`) creates a profile: choose an Odoo version from a select (the
  known versions 17/18/19 plus any `docker/Dockerfile.*`; rows are tagged
  `dockerfile`/`no dockerfile` and `image built`) or type a custom one, then name
  it. The profile is added to workspace state with that version and is **not**
  started; start it later with `space`.
- `e` opens a settings panel. It lists one row per setting and only the field
  you edit is persisted, so changing the database filter mode does not touch the
  master password and vice versa:
  - **database filter mode** — a select for `profile`, `disabled`, or `custom`.
  - **filter pattern** — shown only in `custom` mode (the regular expression is
    matched against physical database names).
  - **Odoo master password** — `admin_passwd` (`/web/database`), not the admin
    user's login password.
  Use `↑`/`↓` to pick a row, `enter` to edit, `esc` to close. The Databases
  panel then lists exactly the databases that mode exposes.
- `space` toggles run/stop for the selected profile; `x` starts, `s` stops,
  `r` restarts, `R` recreates, `enter` opens the URL, `d` removes. Actions that
  do not apply (starting a running profile, stopping a stopped one) report a
  notice instead of failing. Starting a profile waits until Odoo answers before
  the task reports `completed` (cancel with `c`).
- Destructive actions (remove profile, drop database, move/overwrite restore)
  ask you to type the resource name before they run.

Databases (the list follows the profile's database filter mode; click-odoo's
internal `cache-*` template databases are hidden):

- `a`/`A` changes the built-in `admin` user's password for the selected
  database. It runs `odoo shell` inside the profile (the profile must be
  running) and pipes the script on standard input, so the password never
  appears in a process argument list. This is the user login password, **not**
  `admin_passwd`. The same operation is available as
  `lidoo db set-password --name <profile> --database <db>`, taking the password
  from `LIDOO_ADMIN_PASSWORD` or an interactive prompt.
- `R` restores into a database. It opens a file picker that starts in the
  working directory: `↑`/`↓` move, `enter` enters a folder or picks a
  `.zip`/`.dump`, and `h`/`backspace` goes up. Any dump on your machine is
  reachable this way, and the restore form still accepts a typed path. The
  destination defaults to the dump's file name (made unique against existing
  databases), so a restore creates a new database instead of replacing the one
  you had selected; edit `destination` and switch `mode` to `move` to replace an
  existing database.

Add-ons:

- `c` clones a new checkout.
- `w` creates a worktree: choose the source add-on and then a branch from a
  select (with a "New branch…" option).
- `a` attaches and `d` detaches add-ons for the selected profile, `f` fetches,
  `p` pulls, and `x` removes a checkout chosen from a select.

Logs:

- The profile panel shows only the container stream inline; Odoo lines are
  compacted (level gutter, no date/pid, werkzeug access lines summarised with the
  database name).
- Task output is never mixed into that panel. The `Latest task` block below the
  panel shows a short tail, and the full task stream lives in the viewer's
  `tasks` source.
- The inline panel can be scoped to one database. Press `D` in the Profiles panel
  (or `d` in the viewer) to get a select with `all databases` plus every database
  of the profile and every database seen in the log lines. Opening logs from the
  Databases panel with `L` pre-selects the database there; the panel shows the
  active `db filter`.
- `L` opens the fullscreen viewer from any panel. `tab` cycles
  `container` / `tasks` / `all`, `d` opens the database select (`all` or one
  database), `1`–`4` filter by minimum level (`all`, `info+`, `warn+`, `error`),
  `/` searches, `f` toggles follow, and `g`/`G` jump to top/bottom. Mouse wheel
  and `pgup`/`pgdn` scroll. Each profile keeps its own log buffer; profiles never
  share a stream.
- Mouse: the wheel scrolls logs, clicking a profile selects it, and clicking the
  tab bar (`Databases` / `Add-ons` / `Info`) switches panels.

Changes that need a container rebuild stay pending until `R` recreates the
profile, or the recreate option is chosen in the attach/detach dialog.

## Add-on maintenance

`addons list` shows registered paths and attached profiles. `addons status`
reports missing paths, dirty repositories, and unavailable worktree parents:

```sh
go run ./cmd addons list
go run ./cmd addons status [<addon name>]
go run ./cmd addons fetch server-tools
go run ./cmd addons pull server-tools
```

Fetch is non-destructive. Pull is allowed only for a clean normal clone with a
current branch and configured upstream, and uses fast-forward-only behavior.
Worktrees and dirty or ambiguous repositories are refused. Source edits are
bind-mounted and do not require recreation; mount identity changes do.

## Portable profile configuration

Export contains only the independent format version, profile name, Odoo
version, database prefix, and registered add-on names. It contains no
passwords, runtime IDs, or Docker details:

```sh
go run ./cmd profile export --name testing testing-profile.json
go run ./cmd profile import testing-profile.json
# use --yes only when intentionally replacing an existing profile
go run ./cmd profile import testing-profile.json --yes
```

Import validates profile names, prefixes, and registered add-on paths, writes
workspace state only, and never creates a container. Run the profile explicitly
after import.

## Backup, migration, purge, and rollback

Before destructive work, back up every database in the profile and preserve
its filestore and add-on sources. `drop` deletes the selected database and
filestore and requires `--yes`; `remove` deletes only the profile container.
Neither command deletes Compose PostgreSQL data or the named filestore volume.
If a recreation fails, restore the prior `.lidoo.json` from your backup and
inspect the Docker logs before retrying. Database restore is explicit and can
use `--copy` (default) or `--move`; never use `--move` as a rollback substitute.

`.lidoo.json` is created on the first successful state mutation, saved with a
same-directory temporary file, fsynced, and atomically renamed. Mutating
commands hold a process lock; a concurrent mutation fails instead of
overwriting state. Unknown top-level state sections are preserved. Keep
`.lidoo.json`, `.lidoo.json.lock`, backups, the PostgreSQL Compose volume, and
`lidoo-filestore-data` out of source control.

Caddy is an operational prerequisite for profile creation, removal, and
recreation because Lidoo validates and reloads the shared generated
`docker/caddy/Caddyfile`. Caddy has no Docker socket access. Profile hostnames
use `<profile>.lidoo.localhost`; Lidoo does not edit hosts files or configure
DNS.

## Testing

These checks are for local development only. Use a disposable profile and
database; `recreate` replaces the profile container, and the cleanup
commands below are destructive.

Run the focused checks:

```sh
gofmt -l .        # must print nothing
go vet ./...
go test ./...
git diff --check
```

For a minimal Docker smoke test, start the local dependencies first:

```sh
cp .env.example .env
docker compose up -d db caddy
export LIDOO_TEST_PASSWORD='local-only-master-password'
```

Configure state without starting Docker, then create the profile:

```sh
go run ./cmd config --name testing \
  --db-filter-mode profile \
  --admin-passwd "$LIDOO_TEST_PASSWORD"
go run ./cmd run --name testing --version 18
```

Inspect only the generated command and mount metadata. These commands show
the database filter and confirm the read-only `odoo.conf` mount without
printing the password or reading the file contents:

```sh
docker inspect lidoo-testing --format '{{json .Config.Cmd}}' | grep -F -- '--db-filter'
docker inspect lidoo-testing --format '{{range .Mounts}}{{println .Destination .RW}}{{end}}' \\
  | grep -F '/etc/odoo/odoo.conf false'
```

Change the filter to a custom expression and recreate the profile:

```sh
go run ./cmd config --name testing \
  --db-filter-mode custom \
  --db-filter-pattern '^testing__(dev|staging)$'
# Destructive for the disposable profile: replaces its container.
go run ./cmd recreate --name testing
docker inspect lidoo-testing --format '{{json .Config.Cmd}}' | grep -F -- '--db-filter'
```

Verify the stopped-container lifecycle. `run` reuses the stored Odoo
version when `--version` is omitted:

```sh
go run ./cmd stop --name testing
go run ./cmd run --name testing
```

Finally, test disabled mode and confirm that no database filter is passed:

```sh
go run ./cmd config --name testing --db-filter-mode disabled
# Destructive for the disposable profile: replaces its container.
go run ./cmd recreate --name testing
if docker inspect lidoo-testing --format '{{json .Config.Cmd}}' | grep -Fq -- '--db-filter'; then
  echo 'unexpected --db-filter in disabled mode' >&2
  exit 1
fi
```

To clean up the disposable profile and local stack:

```sh
# Destructive: removes the profile container and its managed state entry.
go run ./cmd remove --name testing --yes
# Destructive: removes Compose volumes, including local PostgreSQL data.
docker compose down -v
```
