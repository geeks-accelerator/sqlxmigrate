package sqlxmigrate

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"log"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

const (
	initSchemaMigrationID = "SCHEMA_INIT"
)

// MigrateFunc is the func signature for migrating.
type MigrateFunc func(*sql.Tx) error

// RollbackFunc is the func signature for rollbacking.
type RollbackFunc func(*sql.Tx) error

// InitSchemaFunc is the func signature for initializing the schema.
type InitSchemaFunc func(*sqlx.DB) error

// Options define options for all migrations.
type Options struct {
	// TableName is the migration table.
	TableName string
	// IDColumnName is the name of column where the migration id will be stored.
	IDColumnName string
	// IDColumnSize is the length of the migration id column
	IDColumnSize int
}

// Migration represents a database migration (a modification to be made on the database).
type Migration struct {
	// ID is the migration identifier. Usually a timestamp like "201601021504".
	ID string
	// Migrate is a function that will br executed while running this migration.
	Migrate MigrateFunc
	// Rollback will be executed on rollback. Can be nil.
	Rollback RollbackFunc
}

// Sqlxmigrate represents a collection of all migrations of a database schema.
type Sqlxmigrate struct {
	db         *sqlx.DB
	tx         *sql.Tx
	options    *Options
	migrations []*Migration
	initSchema InitSchemaFunc
	log        *log.Logger
}

// ReservedIDError is returned when a migration is using a reserved ID
type ReservedIDError struct {
	ID string
}

func (e *ReservedIDError) Error() string {
	return fmt.Sprintf(`sqlxmigrate: Reserved migration ID: "%s"`, e.ID)
}

// DuplicatedIDError is returned when more than one migration have the same ID
type DuplicatedIDError struct {
	ID string
}

func (e *DuplicatedIDError) Error() string {
	return fmt.Sprintf(`sqlxmigrate: Duplicated migration ID: "%s"`, e.ID)
}

var (
	// DefaultOptions can be used if you don't want to think about options.
	DefaultOptions = &Options{
		TableName:    "migrations",
		IDColumnName: "id",
		IDColumnSize: 255,
	}

	// ErrRollbackImpossible is returned when trying to rollback a migration
	// that has no rollback function.
	ErrRollbackImpossible = errors.New("sqlxmigrate: It's impossible to rollback this migration")

	// ErrNoMigrationDefined is returned when no migration is defined.
	ErrNoMigrationDefined = errors.New("sqlxmigrate: No migration defined")

	// ErrMissingID is returned when the ID od migration is equal to ""
	ErrMissingID = errors.New("sqlxmigrate: Missing ID in migration")

	// ErrNoRunMigration is returned when any run migration was found while
	// running RollbackLast
	ErrNoRunMigration = errors.New("sqlxmigrate: Could not find last run migration")

	// ErrMigrationIDDoesNotExist is returned when migrating or rolling back to a migration ID that
	// does not exist in the list of migrations
	ErrMigrationIDDoesNotExist = errors.New("sqlxmigrate: Tried to migrate to an ID that doesn't exist")
)

// New returns a new Sqlxmigrate.
func New(db *sqlx.DB, options *Options, migrations []*Migration) *Sqlxmigrate {
	if options.TableName == "" {
		options.TableName = DefaultOptions.TableName
	}
	if options.IDColumnName == "" {
		options.IDColumnName = DefaultOptions.IDColumnName
	}
	if options.IDColumnSize == 0 {
		options.IDColumnSize = DefaultOptions.IDColumnSize
	}

	l := log.New(os.Stdout, "sqlxmigrate : ", log.LstdFlags|log.Lmicroseconds|log.Lshortfile)

	return &Sqlxmigrate{
		db:         db,
		options:    options,
		migrations: migrations,
		log:        l,
	}
}

// SetLogger allows the default logger to be overwritten
func (g *Sqlxmigrate) SetLogger(logger *log.Logger) {
	g.log = logger
}

// InitSchema sets a function that is run if no migration is found.
// The idea is preventing to run all migrations when a new clean database
// is being migrating. In this function you should create all tables and
// foreign key necessary to your application.
func (g *Sqlxmigrate) InitSchema(initSchema InitSchemaFunc) {
	g.initSchema = initSchema
}

// Migrate executes all migrations that did not run yet.
func (g *Sqlxmigrate) Migrate() error {
	if !g.hasMigrations() {
		return ErrNoMigrationDefined
	}
	var targetMigrationID string
	if len(g.migrations) > 0 {
		targetMigrationID = g.migrations[len(g.migrations)-1].ID
	}
	return g.migrate(targetMigrationID)
}

// MigrateTo executes all migrations that did not run yet up to the migration that matches `migrationID`.
func (g *Sqlxmigrate) MigrateTo(migrationID string) error {
	if err := g.checkIDExist(migrationID); err != nil {
		return err
	}
	return g.migrate(migrationID)
}

// migrate
func (g *Sqlxmigrate) migrate(migrationID string) error {
	if !g.hasMigrations() {
		return ErrNoMigrationDefined
	}

	if err := g.checkReservedID(); err != nil {
		return err
	}

	if err := g.checkDuplicatedID(); err != nil {
		return err
	}

	if err := g.begin(); err != nil {
		return err
	}

	defer g.rollback()

	if err := g.createMigrationTableIfNotExists(); err != nil {
		return err
	}

	if g.initSchema != nil {
		canInitializeSchema, err := g.canInitializeSchema()
		if err != nil {
			return err
		}
		if canInitializeSchema {
			if err := g.runInitSchema(); err != nil {
				return err
			}
			return g.commit()
		}
	}

	for _, migration := range g.migrations {
		if err := g.runMigration(migration); err != nil {
			return err
		}
		if migrationID != "" && migration.ID == migrationID {
			break
		}
	}
	return g.commit()
}

// There are migrations to apply if either there's a defined
// initSchema function or if the list of migrations is not empty.
func (g *Sqlxmigrate) hasMigrations() bool {
	return g.initSchema != nil || len(g.migrations) > 0
}

// Check whether any migration is using a reserved ID.
// For now there's only have one reserved ID, but there may be more in the future.
func (g *Sqlxmigrate) checkReservedID() error {
	for _, m := range g.migrations {
		if m.ID == initSchemaMigrationID {
			return &ReservedIDError{ID: m.ID}
		}
	}
	return nil
}

func (g *Sqlxmigrate) checkDuplicatedID() error {
	lookup := make(map[string]struct{}, len(g.migrations))
	for _, m := range g.migrations {
		if _, ok := lookup[m.ID]; ok {
			return &DuplicatedIDError{ID: m.ID}
		}
		lookup[m.ID] = struct{}{}
	}
	return nil
}

func (g *Sqlxmigrate) checkIDExist(migrationID string) error {
	for _, migrate := range g.migrations {
		if migrate.ID == migrationID {
			return nil
		}
	}
	return ErrMigrationIDDoesNotExist
}

// RollbackLast undo the last migration
func (g *Sqlxmigrate) RollbackLast() error {
	if len(g.migrations) == 0 {
		return ErrNoMigrationDefined
	}

	if err := g.begin(); err != nil {
		return err
	}
	defer g.rollback()

	lastRunMigration, err := g.getLastRunMigration()
	if err != nil {
		return err
	}

	if err := g.rollbackMigration(lastRunMigration); err != nil {
		return err
	}
	return g.commit()
}

// RollbackTo undoes migrations up to the given migration that matches the `migrationID`.
// Migration with the matching `migrationID` is not rolled back.
func (g *Sqlxmigrate) RollbackTo(migrationID string) error {
	if len(g.migrations) == 0 {
		return ErrNoMigrationDefined
	}

	if err := g.checkIDExist(migrationID); err != nil {
		return err
	}

	if err := g.begin(); err != nil {
		return err
	}
	defer g.rollback()

	for i := len(g.migrations) - 1; i >= 0; i-- {
		migration := g.migrations[i]
		if migration.ID == migrationID {
			break
		}
		migrationRan, err := g.migrationRan(migration)
		if err != nil {
			return err
		}
		if migrationRan {
			if err := g.rollbackMigration(migration); err != nil {
				return err
			}
		}
	}
	return g.commit()
}

func (g *Sqlxmigrate) getLastRunMigration() (*Migration, error) {
	for i := len(g.migrations) - 1; i >= 0; i-- {
		migration := g.migrations[i]

		migrationRan, err := g.migrationRan(migration)
		if err != nil {
			return nil, err
		}

		if migrationRan {
			return migration, nil
		}
	}
	return nil, ErrNoRunMigration
}

// RollbackMigration undo a migration.
func (g *Sqlxmigrate) RollbackMigration(m *Migration) error {
	if err := g.begin(); err != nil {
		return err
	}
	defer g.rollback()

	if err := g.rollbackMigration(m); err != nil {
		return err
	}
	return g.commit()
}

func (g *Sqlxmigrate) rollbackMigration(m *Migration) error {
	if m.Rollback == nil {
		return ErrRollbackImpossible
	}
	g.log.Printf("Migration %s rollback", m.ID)

	if err := m.Rollback(g.tx); err != nil {
		return err
	}

	sql := fmt.Sprintf("DELETE FROM %s WHERE %s = ?", g.options.TableName, g.options.IDColumnName)
	sql = g.db.Rebind(sql)
	g.log.Printf("Migration %s rollback - %s", m.ID, sql)

	if _, err := g.tx.Exec(sql, m.ID); err != nil {
		return err
	}

	return nil
}

func (g *Sqlxmigrate) runInitSchema() error {
	if err := g.initSchema(g.db); err != nil {
		return err
	}
	if err := g.insertMigration(initSchemaMigrationID); err != nil {
		return err
	}

	for _, migration := range g.migrations {
		if err := g.runMigration(migration); err != nil {
			return err
		}
	}

	return nil
}

func (g *Sqlxmigrate) runMigration(migration *Migration) error {
	if len(migration.ID) == 0 {
		return ErrMissingID
	}
	g.log.Printf("Migration %s - checking", migration.ID)

	migrationRan, err := g.migrationRan(migration)
	if err != nil {
		return err
	}
	if migrationRan {
		g.log.Printf("Migration %s - already ran", migration.ID)
	} else {
		g.log.Printf("Migration %s - starting", migration.ID)

		if err := migration.Migrate(g.tx); err != nil {
			g.log.Printf("Migration %s - failed - %v", migration.ID, err)

			if rerr := migration.Rollback(g.tx); rerr != nil {
				if strings.Contains(rerr.Error(), "current transaction is aborted") {
					g.log.Printf("Migration %s - Rollback skipped, transaction is aborted", migration.ID)
				} else {
					g.log.Printf("Migration %s - Rollback failed - %v", migration.ID, rerr)
				}
			}

			return err
		}

		if err := g.insertMigration(migration.ID); err != nil {
			return err
		}

		g.log.Printf("Migration %s - complete", migration.ID)
	}
	return nil
}

func (g *Sqlxmigrate) createMigrationTableIfNotExists() error {
	if ok, err := g.HasTable(g.options.TableName); ok || err != nil {
		return err
	}

	sql := fmt.Sprintf("CREATE TABLE %s (%s VARCHAR(%d) PRIMARY KEY)", g.options.TableName, g.options.IDColumnName, g.options.IDColumnSize)
	g.log.Printf("createMigrationTableIfNotExists %s", sql)

	if _, err := g.db.Exec(sql); err != nil {
		err = errors.WithMessagef(err, "Query failed %s", sql)
		return err
	}
	return nil
}

func (g *Sqlxmigrate) migrationRan(m *Migration) (bool, error) {
	var count int

	query := fmt.Sprintf("SELECT count(0) FROM %s WHERE %s = ?", g.options.TableName, g.options.IDColumnName)
	query = g.db.Rebind(query)
	g.log.Printf("Migration %s - %s", m.ID, query)

	err := g.db.QueryRow(query, m.ID).Scan(&count)
	if err != nil {
		err = errors.WithMessagef(err, "Query failed %s", query)
		return false, err
	}

	return count > 0, err
}

// The schema can be initialised only if it hasn't been initialised yet
// and no other migration has been applied already.
func (g *Sqlxmigrate) canInitializeSchema() (bool, error) {
	migrationRan, err := g.migrationRan(&Migration{ID: initSchemaMigrationID})
	if err != nil {
		return false, err
	}
	if migrationRan {
		return false, nil
	}

	// If the ID doesn't exist, we also want the list of migrations to be empty
	var count int
	query := fmt.Sprintf("SELECT count(0) FROM %s", g.options.TableName)
	g.log.Printf("canInitializeSchema %s", query)

	err = g.db.QueryRow(query).Scan(&count)
	if err != nil {
		err = errors.WithMessagef(err, "Query failed %s", query)
		return false, err
	}

	return count == 0, err
}

func (g *Sqlxmigrate) insertMigration(id string) error {
	sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (?)", g.options.TableName, g.options.IDColumnName)
	sql = g.db.Rebind(sql)
	g.log.Printf("Migration %s - %s", id, sql)

	if _, err := g.db.Exec(sql, id); err != nil {
		err = errors.WithMessagef(err, "Query failed %s", sql)
		return err
	}

	return nil
}

func (g *Sqlxmigrate) begin() error {
	var err error
	g.tx, err = g.db.Begin()
	return err
}

func (g *Sqlxmigrate) commit() error {
	err := g.tx.Commit()
	g.tx = nil
	return err
}

func (g *Sqlxmigrate) rollback() {
	if g.tx != nil {
		g.tx.Rollback()
		g.log.Printf("tx.rollback executed")
		g.tx = nil
	}
}

func (g *Sqlxmigrate) HasTable(tableName string) (bool, error) {
	query := fmt.Sprintf("SELECT 1 FROM %s", tableName)
	g.log.Printf("HasTable %s - %s", tableName, query)

	if _, err := g.db.Exec(query); err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			// postgres error
			return false, nil
		} else if strings.Contains(err.Error(), "doesn't exist") {
			// mysql table error
			return false, nil
		}
		return false, err
	}
	return true, nil
}
