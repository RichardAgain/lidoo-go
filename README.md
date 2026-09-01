# lidoo

Create the shared database configuration before starting the services:

```sh
cp .env.example .env
docker compose up -d
go run ./cmd up --name testing --version 18
```

To list all profiles discovered through Docker labels, including stopped
profiles:

```sh
go run ./cmd list
```

The CLI automatically adds `testing.lidoo.test` to the system hosts file. The
first run may ask for administrator permission (`sudo` on Unix or UAC on
Windows). Then open Odoo at:

```text
http://testing.lidoo.test
```

If the profile was created before Traefik routing was enabled, it must be
recreated so it receives the new routing labels. For an empty profile:

```sh
go run ./cmd remove --name testing --yes
go run ./cmd up --name testing --version 18
```

The `remove` command also removes the profile's Lidoo-managed hosts entry.

Before doing this with a populated profile, back up its Odoo filestore and
add-ons volumes. The current CLI does not migrate those volumes automatically;
removing the container leaves the old anonymous volumes detached. The
PostgreSQL data volume is managed separately by Compose.
