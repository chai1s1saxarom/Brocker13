package main

import (
	gosdk "brocker/sdk/go"
	"context"
	"log"
	"os"
	"strconv"
	"time"
)

func main() {
	baseURL := getenv("BROKER_URL", "http://broker:8080")
	stream := getenv("STREAM_NAME", "events")
	stype := getenv("STREAM_TYPE", "queue")
	mode := getenv("QUEUE_MODE", "fifo")
	intervalSec, _ := strconv.Atoi(getenv("INTERVAL_SEC", "2"))

	client := gosdk.New(baseURL, 5*time.Second)
	ctx := context.Background()
	if err := client.CreateStream(ctx, stream, stype, mode); err != nil {
		log.Printf("create stream: %v", err)
	}

	i := 1
	for {
		payload := "event #" + strconv.Itoa(i)
		msg, err := client.Publish(ctx, stream, payload, i%3, 90)
		if err != nil {
			log.Printf("publish error: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}
		log.Printf("published: id=%s payload=%s", msg.ID, msg.Payload)
		i++
		time.Sleep(time.Duration(intervalSec) * time.Second)
	}
}

func getenv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
