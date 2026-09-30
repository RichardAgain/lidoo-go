# History

Narrative of how `lidoo` got here. For the exact per-change detail see
`git log`; this file explains the shape of the journey.

## Why it exists

Lidoo was built to run **multiple Odoo profiles on one developer or server
machine** without a heavy stack. The constraints were explicit: keep the fewest
processes and dependencies, do not add a reverse proxy per profile, and do not
manage DNS or hosts files. The answer was a single Go CLI, Docker labels for
discovery, dynamic ports, and one shared Caddy entrypoint that routes
`<profile>.lidoo.localhost` to each container.

## Milestones

| Area | What it added |
| --- | --- |
| Profiles | `run` / `stop` / `restart` / `remove`, profile config in `.lidoo.json`, shared `lidoo-postgres` and `lidoo-caddy` |
| Databases | `init`, `update`, `drop`, `db list/info/shell`, `set-password`, always through click-odoo |
| Backups | `backup`, `restore`, `copydb`, with filestore-aware copy modes |
| Routing | generated Caddyfile and `http://<profile>.lidoo.localhost`, no hosts-file edits |
| Add-ons | registry by name, Git clone, `git worktree`, attach/detach per profile |
| Versions | Dockerfiles and images per Odoo version, 17/18/19 selection |
| Migrations | `migrate` between profiles/databases, with the Enterprise rename map |
| TUI | interactive views for profiles, databases and add-ons, live task log |
| Portability | `profile export` / `profile import` documents |

## Notable fixes

- `init` now passes `--no-demo` to `click-odoo-initdb` (the image's
  click-odoo-contrib uses `--demo/--no-demo`, not `--without-demo=all`), which
  unblocked database creation.
- `migrate` bypasses the image entrypoint so its PostgreSQL argument injection
  does not collide with the migration container.
- `remove` handles profiles whose container was never created.
- `init` no longer loads demo data.
- TUI: bracketed paste and `Ctrl+V` clipboard work in every text field, and
  `Ctrl+C` always quits.

## Current state

Profiles are the runtime unit; one profile can expose several databases.
Add-ons can be shared checkouts or per-profile worktrees. The TUI and CLI share
the same application services, so a workflow added to one is available in the
other. The remaining work is mostly polish and new Odoo versions as they land.
