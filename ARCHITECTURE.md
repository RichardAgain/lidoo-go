# Architecture

Lidoo is a single Go program that manages **Odoo profiles as Docker
containers**. It deliberately keeps the moving parts small: one Go process, the
Docker CLI, a shared PostgreSQL, and a shared Caddy entrypoint. No Kubernetes,
no per-profile reverse proxy, no DNS or hosts-file management.

## Layers

| Layer | Package | Responsibility |
| --- | --- | --- |
| Entry point | `cmd/` | Flags, command dispatch, terminal presentation |
| Application | `internal/app` | Use cases: profiles, databases, add-ons, versions, workspace transactions |
| Odoo operations | `internal/odoo` | `init`, `update`, `backup`, `restore`, `copydb`, `migrate`, `wait`, `db shell` via click-odoo |
| Docker runtime | `internal/docker` | Container lifecycle, labels, shared compose services, volumes |
| Routing | `internal/proxy` | Generate the shared Caddyfile from profile routes |
| Add-ons | `internal/addons` | Registry, Git clone/worktree, mount resolution |
| Profiles | `internal/profile` | Profile config: name, DB prefix, Odoo version, DB filter |
| Profile IO | `internal/profileio` | Export/import a portable profile document |
| State | `internal/files` | `.lidoo.json` workspace state and lock |
| TUI | `internal/tui` | Interactive Bubble Tea interface |

The `bin/lidoo` wrapper is a development convenience: it `cd`s to the repository
root and runs `go run ./cmd`.

## Runtime model

- A **profile** is the runtime unit, persisted under `containers` in
  `.lidoo.json`. One profile may expose **several** Odoo databases; never assume
  one profile maps to one database.
- Running a profile creates container `lidoo-<profile>` on the `lidoo-net`
  network, labelled `io.lidoo.name=<profile>`. Docker labels are the discovery
  mechanism for `list`, `status` and the TUI.
- Shared services come from `compose.yaml` and are started on demand, not
  always-on: `db` (`lidoo-postgres`) and `caddy` (`lidoo-caddy`).
- Add-ons are mounted at `/opt/addons/<name>` and appended to the Odoo
  `--addons-path` through `LIDOO_ADDONS_PATH`.
- Database names are `<prefix><logical>`, e.g. `testing__db`. The profile's
  `db_filter_mode` (`profile`, `custom`, `disabled`) drives `--db-filter`.
- Each profile is reached at `http://<profile>.lidoo.localhost`. The CLI
  regenerates `docker/caddy/Caddyfile` and reloads Caddy. The `.localhost`
  suffix resolves locally without touching `/etc/hosts`.
- Browser session isolation comes from the **profile hostname**, not from a
  different port on `localhost` (cookies are not separated by port).
- Managed containers also receive Docker's standard
  `host.docker.internal:host-gateway` alias, preserved by one-off operations.
  This lets an explicit integration client reach an SSH tunnel on the Docker
  host; it is unrelated to profile routing and does not alter system DNS/hosts.
  The Odoo 18 image includes PySocks for `requests` SOCKS support. A host tunnel
  is still user-started, bound to the bridge address only; no proxy service is added.

## Workspace state

`.lidoo.json` is the canonical local state file:

```json
{
  "_schemaVersion": 1,
  "addons": {
    "lidoo": { "path": "addons/lidoo", "source": "git@github.com:LIDALabs/l10n-ve-lidoo.git" }
  },
  "containers": {
    "testing": { "addons": ["lidoo"], "prefix": "testing__", "version": "18", "db_filter_mode": "profile" }
  }
}
```

`internal/files` reads and writes it under a lock (`op-lease.lock`) so
concurrent CLI/TUI operations do not corrupt state. Application services run
each mutation inside a workspace transaction (`withWorkspaceOperation`) that
saves state only on success.

## Add-ons

An add-on is a Git checkout or a `git worktree`, registered by name with a
`path` and an optional `source`/`branch`. Profiles attach add-ons by name; the
resolver mounts each path and rejects two names that resolve to the same
directory. Worktrees let one profile run a feature branch while another stays
on stable, without flipping the shared checkout.

## Database lifecycle

Every database operation goes through the Odoo image and click-odoo-contrib
(`click-odoo-initdb`, `click-odoo-update`, `click-odoo-backupdb`…), so the
client keeps template caching, checksum-selective updates and filestore
awareness. `internal/odoo` is the only place that invokes those tools; the
`odoo-bin` entrypoint is bypassed when a raw Odoo process is needed so the
image's PostgreSQL argument injection does not interfere.

## Design rules

- Fewest processes, services and dependencies that satisfy the requirement.
- Prefer the existing Docker CLI + dynamic ports over adding a new always-on
  service.
- Keep hostname routing out of the database identity.
- Introduce profile files only for structured configuration that labels cannot
  represent safely.
