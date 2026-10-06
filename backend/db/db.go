package db

import (
	"fmt"
	"log"
	"os"
	"time"

	"indico-backend/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func InitDB() *gorm.DB {
	host := os.Getenv("DB_HOST")
	if host == "" {
		host = "localhost"
	}
	user := os.Getenv("DB_USER")
	if user == "" {
		user = "indico_user"
	}
	password := os.Getenv("DB_PASSWORD")
	if password == "" {
		password = "indico_password"
	}
	dbname := os.Getenv("DB_NAME")
	if dbname == "" {
		dbname = "indico_db"
	}
	port := os.Getenv("DB_PORT")
	if port == "" {
		port = "5432"
	}

	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=UTC", host, user, password, dbname, port)
	
	var db *gorm.DB
	var err error
	
	// Retry connection since DB might take some time to start up in Docker
	for i := 0; i < 5; i++ {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			break
		}
		log.Printf("Failed to connect to database, retrying in 2 seconds... (error: %v)", err)
		time.Sleep(2 * time.Second)
	}

	if err != nil {
		log.Fatalf("Failed to connect to database after 5 attempts: %v", err)
	}

	return db
}

func SeedData(db *gorm.DB) {
	items := []models.InventoryItem{
		{ItemID: "item_4021", TotalStock: 100, ReservedStock: 0, AvailableStock: 100, Version: 1},
		{ItemID: "item_8888", TotalStock: 50, ReservedStock: 0, AvailableStock: 50, Version: 1},
		{ItemID: "item_9999", TotalStock: 200, ReservedStock: 0, AvailableStock: 200, Version: 1},
	}

	for _, item := range items {
		var count int64
		db.Model(&models.InventoryItem{}).Where("item_id = ?", item.ItemID).Count(&count)
		if count == 0 {
			db.Create(&item)
			log.Printf("Seeded initial inventory item: %s\n", item.ItemID)
		}
	}
}
