package main

// The CLI's own command list has to carry every registered command. A command
// that works but is invisible in `fod help` is a drift the audit checklist
// names, and it is cheap to check here instead of by reading the file.

import (
	"regexp"
	"strings"
	"testing"
)

func TestUsageListsEveryRegisteredCommand(t *testing.T) {
	text := usageText()
	named := map[string]bool{}
	for _, match := range regexp.MustCompile(`(?m)^  ([a-z0-9-]+)(?: / ([a-z0-9-]+))?\s`).FindAllStringSubmatch(text, -1) {
		named[match[1]] = true
		if match[2] != "" {
			named[match[2]] = true
		}
	}
	for command := range commandModes {
		if !named[command] {
			t.Errorf("`fod %s` is registered but does not appear in the usage text", command)
		}
	}
	// And the list names nothing the CLI cannot run.
	for command := range named {
		if _, ok := commandModes[command]; !ok {
			t.Errorf("the usage text lists `fod %s`, which is not registered", command)
		}
	}
	if !strings.Contains(text, "shared-node") {
		t.Errorf("the usage text must name the shared-node command the author journey uses")
	}
}
