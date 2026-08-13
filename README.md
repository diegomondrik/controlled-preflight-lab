# Controlled Preflight Laboratory

This repository seed defines a synthetic laboratory for testing whether a hosted
change-review topology preserves permission boundaries, binds decisions to the
intended commit, and keeps candidate-controlled execution separate from a
protected decision path.

The seed is intentionally small and brand-neutral. It contains no production
source, customer data, credentials, private evidence, operational findings, or
real vulnerability details.

## Trust boundaries

- `governance.yml` runs from the protected base on `pull_request_target`. It
  treats candidate changes only as path and identity data and emits a protected
  governance record for the exact candidate commit.
- `candidate-runtime.yml` runs in the untrusted pull-request domain. Candidate
  code is confined to a digest-pinned, network-disabled, read-only container.
  Its result is evidence input only; it is never a protected decision.
- `protected-decision.yml` runs from the protected base after the candidate
  workflow. It consumes provider metadata, protected check metadata, and the
  base manifest. It does not download, parse, or execute candidate content or
  artifacts.

Every identity comparison is exact and every ambiguous or missing observation
fails closed. The target branch is `main`; the required protected result is
named `protected-decision`.

## Synthetic cases

The declarations in `preflight/cases/` describe bounded experiments for
permission denial, check-name impersonation, missing protected producers,
stale commit identity, neutral/skipped results, protected attribution, and
hostile candidate input. Candidate branches and per-case file deltas are
separate experiment inputs; this base seed does not create them.

## Publication warning

Public repository contents and hosted workflow logs should be treated as
irreversible disclosure. Use only synthetic inputs. Do not add secrets,
personal information, confidential data, real exploit material, internal
identifiers, local filesystem paths, or third-party material without an
independent publication right.

## License posture

No license is granted by this seed. Absence of a license is intentional.
