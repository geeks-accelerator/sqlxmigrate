# SqlxMigrate

[![GoDoc](https://godoc.org/github.com/gitwak/sqlxmigrate?status.svg)](https://godoc.org/github.com/gitwak/sqlxmigrate)
[![Go Report Card](https://goreportcard.com/badge/github.com/gitwak/sqlxmigrate)](https://goreportcard.com/report/github.com/gitwak/sqlxmigrate)
[![Build Status](https://travis-ci.org/go-sqlxmigrate/sqlxmigrate.svg?branch=master)](https://travis-ci.org/go-sqlxmigrate/sqlxmigrate)

sqlxmigrate is a minimalistic database schema migration helper for sqlx. 

This project was inspired by github.com/GuiaBolso/darwin as it provides simple approach for handling 
database schema. This package however only supports direct SQL statements for each migration and thus 
unable to handle complex migrations and does not support persisting which migrations have been executed.

The project `gormigrate` github.com/go-gormigrate/gormigrate provides schema migration for the Gorm ORM. 
While GORM has is place, there was an opportunity to replace GORM with the sqlx database package to provide 
a more flexiable schema migration tool. 


## Supported databases

It supports any of the [databases sqlx supports]:

- PostgreSQL


### Mysql support:
Need to determine a plan to abstract the database logic for creating and selecting tables that currently is buried in sqlxmigrate_test.go



## Installing

```bash
go get -u github.com/gitwak/sqlxmigrate
```

## Usage

```go
package main

import (
	"log"

	"github.com/gitwak/sqlxmigrate"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

func main() {
	db, err := gorm.Open("postgres", "mydb.postgres")
	if err != nil {
		log.Fatal(err)
	}

	db.LogMode(true)

	m := sqlxmigrate.New(db, sqlxmigrate.DefaultOptions, []*sqlxmigrate.Migration{
		// create persons table
		{
			ID: "201608301400",
			Migrate: func(tx *gorm.DB) error {
				// it's a good pratice to copy the struct inside the function,
				// so side effects are prevented if the original struct changes during the time
				type Person struct {
					gorm.Model
					Name string
				}
				return tx.AutoMigrate(&Person{}).Error
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.DropTable("people").Error
			},
		},
		// add age column to persons
		{
			ID: "201608301415",
			Migrate: func(tx *gorm.DB) error {
				// when table already exists, it just adds fields as columns
				type Person struct {
					Age int
				}
				return tx.AutoMigrate(&Person{}).Error
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.Table("people").DropColumn("age").Error
			},
		},
		// add pets table
		{
			ID: "201608301430",
			Migrate: func(tx *gorm.DB) error {
				type Pet struct {
					gorm.Model
					Name     string
					PersonID int
				}
				return tx.AutoMigrate(&Pet{}).Error
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.DropTable("pets").Error
			},
		},
	})

	if err = m.Migrate(); err != nil {
		log.Fatalf("Could not migrate: %v", err)
	}
	log.Printf("Migration did run successfully")
}
```

## Having a separated function for initializing the schema

If you have a lot of migrations, it can be a pain to run all them, as example,
when you are deploying a new instance of the app, in a clean database.
To prevent this, you can set a function that will run if no migration was run
before (in a new clean database). Remember to create everything here, all tables,
foreign keys and what more you need in your app.

```go
type Person struct {
	gorm.Model
	Name string
	Age int
}

type Pet struct {
	gorm.Model
	Name     string
	PersonID int
}

m := sqlxmigrate.New(db, sqlxmigrate.DefaultOptions, []*sqlxmigrate.Migration{
    // you migrations here
})

m.InitSchema(func(tx *gorm.DB) error {
	err := tx.AutoMigrate(
		&Person{},
		&Pet{},
		// all other tables of you app
	).Error
	if err != nil {
		return err
	}

	if err := tx.Model(Pet{}).AddForeignKey("person_id", "people (id)", "RESTRICT", "RESTRICT").Error; err != nil {
		return err
	}
	// all other foreign keys...
	return nil
})
```

## Options

This is the options struct, in case you don't want the defaults:

```go
type Options struct {
	// Migrations table name. Default to "migrations".
	TableName string
	// The name of the column that stores the ID of migrations. Defaults to "id".
	IDColumnName string
	// UseTransaction makes SqlxMigrate execute migrations inside a single transaction.
	// Keep in mind that not all databases support DDL commands inside transactions.
	// Defaults to false.
	UseTransaction bool
}
```

## Contributing

To run tests, first copy `.sample.env` as `sample.env` and edit the connection
string of the database you want to run tests against. Then, run tests like
below:

```bash
# running tests for PostgreSQL
export PG_CONN_STRING="host=127.0.0.1 user=postgres dbname=sqlxmigrate_test port=5433 sslmode=disable password=postgres"
go test -tags postgresql

# running test for MySQL
go test -tags mysql

# running test for multiple databases at once
go test -tags 'postgresql mysql'
```

Or altenatively, you could use Docker to easily run tests on all databases
at once. To do that, make sure Docker is installed and running in your machine
and then run:

```bash
task docker
```

Or Manually execute docker

```bash
# Build the test container
docker build -t sqlxmigrate .

# Ensure previous containers have stopped
docker-compose down -v

# Run the tests:
docker-compose run sqlxmigrate go test -v -tags 'postgresql'
```
