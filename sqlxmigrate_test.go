package sqlxmigrate

import (
	"database/sql"
	"fmt"
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/joho/godotenv/autoload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var databases []database

type database struct {
	name    string
	connEnv string
}

var migrations = []*Migration{
	{
		ID: "201608301400",
		Migrate: func(tx *sql.Tx) error {
			q := `CREATE TABLE "people" ("id" serial,"created_at" timestamp with time zone,"updated_at" timestamp with time zone,"deleted_at" timestamp with time zone,"name" text , PRIMARY KEY ("id"))`
			_, err := tx.Exec(q)
			return err
		},
		Rollback: func(tx *sql.Tx) error {
			q := `DROP TABLE IF EXISTS "people" `
			_, err := tx.Exec(q)
			return err
		},
	},
	{
		ID: "201608301430",
		Migrate: func(tx *sql.Tx) error {
			q := ` CREATE TABLE "pets" ("id" serial,"created_at" timestamp with time zone,"updated_at" timestamp with time zone,"deleted_at" timestamp with time zone,"name" text,"person_id" integer , PRIMARY KEY ("id"))`
			_, err := tx.Exec(q)
			return err
		},
		Rollback: func(tx *sql.Tx) error {
			q := `DROP TABLE IF EXISTS "pets" `
			_, err := tx.Exec(q)
			return err
		},
	},
}

var extendedMigrations = append(migrations, &Migration{
	ID: "201807221927",
	Migrate: func(tx *sql.Tx) error {
		q := ` CREATE TABLE "books" ("id" serial,"created_at" timestamp with time zone,"updated_at" timestamp with time zone,"deleted_at" timestamp with time zone,"name" text,"person_id" integer , PRIMARY KEY ("id"))`
		_, err := tx.Exec(q)
		return err
	},
	Rollback: func(tx *sql.Tx) error {
		q := `DROP TABLE IF EXISTS "books" `
		_, err := tx.Exec(q)
		return err
	},
})

var failingMigration = []*Migration{
	{
		ID: "201904231300",
		Migrate: func(tx *sql.Tx) error {
			// missing person_id column to the insert select below should fail
			q := ` CREATE TABLE "newspapers" ("id" serial,"created_at" timestamp with time zone,"updated_at" timestamp with time zone,"deleted_at" timestamp with time zone,"name" text,, PRIMARY KEY ("id"))`
			_, err := tx.Exec(q)
			if err != nil {
				return err
			}

			// Should fail on column mismatch
			sq := `INSERT INTO newspapers SELECT * FROM books`
			_, err = tx.Exec(sq)
			return err
		},
		Rollback: func(tx *sql.Tx) error {
			q := `DROP TABLE IF EXISTS "newspapers" `
			_, err := tx.Exec(q)
			return err
		},
	},
}

func TestMigration(t *testing.T) {
	forEachDatabase(t, func(db *sqlx.DB) {
		m := New(db, DefaultOptions, migrations)

		err := m.Migrate()
		assert.NoError(t, err)
		assert.True(t, m.hasTable("people"))
		assert.True(t, m.hasTable("pets"))
		assert.Equal(t, 2, tableCount(t, db, "migrations"))

		err = m.RollbackLast()
		assert.NoError(t, err)
		assert.True(t, m.hasTable("people"))
		assert.False(t, m.hasTable("pets"))
		assert.Equal(t, 1, tableCount(t, db, "migrations"))

		err = m.RollbackLast()
		assert.NoError(t, err)
		assert.False(t, m.hasTable("people"))
		assert.False(t, m.hasTable("pets"))
		assert.Equal(t, 0, tableCount(t, db, "migrations"))
	})
}

func TestMigrateTo(t *testing.T) {
	forEachDatabase(t, func(db *sqlx.DB) {
		m := New(db, DefaultOptions, extendedMigrations)

		err := m.MigrateTo("201608301430")
		assert.NoError(t, err)
		assert.True(t, m.hasTable("people"))
		assert.True(t, m.hasTable("pets"))
		assert.False(t, m.hasTable("books"))
		assert.Equal(t, 2, tableCount(t, db, "migrations"))
	})
}

func TestRollbackTo(t *testing.T) {
	forEachDatabase(t, func(db *sqlx.DB) {
		m := New(db, DefaultOptions, extendedMigrations)

		// First, apply all migrations.
		err := m.Migrate()
		assert.NoError(t, err)
		assert.True(t, m.hasTable("people"))
		assert.True(t, m.hasTable("pets"))
		assert.True(t, m.hasTable("books"))
		assert.Equal(t, 3, tableCount(t, db, "migrations"))

		// Rollback to the first migration: only the last 2 migrations are expected to be rolled back.
		err = m.RollbackTo("201608301400")
		assert.NoError(t, err)
		assert.True(t, m.hasTable("people"))
		assert.False(t, m.hasTable("pets"))
		assert.False(t, m.hasTable("books"))
		assert.Equal(t, 1, tableCount(t, db, "migrations"))
	})
}

// If initSchema is defined, but no migrations are provided,
// then initSchema is executed.
func TestInitSchemaNoMigrations(t *testing.T) {
	forEachDatabase(t, func(db *sqlx.DB) {
		m := New(db, DefaultOptions, []*Migration{})
		m.InitSchema(func(tx *sqlx.DB) error {
			q := `CREATE TABLE "animals" ("id" serial,"created_at" timestamp with time zone,"updated_at" timestamp with time zone,"deleted_at" timestamp with time zone,"name" text , PRIMARY KEY ("id"))`
			_, err := tx.Exec(q)
			return err
		})

		assert.NoError(t, m.Migrate())
		assert.True(t, m.hasTable("animals"))
		assert.Equal(t, 1, tableCount(t, db, "migrations"))
	})
}

// If initSchema is defined and migrations are provided,
// then initSchema is executed and the migration IDs are stored,
// even though the relevant migrations are not applied.
func TestInitSchemaWithMigrations(t *testing.T) {
	forEachDatabase(t, func(db *sqlx.DB) {
		m := New(db, DefaultOptions, migrations)
		m.InitSchema(func(tx *sqlx.DB) error {
			q := `CREATE TABLE "people" ("id" serial,"created_at" timestamp with time zone,"updated_at" timestamp with time zone,"deleted_at" timestamp with time zone,"name" text , PRIMARY KEY ("id"))`
			_, err := tx.Exec(q)
			return err
		})

		assert.NoError(t, m.Migrate())
		assert.True(t, m.hasTable("people"))
		assert.False(t, m.hasTable("pets"))
		assert.Equal(t, 3, tableCount(t, db, "migrations"))
	})
}

// If the schema has already been initialised,
// then initSchema() is not executed, even if defined.
func TestInitSchemaAlreadyInitialised(t *testing.T) {

	forEachDatabase(t, func(db *sqlx.DB) {
		m := New(db, DefaultOptions, []*Migration{})

		// Migrate with empty initialisation
		m.InitSchema(func(tx *sqlx.DB) error {
			return nil
		})
		assert.NoError(t, m.Migrate())

		// Then migrate again, this time with a non empty initialisation
		// This second initialisation should not happen!
		m.InitSchema(func(tx *sqlx.DB) error {
			q := `CREATE TABLE "cars" ("id" serial,"created_at" timestamp with time zone,"updated_at" timestamp with time zone,"deleted_at" timestamp with time zone,"name" text , PRIMARY KEY ("id"))`
			_, err := tx.Exec(q)
			return err
		})
		assert.NoError(t, m.Migrate())

		assert.False(t, m.hasTable("cars"))
		assert.Equal(t, 1, tableCount(t, db, "migrations"))
	})
}

// If the schema has not already been initialised,
// but any other migration has already been applied,
// then initSchema() is not executed, even if defined.
func TestInitSchemaExistingMigrations(t *testing.T) {
	forEachDatabase(t, func(db *sqlx.DB) {
		m := New(db, DefaultOptions, migrations)

		// Migrate without initialisation
		assert.NoError(t, m.Migrate())

		// Then migrate again, this time with a non empty initialisation
		// This initialisation should not happen!
		m.InitSchema(func(tx *sqlx.DB) error {
			q := `CREATE TABLE "cars" ("id" serial,"created_at" timestamp with time zone,"updated_at" timestamp with time zone,"deleted_at" timestamp with time zone,"name" text , PRIMARY KEY ("id"))`
			_, err := tx.Exec(q)
			return err
		})
		assert.NoError(t, m.Migrate())

		assert.False(t, m.hasTable("cars"))
		assert.Equal(t, 2, tableCount(t, db, "migrations"))
	})
}

func TestMigrationIDDoesNotExist(t *testing.T) {
	forEachDatabase(t, func(db *sqlx.DB) {
		m := New(db, DefaultOptions, migrations)
		assert.Equal(t, ErrMigrationIDDoesNotExist, m.MigrateTo("1234"))
		assert.Equal(t, ErrMigrationIDDoesNotExist, m.RollbackTo("1234"))
		assert.Equal(t, ErrMigrationIDDoesNotExist, m.MigrateTo(""))
		assert.Equal(t, ErrMigrationIDDoesNotExist, m.RollbackTo(""))
	})
}

func TestMissingID(t *testing.T) {
	forEachDatabase(t, func(db *sqlx.DB) {
		migrationsMissingID := []*Migration{
			{
				Migrate: func(tx *sql.Tx) error {
					return nil
				},
			},
		}

		m := New(db, DefaultOptions, migrationsMissingID)
		assert.Equal(t, ErrMissingID, m.Migrate())
	})
}

func TestReservedID(t *testing.T) {
	forEachDatabase(t, func(db *sqlx.DB) {
		migrationsReservedID := []*Migration{
			{
				ID: "SCHEMA_INIT",
				Migrate: func(tx *sql.Tx) error {
					return nil
				},
			},
		}

		m := New(db, DefaultOptions, migrationsReservedID)
		_, isReservedIDError := m.Migrate().(*ReservedIDError)
		assert.True(t, isReservedIDError)
	})
}

func TestDuplicatedID(t *testing.T) {
	forEachDatabase(t, func(db *sqlx.DB) {
		migrationsDuplicatedID := []*Migration{
			{
				ID: "201705061500",
				Migrate: func(tx *sql.Tx) error {
					return nil
				},
			},
			{
				ID: "201705061500",
				Migrate: func(tx *sql.Tx) error {
					return nil
				},
			},
		}

		m := New(db, DefaultOptions, migrationsDuplicatedID)
		_, isDuplicatedIDError := m.Migrate().(*DuplicatedIDError)
		assert.True(t, isDuplicatedIDError)
	})
}

func TestEmptyMigrationList(t *testing.T) {
	forEachDatabase(t, func(db *sqlx.DB) {
		t.Run("with empty list", func(t *testing.T) {
			m := New(db, DefaultOptions, []*Migration{})
			err := m.Migrate()
			assert.Equal(t, ErrNoMigrationDefined, err)
		})

		t.Run("with nil list", func(t *testing.T) {
			m := New(db, DefaultOptions, nil)
			err := m.Migrate()
			assert.Equal(t, ErrNoMigrationDefined, err)
		})
	})
}

func TestMigration_WithUseTransactions(t *testing.T) {
	options := DefaultOptions

	forEachDatabase(t, func(db *sqlx.DB) {
		m := New(db, options, migrations)

		err := m.Migrate()
		require.NoError(t, err)
		assert.True(t, m.hasTable("people"))
		assert.True(t, m.hasTable("pets"))
		assert.Equal(t, 2, tableCount(t, db, "migrations"))

		err = m.RollbackLast()
		require.NoError(t, err)
		assert.True(t, m.hasTable("people"))
		assert.False(t, m.hasTable("pets"))
		assert.Equal(t, 1, tableCount(t, db, "migrations"))

		err = m.RollbackLast()
		require.NoError(t, err)
		assert.False(t, m.hasTable("people"))
		assert.False(t, m.hasTable("pets"))
		assert.Equal(t, 0, tableCount(t, db, "migrations"))
	}, "postgres")
}

func TestMigration_WithUseTransactionsShouldRollback(t *testing.T) {
	options := DefaultOptions

	forEachDatabase(t, func(db *sqlx.DB) {
		assert.True(t, true)
		m := New(db, options, failingMigration)

		// Migration should return an error and not leave around a Book table
		err := m.Migrate()
		assert.Error(t, err)
		assert.False(t, m.hasTable("books"))
	}, "postgres")
}

func tableCount(t *testing.T, db *sqlx.DB, tableName string) (count int) {
	query := fmt.Sprintf("SELECT count(0) FROM %s", tableName)
	assert.NoError(t, db.QueryRow(query).Scan(&count))
	return
}

func forEachDatabase(t *testing.T, fn func(database *sqlx.DB), dialects ...string) {
	if len(databases) == 0 {
		panic("No database choosen for testing!")
	}

	for _, database := range databases {
		if len(dialects) > 0 && !contains(dialects, database.name) {
			t.Skip(fmt.Sprintf("test is not supported by [%s] dialect", database.name))
		}

		// Ensure defers are not stacked up for each DB
		func() {
			db, err := sqlx.Open(database.name, os.Getenv(database.connEnv))
			require.NoError(t, err, "Could not connect to database %s, %v", database.name, err)

			defer db.Close()

			// ensure tables do not exists
			assert.NoError(t, dropTableIfExists(db, "migrations", "people", "pets", "animals", "cars"))

			fn(db)
		}()
	}
}

func dropTableIfExists(db *sqlx.DB, tableNames ...string) error {
	for _, n := range tableNames {
		q := fmt.Sprintf("DROP TABLE IF EXISTS %s", n)
		_, err := db.Exec(q)
		if err != nil {
			return err
		}
	}
	return nil
}

func contains(haystack []string, needle string) bool {
	for _, straw := range haystack {
		if straw == needle {
			return true
		}
	}
	return false
}

func (g *Sqlxmigrate) hasTable(tableName string) bool {
	ok, _ := g.HasTable(tableName)
	return ok
}
