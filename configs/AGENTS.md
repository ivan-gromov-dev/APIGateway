# Configuration fixture instructions

Checked-in YAML files are executable documentation and must pass the strict
`internal/config` loader. Keep local and Docker examples aligned for every
public field, using safe local-demo defaults and explicit comments or README
documentation for intentional differences. Never place production credentials
or reusable secrets here.

When changing a public configuration contract, update loader validation and
tests, all applicable fixtures, Docker wiring, and README examples together.
