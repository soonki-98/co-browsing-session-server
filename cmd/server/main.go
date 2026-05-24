package main

import (
	"log"

	"co-browsing-session-server/internal/app"
)

func main() {
	if err := app.New().Run(":8080"); err != nil {
		log.Fatalf("server: %v", err)
	}
}
