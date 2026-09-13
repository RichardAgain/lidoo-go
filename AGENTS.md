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
