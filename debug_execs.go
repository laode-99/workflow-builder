package main

import (
	"fmt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Execution struct {
	ID         string
	Status     string
	CreatedAt  string
}

func main() {
	dsn := "host=127.0.0.1 user=workflow password=workflow_password dbname=workflow_engine port=5434 sslmode=disable"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		fmt.Println("Err DB:", err)
		return
	}

	var execs []Execution
	db.Table("executions").Order("created_at desc").Limit(5).Scan(&execs)
	
	fmt.Println("Latest 5 Executions:")
	for _, e := range execs {
		fmt.Printf("ID: %s | Status: %s | CreatedAt: %s\n", e.ID, e.Status, e.CreatedAt)
	}
}
