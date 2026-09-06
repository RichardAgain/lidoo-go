# Profile Host Routing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Route each multi-database Odoo profile through a profile hostname on port 80 while preserving label-based lifecycle management.

**Architecture:** Add one Traefik service to the existing Compose network. The CLI creates each Odoo container without a host port and attaches Docker labels that route `<profile>.lidoo.test` to port 8069; the CLI manages the profile's `127.0.0.1` entry in the system hosts file through a cross-platform helper.

**Tech Stack:** Go 1.26, Docker CLI, Docker Compose, Traefik Docker provider, PostgreSQL 17, Odoo 18.

**Spec:** `docs/superpowers/specs/2026-08-31-profile-host-routing-design.md`

## Global Constraints

- Keep one shared Traefik process for all profiles.
- Keep PostgreSQL and profile database ownership unchanged.
- Keep Docker labels as runtime metadata and discovery.
- Do not add a profile configuration file for this routing-only change.
- Manage only Lidoo-owned hosts entries and never overwrite conflicting user
  mappings.
- Do not add a resident hosts or DNS service; elevation is performed only for
  the write operation when required.
- Keep `list` read-only: discover profiles through `io.lidoo.name` and report
  their state and derived hostname.

---

### Task 1: Define and test profile routing metadata

- **Files:**
- Modify: `internal/docker/up.go`
- Create: `internal/docker/up_test.go`

**Interfaces:**
- Produce `profileHostname(name string) string` returning `name + ".lidoo.test"`.
- Produce `validateProfileName(name string) error` rejecting names that are not lowercase DNS-safe slugs.
- Produce `traefikLabels(name string) []string` containing the enabled flag,
  Docker network, router rule, entrypoint, and backend port labels.

- [ ] **Step 1: Write failing tests**

Add table-driven tests for valid names (`testing`, `dev-18`) and invalid names
(`Testing`, `dev_18`, `-dev`, `dev.`), plus an exact assertion that the labels
for `testing` contain `Host(\`testing.lidoo.test\`)` and backend port `8069`.

- [ ] **Step 2: Run the focused tests and verify the expected failure**

Run `go test ./internal/docker -run 'Test(ProfileName|Traefik)' -v`.
Expected result: compilation fails because the new helpers do not exist yet.

- [ ] **Step 3: Implement the minimal pure helpers**

Add the validation regexp, hostname helper, and deterministic label builder in
`up.go`. Keep the existing `io.lidoo.name` label in the generated container
arguments and add the Traefik labels alongside it.

- [ ] **Step 4: Run the focused tests and verify they pass**

Run `go test ./internal/docker -run 'Test(ProfileName|Traefik)' -v`.
Expected result: all focused tests pass.

### Task 2: Route Odoo containers through Traefik

**Files:**
- Modify: `internal/docker/up.go`
- Modify: `internal/docker/common.go`

**Interfaces:**
- `Up` validates the profile slug, creates the container without `-p`, and
  reports the derived `http://<profile>.lidoo.test` URL.
- Existing lifecycle operations continue filtering on `io.lidoo.name`.

- [ ] **Step 1: Replace port publication with routing labels**

Remove the host-port allocation and inspection path from `Up`. Pass the labels
from `traefikLabels` to `docker run`, keep `--network lidoo-net`, and leave the
Odoo internal port at 8069 for Traefik.

- [ ] **Step 2: Update the startup report**

Replace the port inspection report with a hostname report. The CLI ensures the
profile hostname is mapped to `127.0.0.1` through the cross-platform hosts
helper, requesting elevation only when the operating system requires it.

- [ ] **Step 3: Run unit tests and compile the packages**

Run `go test ./...`.
Expected result: all tests pass and the package compiles without unused port
allocation code.

### Task 3: Add the shared Traefik service and documentation

**Files:**
- Modify: `compose.yaml`
- Modify: `README.md`

**Interfaces:**
- Compose provides a container named `lidoo-traefik` on `lidoo-net`, listening
  on host port 80 and reading Docker metadata through a read-only socket.
- README documents automatic hosts-file management, elevation prompts, and the
  new startup URL.

- [ ] **Step 1: Add Traefik to Compose**

Add the pinned `traefik:v3.7` image with the Docker provider enabled,
`exposedByDefault=false`, the `web` entrypoint on `:80`, the read-only Docker
socket mount, host port `80:80`, and membership in `lidoo-net`.

- [ ] **Step 2: Update the documented startup flow**

Change the README startup command to `docker compose up -d`, document that the
CLI adds the managed hosts entry with elevation when needed, and show
`http://testing.lidoo.test` as the resulting URL.

- [ ] **Step 3: Validate Compose and the final diff**

Run `docker compose config` and `git diff --check`.
Expected result: Compose renders successfully and the diff has no whitespace
errors.

### Task 4: Verify runtime routing

**Files:**
- No source changes.

- [ ] **Step 1: Start the shared services**

Run `docker compose up -d` from the repository root and confirm both
`lidoo-postgres` and `lidoo-traefik` are running.

- [ ] **Step 2: Recreate the test profile if it predates routing labels**

For an empty legacy profile, use the existing lifecycle command to remove the
old `lidoo-testing` container, then run `go run ./cmd up --name testing
--version 18`. For a populated profile, back up and manually migrate Odoo's
filestore/add-ons volumes first; this change does not automate that migration.

- [ ] **Step 3: Verify the generated route**

Confirm the CLI adds the managed `testing.lidoo.test` entry automatically,
request `http://testing.lidoo.test`, and confirm Traefik routes to Odoo. Inspect
the container labels and confirm no host port is published for the Odoo
container.
