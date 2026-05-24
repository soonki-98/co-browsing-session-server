package main

import "co-browsing-session-server/internal/app"

func main() {
	application := app.NewApp()
	application.Run(":8080")
}
