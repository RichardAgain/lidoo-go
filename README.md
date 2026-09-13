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
