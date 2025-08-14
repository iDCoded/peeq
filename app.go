package main

import (
	"context"
	"log"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/joho/godotenv"
)

type Product struct {
	gorm.Model
	Code  string
	Price uint
}

// App struct
type App struct {
	ctx context.Context
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	log.Println("[Peeq] Initialized application")
	connectToDB("mydb")
	a.ctx = ctx
}

func connectToDB(connString string) {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error accessing .env file")
	}

	log.Println("[DB] Establishing connection...", connString)

	dsn := os.Getenv("DSN")
	log.Println(dsn)
	if dsn == "" {
		log.Fatal("[DB] No data source name provided")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("[DB] Failed to connect to database")
	}

	ctx := context.Background()

	db.AutoMigrate(&Product{})

	err = gorm.G[Product](db).Create(ctx, &Product{Code: "K31", Price: 100})
	if err != nil {
		log.Fatal("[DB] Error creating the product")
	}

	product, err := gorm.G[Product](db).Where("id = ?", 1).First(ctx)
	if err != nil {
		log.Fatal("[DB] Error finding the product")
	}

	log.Println(product)
}
