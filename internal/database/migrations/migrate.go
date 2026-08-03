package migrations

import (
	"embed"
	"errors"
	"io/fs"
	"log"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed sql/*.sql
var embedFS embed.FS

func RunMigrations(dbURL string) {
	subFS, err := fs.Sub(embedFS, "sql")
	if err != nil {
		log.Fatalf("failed to get subdirectory sql: %v", err)
	}

	sourceDriver, err := iofs.New(subFS, ".")
	if err != nil {
		log.Fatalf("failed to create migrations source: %v", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", sourceDriver, dbURL)
	if err != nil {
		log.Fatalf("failed to create migrate runner: %v", err)
	}

	err = m.Up()

	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		log.Fatalf("failed to runnig migrations: %v", err)
	}

	if errors.Is(err, migrate.ErrNoChange) {
		log.Println("no migrations needed, database is up to date.")
	} else {
		log.Println("migrations applied")
	}
}
