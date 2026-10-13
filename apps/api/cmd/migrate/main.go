package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

const usageText = `usage: migrate [flags] <command> [args]

commands:
  up              apply all pending migrations
  down N          roll back the last N migrations
  version         print the current schema version
  force V         force schema version V without running migrations
  create NAME     create the next empty up/down migration pair
`

func main() {
	dir := flag.String("dir", "infra/db/migrations", "path to the migrations directory")
	database := flag.String("database", os.Getenv("API_DATABASE_URL"), "database URL; defaults to API_DATABASE_URL")
	flag.Usage = func() { fmt.Fprint(os.Stderr, usageText) }
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		os.Exit(2)
	}

	var err error
	switch args[0] {
	case "create":
		if len(args) != 2 {
			fatal("create requires exactly one migration name")
		}
		err = createMigrationPair(*dir, args[1])
	case "up", "down", "version", "force":
		if *database == "" {
			fatal("database URL is required (pass -database or set API_DATABASE_URL)")
		}
		err = runMigration(*dir, *database, args)
	default:
		flag.Usage()
		os.Exit(2)
	}

	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		fatal(err.Error())
	}
	if errors.Is(err, migrate.ErrNoChange) {
		fmt.Println("database is up to date; no change")
	}
}

func runMigration(dir, database string, args []string) error {
	m, err := migrate.New("file://"+filepath.ToSlash(dir), database)
	if err != nil {
		return err
	}
	defer m.Close()

	switch args[0] {
	case "up":
		return m.Up()
	case "down":
		if len(args) != 2 {
			return errors.New("down requires a step count, e.g. down 1")
		}
		steps, err := strconv.Atoi(args[1])
		if err != nil || steps < 1 {
			return fmt.Errorf("down steps must be a positive integer, got %q", args[1])
		}
		return m.Steps(-steps)
	case "version":
		version, dirty, err := m.Version()
		if err != nil {
			return err
		}
		fmt.Printf("version=%d dirty=%t\n", version, dirty)
		return nil
	case "force":
		if len(args) != 2 {
			return errors.New("force requires a version, e.g. force 1")
		}
		version, err := strconv.Atoi(args[1])
		if err != nil || version < -1 {
			return fmt.Errorf("force version must be an integer >= -1, got %q", args[1])
		}
		return m.Force(version)
	}
	return fmt.Errorf("unknown command %q", args[0])
}

func createMigrationPair(dir, name string) error {
	if name == "" || strings.TrimSpace(name) != name || strings.ContainsAny(name, "/\\ ") {
		return fmt.Errorf("invalid migration name %q", name)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	maxSeq := 0
	for _, e := range entries {
		base := filepath.Base(e.Name())
		if len(base) < 7 || base[6] != '_' {
			continue
		}
		if n, err := strconv.Atoi(base[:6]); err == nil && n > maxSeq {
			maxSeq = n
		}
	}
	next := fmt.Sprintf("%06d", maxSeq+1)
	for _, suffix := range []string{"up.sql", "down.sql"} {
		path := filepath.Join(dir, next+"_"+name+"."+suffix)
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			return err
		}
		fmt.Printf("created %s\n", path)
	}
	return nil
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, "migrate:", msg)
	os.Exit(1)
}
