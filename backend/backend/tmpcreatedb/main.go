package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"
)

func main() {
	dsn := "host=127.0.0.1 port=5432 user=postgres password=Nsi!7s97sdh(2js dbname=postgres sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Println("open error:", err)
		os.Exit(1)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		fmt.Println("ping error:", err)
		os.Exit(1)
	}
	var exists bool
	if err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname='sub2api')").Scan(&exists); err != nil {
		fmt.Println("query error:", err)
		os.Exit(1)
	}
	if exists {
		fmt.Println("database 'sub2api' already exists")
		return
	}
	if _, err := db.Exec("CREATE DATABASE sub2api"); err != nil {
		fmt.Println("create error:", err)
		os.Exit(1)
	}
	fmt.Println("database 'sub2api' created")
}
