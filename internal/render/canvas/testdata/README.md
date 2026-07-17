# Golden image fixtures

This directory will hold golden PNG fixtures for `canvas` package tests
(see `docs/ARCHITECTURE.md` §9 and `CONTRIBUTING.md`'s testing
requirements). Regenerate fixtures with the test suite's `-update` flag
once it exists, and review the diff of any regenerated fixture like a
real code change — a golden file that silently starts matching a broken
implementation defeats the point of the test.
