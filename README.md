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

`remove` does not delete the PostgreSQL Compose volume, but it leaves the
profile's anonymous Odoo volumes detached. For a populated profile, back up
the filestore and add-ons first and preserve or reattach those volumes during
the recreation.

`--name` always selects the profile/container. `--database` selects one Odoo
database inside that profile; a profile can contain multiple databases.

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

The selected profile must be running. These commands execute
`click-odoo-initdb`, `click-odoo-update`, or `click-odoo-dropdb` inside the
profile container, using its Odoo version, addons path, and PostgreSQL
connection. PostgreSQL credentials are always read from the profile
environment (`POSTGRES_USER` and `POSTGRES_PASSWORD`); Lidoo has no hardcoded
database password fallback. Custom add-ons must already be mounted or otherwise available at
the addons path configured inside the container; persistent per-profile
add-on mounts are not configured by this version.

Database operation logs are intentionally concise. Lidoo removes only the
known non-fatal ReportLab and Odoo 18 `mail` description warnings; actual
command errors and database failures remain visible.

Before doing this with a populated profile, back up its Odoo filestore and
add-ons volumes. The current CLI does not migrate those volumes automatically;
removing the container leaves the old anonymous volumes detached. The
PostgreSQL data volume is managed separately by Compose.

## Testing

These checks are for local development only. Use a disposable profile and
database; `recreate` replaces the profile container, and the cleanup
commands below are destructive.

Run the focused CLI tests and whitespace check:

```sh
go test ./cmd
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
