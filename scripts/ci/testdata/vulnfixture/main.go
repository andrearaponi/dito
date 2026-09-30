// Command vulnfixture calls net/url.ParseQuery, which is vulnerable in the
// Go 1.24.0 standard library (GO-2026-4341). The mutation harness scans it
// with that toolchain to prove that the vuln target fails on a reachable
// vulnerability. It is never built or shipped.
package main

import (
	"fmt"
	"net/url"
	"os"
)

func main() {
	values, err := url.ParseQuery(os.Args[1])
	fmt.Println(values, err)
}
