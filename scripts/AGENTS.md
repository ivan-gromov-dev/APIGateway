# Verification script instructions

Scripts are cross-platform developer entry points. PowerShell and POSIX
variants must enforce equivalent gates, fail immediately with actionable
output, run from any current directory, and leave generated artifacts only
under ignored `.cache` or documented result directories.

Never weaken formatting, vet, test, race, build, or coverage policy for local
convenience. Quote paths and keep external-tool requirements documented.
