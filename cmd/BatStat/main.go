package main

import (
	"log"

	"github.com/Mrbrooks89/BatStat/internal/tui"
)

func main() {
	app := tui.NewApp()
	if err := app.Run(); err != nil {
		log.Fatalf("failed to start app: %v", err)
	}
}
