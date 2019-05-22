// Package sqlxmigrate is a migration helper for Gorm (http://jinzhu.me/gorm/).
// Gorm already have useful migrate functions
// (http://jinzhu.me/gorm/database.html#migration), just misses
// proper schema versioning and rollback cababilities.
//
// Example:
//
//  package main
//
//  import (
//      "log"
//
//      "github.com/go-sqlxmigrate/sqlxmigrate"
//      "github.com/jmoiron/sqlx"
//      _ "github.com/lib/pq"
//  )
//
//  type Person struct {
//      gorm.Model
//      Name string
//  }
//
//  type Pet struct {
//      gorm.Model
//      Name     string
//      PersonID int
//  }
//
//  func main() {
//      db, err := gorm.Open("postgres", "mydb.postgres")
//      if err != nil {
//          log.Fatal(err)
//      }
//      if err = db.DB().Ping(); err != nil {
//          log.Fatal(err)
//      }
//
//      db.LogMode(true)
//
//      m := sqlxmigrate.New(db, sqlxmigrate.DefaultOptions, []*sqlxmigrate.Migration{
//          {
//              ID: "201608301400",
//              Migrate: func(tx *gorm.DB) error {
//                  return tx.AutoMigrate(&Person{}).Error
//              },
//              Rollback: func(tx *gorm.DB) error {
//                  return tx.DropTable("people").Error
//              },
//          },
//          {
//              ID: "201608301430",
//              Migrate: func(tx *gorm.DB) error {
//                  return tx.AutoMigrate(&Pet{}).Error
//              },
//              Rollback: func(tx *gorm.DB) error {
//                  return tx.DropTable("pets").Error
//              },
//          },
//      })
//
//      err = m.Migrate()
//      if err == nil {
//          log.Printf("Migration did run successfully")
//      } else {
//          log.Printf("Could not migrate: %v", err)
//      }
//  }
package sqlxmigrate
