module vulnfixture

// Deliberately old: the selftest scans this module with the Go 1.24.0
// standard library, which has reachable vulnerabilities (case vuln-reachable).
go 1.24.0
