package main

import (
	"brocker/internal/broker"
	"brocker/internal/httpapi"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

func main() {
	dataDir := getenv("DATA_DIR", "./data")
	addr := getenv("HTTP_ADDR", ":8080")
	ackTimeoutSec, _ := strconv.Atoi(getenv("ACK_TIMEOUT_SEC", "15"))
	maxDeliveries, _ := strconv.Atoi(getenv("MAX_DELIVERIES", "3"))

	b, err := broker.New(dataDir, time.Duration(ackTimeoutSec)*time.Second, maxDeliveries)
	if err != nil {
		log.Fatalf("init broker: %v", err)
	}
	defer b.Close()

	h := httpapi.NewHandler(b)
	log.Printf("broker started on %s", addr)
	if err := http.ListenAndServe(addr, h.Routes()); err != nil {
		log.Fatalf("listen and serve: %v", err)
	}
}

func getenv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
