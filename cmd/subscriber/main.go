package main

import (
	gosdk "brocker/sdk/go"
	"context"
	"log"
	"os"
	"time"
)

func main() {
	baseURL := getenv("BROKER_URL", "http://broker:8080")
	stream := getenv("STREAM_NAME", "events")
	name := getenv("SUBSCRIBER_NAME", "subscriber-1")
	stype := getenv("STREAM_TYPE", "queue")

	client := gosdk.New(baseURL, 5*time.Second)
	ctx := context.Background()
	if err := client.CreateStream(ctx, stream, stype, "fifo"); err != nil {
		log.Printf("create stream: %v", err)
	}
	if err := client.Subscribe(ctx, stream, name); err != nil {
		log.Fatalf("subscribe: %v", err)
	}

	for {
		msgs, err := client.Pull(ctx, stream, name, 1)
		if err != nil {
			log.Printf("pull error: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}
		if len(msgs) == 0 {
			time.Sleep(700 * time.Millisecond)
			continue
		}
		for _, msg := range msgs {
			log.Printf("[%s] received: id=%s payload=%s priority=%d", name, msg.ID, msg.Payload, msg.Priority)
			if err := client.Ack(ctx, stream, name, msg.ID); err != nil {
				log.Printf("[%s] ack error: %v", name, err)
			}
		}
	}
}

func getenv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
