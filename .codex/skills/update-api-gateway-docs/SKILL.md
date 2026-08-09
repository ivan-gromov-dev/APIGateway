---
name: update-api-gateway-docs
description: Synchronize all API Gateway documentation with the current repository, including README, configuration and deployment guides, test documentation, scoped contributor instructions, changelog, and roadmap. Use after completing a milestone or release, before tagging or publishing a release, or whenever documentation may have drifted from code, configuration, Docker, CI, tests, or supported runtime behaviour.
---

# Update API Gateway Documentation

Keep documentation evidence-based and consistent with the repository. Treat code,
strict configuration, tests, Docker Compose, and CI as the sources of truth.

## Establish the baseline

1. Read the root `AGENTS.md`, `README.md`, `CHANGELOG.md`, and `ROADMAP.md`.
2. Inspect `git status --short --branch`, recent tags and commits, and preserve
   unrelated changes.
3. Find every Markdown file and every scoped `AGENTS.md` in the repository.
4. Inspect the current configuration schema and examples, runtime wiring,
   public and admin endpoints, Docker Compose files, CI workflows, and tests.
5. Use repository evidence to identify stale, missing, contradictory, or
   duplicated statements. Do not infer shipped behaviour from roadmap text.

## Synchronize the documentation surface

- Update README capabilities, architecture, examples, semantics, operational
  endpoints, configuration, repository layout, quality gates, and limitations.
- Update deployment, integration, load, and smoke-test guides when their actual
  commands, services, prerequisites, or coverage changed.
- Update scoped `AGENTS.md` only when package ownership or contributor contracts
  changed; do not turn them into feature documentation.
- Keep checked-in YAML and Docker examples runnable and aligned with documented
  defaults. Do not weaken strict validation to preserve an obsolete example.
- Preserve explicit security boundaries, failure policies, cancellation,
  lifecycle ownership, bounded observability, and local-demo warnings.
- Check links, file paths, commands, version references, and terminology across
  all documentation.

## Close a release

When the user asks to mark a version released:

1. Verify the version from the requested release, an existing tag, or an
   unambiguous repository state. Never guess a version.
2. Replace `Unreleased` for that version in `CHANGELOG.md` with the release date.
   Use the repository tag date when available; otherwise use the user-confirmed
   date or the current date when the release happened today.
3. Ensure the changelog describes only behaviour present in that release and
   that its comparison link targets the correct tag.
4. Remove the completed milestone and its delivered checklist from
   `ROADMAP.md`. Keep shipped behaviour in `CHANGELOG.md` and concise current
   boundaries in `README.md`; the roadmap contains future work only.
5. Promote the next incomplete milestone to the leading roadmap position and
   make its status and dependency ordering explicit.
6. Remove stale phrases such as "planned", "next", or "unreleased" when they
   refer to the completed version.

Do not create, move, or rewrite Git tags, commits, releases, or branches unless
the user requests those actions separately.

## Validate

Review the final diff for contradictions and accidental scope expansion. Run
format or repository checks appropriate to the changed artifacts; for broad
documentation updates, run the repository verification gate when practical.
Report files changed, checks run, and any claims that could not be verified.
