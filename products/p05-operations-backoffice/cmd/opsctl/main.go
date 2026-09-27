// Command opsctl is the P05 operations CLI ([N20]). Each command group maps to
// a future opsd API resource. Commands are implemented per WBS2-P05-01..04.
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// commands lists the planned groups and the WBS task that implements them.
var commands = map[string]string{
	"merchant":    "register | revoke | payout-change | payout-cancel   (WBS2-P05-01, P05-04)",
	"attestation": "issue                                              (WBS2-P05-01)",
	"order":       "sign   (test merchant)                             (WBS2-P05-01)",
	"rental":      "provision | re-anchor | return                     (WBS2-P05-02, P05-03)",
	"withdraw":    "request | cancel | execute                         (WBS2-P05-03)",
	"refusal":     "host   (UNSUPPORTED_TYPE demo)                     (WBS2-P05-04)",
	"key":         "handover   (test merchant key to kiosk secretRef)  (WBS2-P05-01)",
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: opsctl <group> <command> [flags]")
	names := make([]string, 0, len(commands))
	for n := range commands {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(os.Stderr, "  %-12s %s\n", n, commands[n])
	}
}

func main() {
	if len(os.Args) < 2 || commands[os.Args[1]] == "" {
		usage()
		os.Exit(2)
	}
	fmt.Fprintf(os.Stderr, "opsctl %s: not implemented yet (%s)\n", os.Args[1], strings.TrimSpace(commands[os.Args[1]]))
	os.Exit(1)
}
