// Package sqlxmigrate is a migration helper for sqlx (https://github.com/jmoiron/sqlx/).
// Enables schema versioning and rollback cababilities.
//
// Example:
//
// package main
//
// import (
// 	"database/sql"
// 	"log"
//
// 	"github.com/geeks-accelerator/sqlxmigrate"
// 	"github.com/jmoiron/sqlx"
// 	_ "github.com/lib/pq"
// )
//
// func main() {
// 	// this Pings the database trying to connect, panics on error
// 	// use sqlx.Open() for sql.Open() semantics
// 	db, err := sqlx.Connect("postgres", "host=127.0.0.1 user=postgres dbname=sqlxmigrate_test port=5433 sslmode=disable password=postgres")
// 	if err != nil {
// 		log.Fatalf("main : Register DB : %v", err)
// 	}
// 	defer db.Close()
//
// 	m := sqlxmigrate.New(db, sqlxmigrate.DefaultOptions, []*sqlxmigrate.Migration{
// 		// create persons table
// 		{
// 			ID: "201608301400",
// 			Migrate: func(tx *sql.Tx) error {
// 				q := `CREATE TABLE "people" (
// 						"id" serial,
// 						"created_at" timestamp with time zone,
// 						"updated_at" timestamp with time zone,
// 						"deleted_at" timestamp with time zone,
// 						"name" text ,
// 						PRIMARY KEY ("id")
// 					)`
// 				_, err = tx.Exec(q)
// 				return err
// 			},
// 			Rollback: func(tx *sql.Tx) error {
// 				q := `DROP TABLE IF EXISTS people`
// 				_, err = tx.Exec(q)
// 				return err
// 			},
// 		},
// 		// add age column to persons
// 		{
// 			ID: "201608301415",
// 			Migrate: func(tx *sql.Tx) error {
// 				q := `ALTER TABLE people
// 						ADD column age int
// 					`
// 				_, err = tx.Exec(q)
// 				return err
// 			},
// 			Rollback: func(tx *sql.Tx) error {
// 				q := `ALTER TABLE people
// 						DROP column age`
// 				_, err = tx.Exec(q)
// 				return err
// 			},
// 		},
// 	})
//
// 	if err = m.Migrate(); err != nil {
// 		log.Fatalf("Could not migrate: %v", err)
// 	}
// 	log.Printf("Migration did run successfully")
// }
//
package sqlxmigrate
