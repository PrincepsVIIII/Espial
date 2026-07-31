package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run(context.Background(), []string{"version"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if strings.TrimSpace(stdout.String()) != version {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestUnknownCommandPrintsUsage(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run(context.Background(), []string{"unknown"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(stderr.String(), "usage:") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestReadPasswordRequiresMatchingConfirmation(t *testing.T) {
	password, err := readPassword(strings.NewReader("correct horse battery\ncorrect horse battery\n"), &bytes.Buffer{})
	if err != nil || password != "correct horse battery" {
		t.Fatalf("password = %q, error = %v", password, err)
	}
	if _, err := readPassword(strings.NewReader("first\nsecond\n"), &bytes.Buffer{}); err == nil {
		t.Fatal("mismatched confirmation was accepted")
	}
}

func TestParseAdminCommands(t *testing.T) {
	tests := []struct {
		args      []string
		operation string
		username  string
		role      string
	}{
		{[]string{"admin", "bootstrap", "--username", "admin"}, "bootstrap", "admin", ""},
		{[]string{"admin", "user", "create", "--username", "viewer", "--role", "viewer"}, "create", "viewer", "viewer"},
		{[]string{"admin", "user", "role", "--username", "viewer", "--role", "operator"}, "role", "viewer", "operator"},
		{[]string{"admin", "user", "password", "--username", "viewer"}, "password", "viewer", ""},
		{[]string{"admin", "user", "disable", "--username", "viewer"}, "disable", "viewer", ""},
	}
	for _, test := range tests {
		command, ok := parseAdminCommand(test.args)
		if !ok || command.operation != test.operation || command.username != test.username || command.role != test.role {
			t.Fatalf("parse %#v = %#v, %v", test.args, command, ok)
		}
	}
	if _, ok := parseAdminCommand([]string{"admin", "user", "create", "viewer"}); ok {
		t.Fatal("malformed admin command was accepted")
	}
}
