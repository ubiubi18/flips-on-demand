package main

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// PassphraseEnvVar lets scripted use supply a passphrase without putting it on
// the command line, where it would be visible to other processes.
const PassphraseEnvVar = "FOD_PASSPHRASE"

// readPassphrase reads a vault passphrase.
//
// Order: the environment variable when set (scripted and test use), otherwise an
// interactive prompt with echo disabled. A passphrase is never accepted as a
// command-line flag.
func readPassphrase(prompt string) (string, error) {
	if value, ok := os.LookupEnv(PassphraseEnvVar); ok {
		return value, nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", fmt.Errorf("%s is not set and stdin is not a terminal", PassphraseEnvVar)
	}
	fmt.Fprint(os.Stderr, prompt)
	raw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read passphrase: %w", err)
	}
	return strings.TrimRight(string(raw), "\r\n"), nil
}

// confirmPassphrase re-reads interactively to catch typos when writing a vault.
// With the environment variable set there is nothing to confirm.
func confirmPassphrase() (string, error) {
	if value, ok := os.LookupEnv(PassphraseEnvVar); ok {
		return value, nil
	}
	first, err := readPassphrase("Passphrase: ")
	if err != nil {
		return "", err
	}
	fmt.Fprint(os.Stderr, "Repeat passphrase: ")
	raw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read passphrase confirmation: %w", err)
	}
	second := strings.TrimRight(string(raw), "\r\n")
	if first != second {
		return "", fmt.Errorf("passphrases do not match")
	}
	return first, nil
}
