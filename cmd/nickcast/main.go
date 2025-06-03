package main

import (
    "fmt"
    "log"
    "nickcast/config"
    "nickcast/internal/server"
)

func main() {
    err := config.LoadConfig()
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }

    fmt.Println("Starting stream server on", config.AppConfig.ListenAddress)
    server.Start()
}
