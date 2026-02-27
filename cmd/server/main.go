package main

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/boeing/go-gls-test/internal/cache"
	"github.com/boeing/go-gls-test/internal/config"
	"github.com/boeing/go-gls-test/internal/handler"
	"github.com/boeing/go-gls-test/internal/model"
	"github.com/boeing/go-gls-test/internal/repository"
	"github.com/boeing/go-gls-test/internal/service"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"
)

func main() {
	cfg := config.Load()

	// Connect to PostgreSQL
	db, err := sql.Open("postgres", cfg.DSN())
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(cfg.DBMaxOpenConns)
	db.SetMaxIdleConns(cfg.DBMaxIdleConns)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Wait for database to be ready
	for i := 0; i < 30; i++ {
		if err := db.Ping(); err == nil {
			break
		}
		log.Println("Waiting for database...")
		time.Sleep(time.Second)
	}
	if err := db.Ping(); err != nil {
		log.Fatalf("Database not ready: %v", err)
	}
	log.Println("Connected to PostgreSQL")

	// Connect to Redis
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	defer rdb.Close()

	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	log.Println("Connected to Redis")

	// Initialize layers
	repo := repository.New(db)
	modelClient := model.New(repo)
	cacheLayer := cache.New(rdb)
	svc := service.New(repo, modelClient, cacheLayer)
	h := handler.New(svc)

	// Setup Fiber
	app := fiber.New(fiber.Config{
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	})

	app.Use(logger.New())
	app.Use(recover.New())

	// Register routes
	h.RegisterRoutes(app)

	// Health check
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	// Setup cron job for cache warming (every 30 minutes, starts after 30 min)
	c := cron.New()
	_, err = c.AddFunc("@every 30m", func() {
		log.Println("Cron: warming batch cache...")
		svc.WarmBatchCache(context.Background())
	})
	if err != nil {
		log.Fatalf("Failed to add cron job: %v", err)
	}

	// Delay the first cache warming by 30 minutes
	go func() {
		time.Sleep(30 * time.Minute)
		log.Println("Initial cache warming trigger after 30 minutes...")
		svc.WarmBatchCache(context.Background())
	}()
	c.Start()
	defer c.Stop()

	// Start server
	log.Printf("Starting server on :%s", cfg.ServerPort)
	if err := app.Listen(":" + cfg.ServerPort); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
