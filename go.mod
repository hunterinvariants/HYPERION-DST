module github.com/hunterinvariants/promtact

go 1.25

// Pinned for security, not for language features: 1.25.13 fixes the
// crypto/tls, net/http and encoding/asn1 advisories that govulncheck reports
// as reachable from this module. Bump it when the next one lands.
toolchain go1.25.13
