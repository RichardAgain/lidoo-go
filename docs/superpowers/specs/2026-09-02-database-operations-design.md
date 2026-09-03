# Profile Database Operations Design

## Goal

Add profile-aware commands for initializing, updating, and dropping individual
Odoo databases through `click-odoo-contrib`, while preserving the rule that a
profile may contain multiple databases.

## Design

The Odoo image will install the pinned Python package
`click-odoo-contrib==1.23.1`. This package provides the stable
`click-odoo-initdb`, `click-odoo-update`, and `click-odoo-dropdb` commands and
uses `click-odoo` as its Odoo scripting layer. The package is installed at
image-build time, never on the host and never at container startup.

Lidoo will expose these top-level commands:

```text
lidoo init   --name <profile> --database <database> [--modules <csv>]
lidoo update --name <profile> --database <database> [--update-all]
lidoo drop   --name <profile> --database <database> --yes
```

`--name` selects the running Odoo container through the existing
`io.lidoo.name` label. `--database` is the actual PostgreSQL/Odoo database
name and is never used to derive the profile hostname. Each command executes
the corresponding click-odoo-contrib binary through `docker exec` in the
selected container, so it uses the same Odoo version, addons path, Python
environment, and database credentials as that profile.

The wrapper exports the container's `HOST`, `PORT`, `POSTGRES_USER`, and
`POSTGRES_PASSWORD` environment values as the standard PostgreSQL variables
`PGHOST`, `PGPORT`, `PGUSER`, and `PGPASSWORD`, which are consumed by Odoo's
PostgreSQL driver.
The profile must provide its PostgreSQL user and password through the
environment; no database credential is hardcoded in the CLI or image.
Database names and profile names are validated before Docker is called. The wrapper rejects a
missing or stopped profile, rejects malformed database names, and requires
`--yes` for destructive drops. It does not stop the profile automatically,
because stopping one profile would interrupt every database served by it.

The first implementation accepts the module list through `--modules` and uses
the addons path already present in the Odoo image/configuration. Persistent
per-profile addon mounts and addon configuration remain a separate change; no
profile file is added solely for these database commands.

Profile builds use `docker buildx build --load` when the Docker Buildx plugin
is available. Older Docker installations use the compatible `docker build`
fallback; successful builder output is suppressed while build failures remain
visible. Lifecycle commands also suppress Docker IDs and expose concise
profile status messages.

Database commands use a streaming output filter that removes only the known
non-fatal Odoo 18 ReportLab Courier warning, the malformed `mail` module RST
messages, ANSI color codes, and redundant `click_odoo_contrib` info lines.
Actual command errors and database failures are preserved. New Odoo images
link ReportLab's bundled Nimbus Mono font to the Courier Type 1 filename
expected by Odoo, avoiding an additional font package.

## Runtime flow

```text
Lidoo CLI
    -> Docker label io.lidoo.name=testing
    -> docker exec lidoo-testing
    -> click-odoo-initdb/update/dropdb
    -> PostgreSQL database selected by --database
```

## Safety and failure behavior

- `init` keeps click-odoo-initdb's default refusal to overwrite an existing
  database.
- `update` delegates module change detection to click-odoo-update; `--update-all`
  is forwarded when explicitly requested.
- `drop` never runs without `--yes` and only passes the selected database name
  to click-odoo-dropdb.
- The commands do not modify hosts entries, Traefik labels, containers, or
  other databases.
- A failed click-odoo command returns its exit status and output to the user.
- A profile created from an old image reports that it must be rebuilt and
  recreated before database tools can run.
- Database operation output is concise without hiding real errors.

## Acceptance criteria

- Rebuilding `docker/Dockerfile.18` makes all three click-odoo-contrib binaries
  available inside new Odoo profile containers.
- `init --name testing --database testing_db --modules base,sale` invokes
  `click-odoo-initdb` inside `lidoo-testing`.
- `update --name testing --database testing_db` invokes
  `click-odoo-update` inside `lidoo-testing`.
- `drop --name testing --database testing_db` fails before Docker execution,
  while the same command with `--yes` invokes `click-odoo-dropdb`.
- A profile can initialize or update more than one database by changing only
  `--database`.
- New profile builds avoid the legacy builder banner when Buildx is available,
  and successful Docker lifecycle output contains no container IDs.
- Database operations do not emit the known Courier/RST noise or ANSI color
  sequences, while real errors remain visible.
- Unit tests cover database-name validation and generated click-odoo command
  arguments; Go tests, vet, image build, and Docker help smoke tests pass.
