# lidoo

Create the shared database configuration before starting the services:

```sh
cp .env.example .env
docker compose up -d db caddy
go run ./cmd run --name testing --version 18
```

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
```

`db list` only shows databases allowed by the profile prefix. `db shell` is an
interactive `psql` session; its terminal and exit status are passed through.

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
  --database testing_db \
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
`--force`, `--neutralize`, and `--jobs N`.

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
