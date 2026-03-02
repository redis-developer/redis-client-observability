// EXAMPLE: otel_metrics
// HIDE_START
package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	redisotel "github.com/redis/go-redis/extra/redisotel-native/v9"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

// ExampleClient_otel_metrics demonstrates how to enable OpenTelemetry metrics
// for Redis operations and export them to an OTLP collector.
func main() {
	// Create context that can be cancelled with Ctrl+C
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Create OTLP exporter that sends metrics to the collector
	// Default endpoint is localhost:4317 (gRPC)
	exporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithInsecure(), // Use insecure for local development
		// For production, configure TLS and authentication:
		// otlpmetricgrpc.WithEndpoint("your-collector:4317"),
		// otlpmetricgrpc.WithTLSCredentials(...),
	)
	if err != nil {
		log.Fatalf("Failed to create OTLP exporter: %v", err)
	}

	// Create resource with service name
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(fmt.Sprintf("go-redis-examples:%d", time.Now().Unix())),
		),
	)
	if err != nil {
		log.Fatalf("Failed to create resource: %v", err)
	}

	// Create meter provider with periodic reader
	// Metrics are exported every 10 seconds
	meterProvider := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(
			metric.NewPeriodicReader(exporter,
				metric.WithInterval(10*time.Second),
			),
		),
	)
	defer func() {
		if err := meterProvider.Shutdown(context.Background()); err != nil {
			log.Printf("Error shutting down meter provider: %v", err)
		}
	}()

	// Set the global meter provider
	otel.SetMeterProvider(meterProvider)

	// Create Redis client
	// Initialize OTel instrumentation BEFORE creating Redis clients
	otelInstance := redisotel.GetObservabilityInstance()
	config := redisotel.NewConfig().WithEnabled(true).WithMetricGroups(redisotel.MetricGroupAll)
	if err := otelInstance.Init(config); err != nil {
		log.Fatalf("Failed to initialize OTel: %v", err)
	}
	defer otelInstance.Shutdown()

	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	defer rdb.Close()

	// Execute Redis operations continuously - metrics are automatically collected
	log.Println("Starting Redis operations... Press Ctrl+C to stop")

	var wg sync.WaitGroup

	// Goroutine 1: GET, SET, and HSET operations
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Println("Worker 1: Starting GET/SET/HSET operations")
		counter := 0
		for {
			select {
			case <-ctx.Done():
				log.Println("Worker 1: Shutting down")
				return
			default:
				counter++

				// SET operation
				key := "key:" + strconv.Itoa(counter%100)
				if err := rdb.Set(ctx, key, "value-"+strconv.Itoa(counter), 0).Err(); err != nil {
					log.Printf("Worker 1: Error setting key: %v", err)
				}

				// GET operation
				if _, err := rdb.Get(ctx, key).Result(); err != nil && err != redis.Nil {
					log.Printf("Worker 1: Error getting key: %v", err)
				}

				// HSET operation
				hashKey := "hash:" + strconv.Itoa(counter%50)
				if err := rdb.HSet(ctx, hashKey, "field"+strconv.Itoa(counter%10), "value-"+strconv.Itoa(counter)).Err(); err != nil {
					log.Printf("Worker 1: Error setting hash: %v", err)
				}

				// HGET operation
				if _, err := rdb.HGet(ctx, hashKey, "field"+strconv.Itoa(counter%10)).Result(); err != nil && err != redis.Nil {
					log.Printf("Worker 1: Error getting hash field: %v", err)
				}

				time.Sleep(time.Millisecond * time.Duration(100+rand.Intn(200)))
			}
		}
	}()

	// Goroutine 2: Stream producer (XADD)
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Println("Worker 2: Starting stream producer")
		streamName := "mystream"
		counter := 0
		for {
			select {
			case <-ctx.Done():
				log.Println("Worker 2: Shutting down")
				return
			default:
				counter++
				if err := rdb.XAdd(ctx, &redis.XAddArgs{
					Stream: streamName,
					Values: map[string]interface{}{
						"message":   "stream-message-" + strconv.Itoa(counter),
						"timestamp": time.Now().Unix(),
						"counter":   counter,
					},
				}).Err(); err != nil {
					log.Printf("Worker 2: Error adding to stream: %v", err)
				}

				time.Sleep(time.Millisecond * time.Duration(200+rand.Intn(300)))
			}
		}
	}()

	// Goroutine 3: Stream consumer (XREADGROUP with ACK)
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Println("Worker 3: Starting stream consumer")
		streamName := "mystream"
		groupName := "mygroup"
		consumerName := "consumer-1"

		// Create consumer group (ignore error if already exists)
		rdb.XGroupCreateMkStream(ctx, streamName, groupName, "0")

		for {
			select {
			case <-ctx.Done():
				log.Println("Worker 3: Shutting down")
				return
			default:
				// Read from stream using consumer group
				streams, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
					Group:    groupName,
					Consumer: consumerName,
					Streams:  []string{streamName, ">"},
					Count:    10,
					Block:    time.Second,
				}).Result()

				if err != nil && err != redis.Nil {
					log.Printf("Worker 3: Error reading from stream: %v", err)
					time.Sleep(time.Second)
					continue
				}

				// Process and acknowledge messages
				for _, stream := range streams {
					for _, message := range stream.Messages {
						log.Printf("Worker 3: Processing message ID %s: %v", message.ID, message.Values)

						// Acknowledge the message
						if err := rdb.XAck(ctx, streamName, groupName, message.ID).Err(); err != nil {
							log.Printf("Worker 3: Error acknowledging message: %v", err)
						}
					}
				}
			}
		}
	}()

	// Goroutine 4: Pub/Sub publisher
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Println("Worker 4: Starting pub/sub publisher")
		counter := 0
		for {
			select {
			case <-ctx.Done():
				log.Println("Worker 4: Shutting down")
				return
			default:
				counter++

				// Publish to different channels
				channels := []string{"channel1", "channel2", "notifications"}
				channel := channels[counter%len(channels)]

				message := "message-" + strconv.Itoa(counter) + "-" + time.Now().Format("15:04:05")
				if err := rdb.Publish(ctx, channel, message).Err(); err != nil {
					log.Printf("Worker 4: Error publishing to %s: %v", channel, err)
				}

				time.Sleep(time.Millisecond * time.Duration(300+rand.Intn(400)))
			}
		}
	}()

	// Goroutine 5: Pub/Sub subscriber
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Println("Worker 5: Starting pub/sub subscriber")

		pubsub := rdb.Subscribe(ctx, "channel1", "channel2", "notifications")
		defer pubsub.Close()

		ch := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				log.Println("Worker 5: Shutting down")
				return
			case msg := <-ch:
				if msg != nil {
					log.Printf("Worker 5: Received on %s: %s", msg.Channel, msg.Payload)
				}
			}
		}
	}()

	// Goroutine 6: Error generator - intentionally produces various Redis errors
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Println("Worker 6: Starting error generator")
		counter := 0

		// Set up some keys with specific types for error generation
		rdb.Set(ctx, "string-key", "I am a string", 0)
		rdb.SAdd(ctx, "set-key", "member1", "member2")
		rdb.LPush(ctx, "list-key", "item1", "item2")

		for {
			select {
			case <-ctx.Done():
				log.Println("Worker 6: Shutting down")
				return
			default:
				counter++
				errorType := counter % 5

				switch errorType {
				case 0:
					// WRONGTYPE error: Try to HSET on a string key
					if err := rdb.HSet(ctx, "string-key", "field", "value").Err(); err != nil {
						log.Printf("Worker 6: Expected WRONGTYPE error: %v", err)
					}

				case 1:
					// WRONGTYPE error: Try to LPUSH on a set
					if err := rdb.LPush(ctx, "set-key", "value").Err(); err != nil {
						log.Printf("Worker 6: Expected WRONGTYPE error: %v", err)
					}

				case 2:
					// WRONGTYPE error: Try to SADD on a list
					if err := rdb.SAdd(ctx, "list-key", "member").Err(); err != nil {
						log.Printf("Worker 6: Expected WRONGTYPE error: %v", err)
					}

				case 3:
					// Invalid argument error: GETRANGE with invalid indices
					if err := rdb.GetRange(ctx, "string-key", -1000, -2000).Err(); err != nil {
						log.Printf("Worker 6: Error from invalid range: %v", err)
					}

				case 4:
					// Stream error: Try to read from non-existent consumer group
					if _, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
						Group:    "nonexistent-group",
						Consumer: "consumer",
						Streams:  []string{"mystream", ">"},
						Count:    1,
						Block:    100 * time.Millisecond,
					}).Result(); err != nil && err != redis.Nil {
						log.Printf("Worker 6: Expected NOGROUP error: %v", err)
					}
				}

				time.Sleep(time.Millisecond * time.Duration(500+rand.Intn(1000)))
			}
		}
	}()

	// Wait for all workers to finish
	wg.Wait()
	log.Println("All workers stopped")
}
