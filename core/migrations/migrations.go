// Package migrations embeds the ordered PostgreSQL migrations.
package migrations

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

//go:embed *.sql
var files embed.FS

type Migration struct {
	Version int
	Name    string
	SQL     string
}

// All returns migrations in version order.
func All() ([]Migration, error) {
	names, err := fs.Glob(files, "*.sql")
	if err != nil {
		return nil, fmt.Errorf("list migrations: %w", err)
	}
	sort.Strings(names)

	result := make([]Migration, 0, len(names))
	for index, name := range names {
		prefix, _, ok := strings.Cut(name, "_")
		if !ok {
			return nil, fmt.Errorf("invalid migration name %q", name)
		}
		version, err := strconv.Atoi(prefix)
		if err != nil {
			return nil, fmt.Errorf("parse migration version %q: %w", name, err)
		}
		if version != index+1 {
			return nil, fmt.Errorf("migration %q has version %d; want %d", name, version, index+1)
		}

		contents, err := files.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", name, err)
		}
		result = append(result, Migration{Version: version, Name: name, SQL: string(contents)})
	}
	return result, nil
}
