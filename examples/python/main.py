#!/usr/bin/env python3
"""
Redis OpenTelemetry Metrics Example (Python)

This example demonstrates how to use redis-py with OpenTelemetry metrics.
It performs various Redis operations and exports metrics to an OTLP collector.

Prerequisites:
    - Redis server running on localhost:6379
    - OpenTelemetry Collector running on localhost:4318 (HTTP)
    - Install dependencies: pip install -r requirements.txt

Usage:
    python main.py
    python main.py --host localhost --port 6379
"""

import argparse
import time
import sys
from typing import Optional

# Set up OpenTelemetry before importing redis
from opentelemetry import metrics
from opentelemetry.sdk.metrics import MeterProvider
from opentelemetry.sdk.metrics.export import PeriodicExportingMetricReader
from opentelemetry.exporter.otlp.proto.http.metric_exporter import OTLPMetricExporter
from opentelemetry.sdk.resources import Resource

import redis
from redis.observability.providers import get_observability_instance
from redis.observability.config import OTelConfig, MetricGroup


def setup_otel(export_interval_ms: int = 5000, endpoint: Optional[str] = None):
    """Set up OpenTelemetry with HTTP exporter."""
    if endpoint is None:
        endpoint = "http://localhost:4318/v1/metrics"

    # Create resource with service name
    resource = Resource.create({
        "service.name": "redis-py-metrics-generator-" + str(time.time()),
        "service.version": "1.0.0",
    })

    exporter = OTLPMetricExporter(endpoint=endpoint)
    reader = PeriodicExportingMetricReader(exporter, export_interval_millis=export_interval_ms)
    provider = MeterProvider(resource=resource, metric_readers=[reader])
    metrics.set_meter_provider(provider)
    
    # Initialize redis-py observability with ALL metric groups
    otel = get_observability_instance()
    otel.init(OTelConfig(
        metric_groups=[
            MetricGroup.COMMAND,
            MetricGroup.PUBSUB,
            MetricGroup.STREAMING,
            MetricGroup.CSC,
            MetricGroup.CONNECTION_BASIC,
            MetricGroup.CONNECTION_ADVANCED,
            MetricGroup.RESILIENCY,
        ]
    ))
    
    print(f"✓ OTel configured to export to {endpoint} every {export_interval_ms}ms")
    print("✓ All metric groups enabled")


def run_redis_operations(host: str = "localhost", port: int = 6379):
    """Execute various Redis operations to generate metrics."""
    print(f"\n{'='*60}")
    print("Starting Redis operations")
    print(f"Redis: {host}:{port}")
    print("Press Ctrl+C to stop")
    print(f"{'='*60}\n")
    
    # Create Redis client
    client = redis.Redis(host=host, port=port, decode_responses=True)
    
    # Create PubSub client
    pubsub_client = redis.Redis(host=host, port=port, decode_responses=True)
    pubsub = pubsub_client.pubsub()
    pubsub.subscribe("metrics_test_channel")
    pubsub.get_message(timeout=1.0)  # Consume subscription confirmation
    
    # Create stream and consumer group
    stream_name = "metrics_test_stream"
    consumer_group = "metrics_test_group"
    consumer_name = "metrics_generator"
    
    try:
        client.xgroup_create(stream_name, consumer_group, id="0", mkstream=True)
    except Exception:
        pass  # Group may already exist
    
    # Try to set up CSC client
    csc_client = None
    try:
        from redis.cache import CacheConfig
        csc_client = redis.Redis(
            host=host, port=port, decode_responses=True,
            protocol=3, cache_config=CacheConfig(max_size=100)
        )
        csc_client.ping()
        print("✓ CSC client initialized")
    except Exception as e:
        print(f"⚠ CSC not available: {e}")
    
    print("\nGenerating metrics...\n")
    
    iteration = 0
    start_time = time.time()

    try:
        while True:
            iteration += 1

            # 1. Command operations (generates command metrics)
            key = f"metrics_test:{iteration % 1000}"
            value = f"value_{iteration}"

            client.set(key, value)
            client.get(key)
            client.incr(f"counter:{iteration % 10}")
            client.lpush(f"list:{iteration % 5}", value)
            client.lpop(f"list:{iteration % 5}")
            client.sadd(f"set:{iteration % 5}", value)
            client.hset(f"hash:{iteration % 5}", "field", value)

            # Occasionally delete keys
            if iteration % 10 == 0:
                client.delete(key)

            # 2. PubSub operations (generates pubsub metrics)
            pubsub_client.publish("metrics_test_channel", f"message_{iteration}")
            pubsub.get_message(timeout=0.001)  # Non-blocking receive

            # 3. Streaming operations (generates streaming metrics)
            entry_id = client.xadd(
                stream_name,
                {"data": f"stream_data_{iteration}", "timestamp": str(time.time())},
                maxlen=1000
            )

            # Read from stream
            messages = client.xreadgroup(
                consumer_group,
                consumer_name,
                {stream_name: ">"},
                count=5,
                block=1
            )

            # Acknowledge messages
            if messages:
                for stream, entries in messages:
                    for msg_id, _ in entries:
                        client.xack(stream_name, consumer_group, msg_id)

            # 4. CSC operations (generates CSC metrics if available)
            if csc_client:
                csc_key = f"csc_test:{iteration % 50}"
                csc_client.set(csc_key, value)
                csc_client.get(csc_key)  # Cache miss
                csc_client.get(csc_key)  # Cache hit

            # 5. Pipeline operations (generates batch command metrics)
            if iteration % 5 == 0:
                pipe = client.pipeline()
                for i in range(10):
                    pipe.set(f"pipe:{i}", f"val_{iteration}")
                pipe.execute()

            # 6. Occasionally trigger errors (generates error metrics)
            if iteration % 50 == 0:
                try:
                    client.execute_command("INVALID_COMMAND")
                except Exception:
                    pass  # Expected error

            # Print progress
            if iteration % 100 == 0:
                elapsed = time.time() - start_time
                rate = iteration / elapsed
                print(f"Iteration {iteration:,} | {rate:.1f} iterations/sec | Elapsed: {elapsed:.0f}s")

            # Small delay to avoid overwhelming Redis
            time.sleep(0.01)

    except KeyboardInterrupt:
        print("\n\nStopping metrics generation...")
        elapsed = time.time() - start_time
        print(f"\n{'='*60}")
        print("SUMMARY")
        print(f"{'='*60}")
        print(f"Total iterations: {iteration:,}")
        print(f"Total time: {elapsed:.0f}s ({elapsed/60:.1f} minutes)")
        print(f"Average rate: {iteration/elapsed:.1f} iterations/sec")
        print(f"{'='*60}")

    finally:
        # Cleanup
        print("\nCleaning up...")
        try:
            pubsub.unsubscribe()
            pubsub.close()
            pubsub_client.close()
            client.close()
            if csc_client:
                csc_client.close()
        except Exception:
            pass


def main():
    """Main entry point."""
    parser = argparse.ArgumentParser(
        description="Generate Redis metrics continuously for Grafana dashboard testing"
    )
    parser.add_argument(
        "--host", type=str, default="localhost",
        help="Redis host (default: localhost)"
    )
    parser.add_argument(
        "--port", type=int, default=6379,
        help="Redis port (default: 6379)"
    )
    parser.add_argument(
        "--export-interval", type=int, default=10000,
        help="OTel metric export interval in milliseconds (default: 10000)"
    )
    parser.add_argument(
        "--endpoint", type=str, default=None,
        help="OTel collector endpoint (default: http://localhost:4318/v1/metrics)"
    )

    args = parser.parse_args()

    # Set up OTel
    setup_otel(export_interval_ms=args.export_interval, endpoint=args.endpoint)

    # Generate metrics
    run_redis_operations(host=args.host, port=args.port)

    return 0


if __name__ == "__main__":
    sys.exit(main())
