package main

import (
	"fmt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Execution struct {
	ID         string
	ExternalID string
	Status     string
}

func main() {
	dsn := "host=127.0.0.1 user=workflow password=workflow_password dbname=workflow_engine port=5434 sslmode=disable"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		fmt.Println("Err DB:", err)
		return
	}

	var exec Execution
	err = db.Table("executions").Order("created_at desc").First(&exec).Error
	if err != nil {
		fmt.Println("Err query:", err)
		return
	}
	
	fmt.Printf("Latest Execution: ID=%s | ExternalID='%s' | Status=%s\n", exec.ID, exec.ExternalID, exec.Status)
}
