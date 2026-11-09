package main

import (
	"backend/schema"
	"database/sql"
	"log"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB

func InitDB() {
	var err error
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./database.db"
	}
	db, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatal(err)
	}

	if err := schema.Apply(db); err != nil {
		log.Fatal("Failed to execute database schema:", err)
	}

	log.Println("Database initialized successfully")
}

func GetDB() *sql.DB {
	return db
}
