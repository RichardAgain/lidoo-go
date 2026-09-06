# Profile Host Routing Design

## Goal

Allow each Lidoo profile to be accessed through a profile-level hostname
without exposing an Odoo port in the browser URL, while preserving the
existing label-based discovery model and allowing each profile to contain
multiple Odoo databases.

## Design

The Docker Compose stack will add one Traefik container listening on host port
80. Odoo containers will no longer publish host ports. When the CLI creates a
profile, it will add Docker labels that route `<profile>.lidoo.test` to the
container's internal Odoo port 8069. The CLI will manage the profile hostname
in the system hosts file through a cross-platform library and request elevated
permissions only when the operating system requires them.

The existing `io.lidoo.name=<profile>` label remains the lifecycle and
discovery identity. Hostnames are derived from that profile name and are not
derived from, or assigned to, individual databases. No database creation,
database filtering, add-on configuration, or reverse-proxy process per
profile is included in this change.

The `list` command uses the same label to inventory profiles, including stopped
containers. It reports the profile name, Docker state, and derived hostname
without changing Docker or the hosts file.

## Runtime flow

```text
Browser: testing.lidoo.test:80
    -> /etc/hosts: 127.0.0.1
    -> Traefik: host port 80, Docker provider
    -> lidoo-testing:8069 on lidoo-net
```

The profile name must be a lowercase DNS-safe slug matching
`[a-z0-9][a-z0-9-]*`. This prevents invalid hostnames and unsafe Traefik rule
values. Existing containers created with the old direct-port flow must be
removed and recreated once so that they receive the Traefik labels and stop
using their old host-port mapping; their PostgreSQL data is unaffected. The
CLI does not migrate Odoo's anonymous filestore or add-on volumes, so populated
legacy profiles require a manual volume backup/migration before recreation.

## Project constraints

- Keep one shared Traefik process for all profiles.
- Keep PostgreSQL and profile database ownership unchanged.
- Keep Docker labels as runtime metadata and discovery.
- Do not add a profile configuration file for this routing-only change.
- Manage only Lidoo-owned hosts entries and never overwrite conflicting user
  mappings.
- Do not add a resident hosts or DNS service; elevation is performed only for
  the write operation when required.

## Acceptance criteria

- `docker compose up -d` starts PostgreSQL, Traefik, and the shared network.
- `go run ./cmd up --name testing --version 18` creates `lidoo-testing` with
  Traefik labels and no host port publication.
- The CLI reports `http://testing.lidoo.test` and automatically creates its
  hosts-file entry, requesting elevation when required.
- `testing.lidoo.test` and another profile hostname route to different Odoo
  containers and therefore use separate browser cookie scopes.
- `stop`, `restart`, and `remove` continue to find profiles through
  `io.lidoo.name`.
- `list` reports all labeled profiles, including stopped profiles, with their
  derived hostnames.
- Unit tests cover profile-name validation and generated routing metadata.
