# Migración `ujap` — Odoo 18 Enterprise → Odoo 19

**Fecha:** 2026-09-29 · **Estado:** ensayo completado y validado · **Registro máquina:** [`ujap-18to19.json`](ujap-18to19.json)

## Comparar las dos bases

| | Perfil | URL | Base |
|---|---|---|---|
| Origen 18 | `ujap18` | <http://ujap18.lidoo.localhost> | `ujap18__ujap` |
| Destino 19 | `ujap19` | <http://ujap19.lidoo.localhost> | `ujap19__ujap` |

Ambos perfiles viven en `lidoo-go` y comparten el contenedor `lidoo-postgres`.
Se levantan con `lidoo run --name ujap18 --version 18` y
`lidoo run --name ujap19 --version 19`.

## Commits de origen (18.0) — **guardar para el seguimiento**

| Repo | Commit 18.0 |
|---|---|
| `l10n-ve-lidoo` | `dd4695d52366a99cc7abca413445004612e5ddf1` |
| `lidoo-analytics` | `8c8c36bb6581b48cecbf8b94455077ae10d8e1b2` |

Son la **base del port**. Si la 18 recibe cambios, esos commits son el punto de
partida para comparar y volver a portar.

Backup de origen: `/home/moi/Descargas/ujap_2026-09-29_13-44-22.zip`
(`18.0+e-20260908`, PostgreSQL 17, 162 módulos instalados, 36 propios). Las
versiones por módulo están en el JSON.

## Commits de destino (19)

| Repo | Commit |
|---|---|
| `l10n-ve-lidoo` | `port/odoo19-compat @ 65f62aed862f36172f087ad1c60ee17471347d89` |
| `lidoo-analytics` | `port/odoo19-compat @ 003dc8ae74f08a0c4d19b7d207ce7a77d263e659` |

OpenUpgrade: `709bffde906a4df1810dc998f49232cf1759e070` (rama `19.0`).

## Cómo se hizo

```bash
# perfil destino con los addons copiados y adjuntos
lidoo migrate --name ujap19 --source-profile ujap18 \
  --database ujap --openupgrade openupgrade --force
```

El comando hace, dentro de un contenedor one-off con el runtime del perfil:

1. `click-odoo-backupdb` del origen (con filestore).
2. `odoo db load` — carga el esquema pre-upgrade **sin abrir el registry**.
3. `odoo -u all --load=base,web,openupgrade_framework,lidoo_upgrade_19`.

`-u all` ejecuta los scripts de migración propios de **cada** módulo instalado
(Community **y** Enterprise); OpenUpgrade solo **añade** los suyos para
Community. **Sin SQL externo**: nada de `psql` ni bridges.

## Resultado

| Verificación | Resultado |
|---|---|
| Módulos instalados | 162 → **171** |
| Módulos propios `l10n_ve_lidoo_*` | **36** preservados |
| Módulos pendientes | **0** |
| `res_partner` · `product_template` | 89 → 89 · 45 → 45 |
| `account_move` · `account_move_line` | 390 → 390 · 899 → 899 |
| `account_payment` · `l10n_ve_lidoo_withholding` | 28 → 28 · 230 → 230 |
| `account_move_line.no_followup` | 473 → 473 |

### Renames Enterprise

`account_auto_transfer` → `account_transfer`; `account_disallowed_expenses` →
`account_fiscal_categories`; `account_no_followup` (backport solo-18) →
`account`. OpenUpgrade no los mapea (su `apriori` solo cubre Community), así que
los aporta el addon **`lidoo_upgrade_19`** cargado con `--load`. Los dos primeros
módulos tenían **0 filas**: no había configuración que preservar.

## Cuando la 18 se actualice

1. Anotar el nuevo commit de `18.0` de `l10n-ve-lidoo` y `lidoo-analytics`.
2. Revisar el diff `commit_anterior..nuevo` y portarlo a `port/odoo19-compat`
   (misma disciplina: APIs nativas 19, no copiar el código de 18).
3. Re-ejecutar el ensayo sobre una **copia nueva** del dump actualizado.
4. Comparar: módulos, conteos y los flujos fiscales (factura, retención IVA/ISLR,
   IGTF, libro IVA, guía de despacho).

## Referencias

- Plan y bitácora del port: `docs/plans/l10n-ve-lidoo/2026-09-28-odoo19-compatibility-handoff.md`
- Plan de migración: `docs/plans/l10n-ve-lidoo/2026-09-29-ujap-18to19-migration-plan.md`
- Copia de este documento en `lazylidoo`: `docs/migrations/ujap-18to19.md`
