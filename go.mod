// Module path must match this repository's actual location — update it,
// and every import statement under internal/ and cmd/, together in one
// commit if this repo is ever forked or renamed.
module github.com/harveysandiego/receiptd

// Deliberately the previous stable release (oldstable), not the newest
// one: the `go` directive is a minimum-version requirement, and with
// Go's automatic toolchain switching (GOTOOLCHAIN=auto, the default),
// pinning it to the newest release would make every contributor and
// every "oldstable" leg of the CI matrix (.github/workflows/ci.yml)
// silently auto-download the newest toolchain just to build — quietly
// defeating the point of testing against the previous release at all.
// Bump this only when the *previous* stable release changes (i.e. once
// every ~6 months, together with the CI matrix), not on every new Go
// release.
go 1.25.0

require (
	github.com/spf13/cobra v1.10.2
	go.etcd.io/bbolt v1.5.0
	golang.org/x/image v0.44.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	golang.org/x/sys v0.45.0 // indirect
)
