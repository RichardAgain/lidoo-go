# Lidoo project instructions

## Architecture principles

- Keep the project as lightweight, fast, and clean as possible.
- Prefer the fewest processes, services, dependencies, and moving parts that
  satisfy the requirement.
- Do not add Traefik, a reverse proxy, or another always-on service unless it
  solves a concrete requirement that cannot be handled by the existing Docker
  CLI and dynamic ports.
- Treat a profile as the runtime unit managed by Lidoo. One profile may expose
  multiple Odoo databases; never assume that one profile maps to one database.
- Continue using Docker labels as the runtime discovery mechanism. In
  particular, `io.lidoo.name` identifies a profile/container for lifecycle
  operations.
- For browser session isolation, use a profile-level hostname routed through
  the shared entrypoint. Do not use a different port on `localhost` as the
  only isolation mechanism, because browser cookies are not separated by port.
- Use `<profile>.lidoo.localhost` for profile hostnames. The `.localhost`
  suffix provides local resolution; do not modify the system hosts file or
  add custom DNS management. Keep hostname routing out of the database
  identity.
- Introduce profile files only when persistent, structured configuration (for
  example add-ons) cannot be represented safely with labels; do not add files
  merely to duplicate runtime discovery.

## Decision rule for new components

Before adding a process, dependency, or configuration layer, state the problem
it solves, why the current Docker/CLI flow cannot solve it, and the operational
cost it introduces. Do not add DNS or hosts-file management for local profile
routing.

## Documentation map

- `ARCHITECTURE.md` — layers, packages, runtime model (profiles, shared
  Postgres/Caddy, add-on mounts, database lifecycle). Read it before changing a
  package boundary or the runtime model.
- `HISTORY.md` — how the CLI and TUI got here; read it for the "why" behind a
  command or constraint.
- `README.md` — user-facing command reference; keep it in sync with the CLI.
- A structural change updates `ARCHITECTURE.md` in the same commit; a new
  command or behavior updates `README.md`.

## Commits

Use Conventional Commits (`feat(scope):`, `fix(scope):`, `docs:`, `chore:`),
one reviewable work unit per commit.

## Criterio ante información faltante

Cuando se delega una tarea, casi nunca vienen todos los detalles: parte está en el mensaje, parte en el código y parte en la cabeza del usuario. Completar los vacíos con suposiciones puede parecer razonable y aun así diferir de lo que el usuario tenía en mente. Ante cualquier vacío:

1. Primero busca la respuesta en el código y en lo ya conversado.
2. Si sigue faltando información, pregunta al usuario antes de avanzar.
3. No inventes requisitos ni hagas cambios fuera de lo pedido. Si ves algo que valdría la pena cambiar, menciónalo y espera confirmación.

Cuanto más trabajo se delega, más importa tener claro hasta dónde se puede avanzar solo.
