/**
 * Redis OpenTelemetry Metrics Example (TypeScript / node-redis)
 *
 * This example demonstrates how to use node-redis with OpenTelemetry metrics.
 * It performs various Redis operations continuously and exports metrics to an
 * OTLP collector.
 *
 * Prerequisites:
 *   - Redis server running on localhost:6379
 *   - OpenTelemetry Collector running on localhost:4318 (HTTP)
 *   - Node.js 18.19.0+
 *   - Install dependencies: npm install
 *
 * Usage:
 *   npm start
 *   npm start -- --host localhost --port 6379
 *   npm start -- --endpoint http://localhost:4318/v1/metrics
 *   REDIS_URL=redis://localhost:6379 npm start
 */

import { parseArgs } from 'node:util';
import { OTLPMetricExporter } from '@opentelemetry/exporter-metrics-otlp-http';
import {
  createClient,
  OpenTelemetry,
  type RedisClientType
} from 'redis';
import {
  MeterProvider,
  PeriodicExportingMetricReader
} from '@opentelemetry/sdk-metrics';

type RedisClient = RedisClientType<any, any, any, any, any>;

const connectClient = async (
  client: RedisClient,
  clientName: string
): Promise<void> => {
  await client.connect();
  console.log(`${clientName} connected`);
};

const parseCommandLineArgs = (): {
  host: string;
  port: number;
  exportInterval: number;
  endpoint: string;
  redisUrl: string | undefined;
} => {
  const parsed = parseArgs({
    args: process.argv.slice(2),
    allowPositionals: false,
    strict: true,
    options: {
      host: {
        type: 'string',
        default: 'localhost'
      },
      port: {
        type: 'string',
        default: '6379'
      },
      'export-interval': {
        type: 'string',
        default: '10000'
      },
      endpoint: {
        type: 'string',
        default: 'http://localhost:4318/v1/metrics',
      },
      'redis-url': {
        type: 'string'
      }
    }
  });

  const port = Number(parsed.values.port);
  if (Number.isNaN(port)) {
    throw new TypeError(`Invalid value for --port: ${parsed.values.port}`);
  }

  const exportInterval = Number(parsed.values['export-interval']);
  if (Number.isNaN(exportInterval)) {
    throw new TypeError(
      `Invalid value for --export-interval: ${parsed.values['export-interval']}`
    );
  }

  return {
    host: parsed.values.host ?? 'localhost',
    port,
    exportInterval,
    endpoint: parsed.values.endpoint,
    redisUrl: parsed.values['redis-url'] ?? process.env.REDIS_URL
  };
};

const setupOtel = async (
  exportIntervalMs: number,
  endpoint: string
): Promise<MeterProvider> => {
  const reader = new PeriodicExportingMetricReader({
    exporter: new OTLPMetricExporter({
      url: process.env.OTEL_EXPORTER_OTLP_ENDPOINT ?? endpoint 
    }),
    exportIntervalMillis: exportIntervalMs
  });

  const meterProvider = new MeterProvider({
    readers: [reader]
  });

  OpenTelemetry.init({
    metrics: {
      enabled: true,
      meterProvider,
      enabledMetricGroups: [
        'command',
        'pubsub',
        'streaming',
        'client-side-caching',
        'connection-basic',
        'connection-advanced',
        'resiliency'
      ]
    }
  });

  console.log(`OTel configured to export to ${endpoint} every ${exportIntervalMs}ms`);
  console.log('All metric groups enabled');

  return meterProvider;
};

const runRedisOperations = async (redisUrl: string): Promise<void> => {
  console.log(`\n${'='.repeat(60)}`);
  console.log('Starting Redis operations');
  console.log(`Redis: ${redisUrl}`);
  console.log('Press Ctrl+C to stop');
  console.log(`${'='.repeat(60)}\n`);

  const client: RedisClient = createClient({ url: redisUrl });
  client.on('error', (error) => {
    console.error('Redis client error:', error);
  });

  const pubsubPublisher: RedisClient = createClient({ url: redisUrl });
  pubsubPublisher.on('error', (error) => {
    console.error('Redis pub/sub publisher error:', error);
  });

  const pubsubSubscriber: RedisClient = createClient({ url: redisUrl });
  pubsubSubscriber.on('error', (error) => {
    console.error('Redis pub/sub subscriber error:', error);
  });

  await connectClient(client, 'Primary Redis client');
  await connectClient(pubsubPublisher, 'Redis pub/sub publisher client');
  await connectClient(pubsubSubscriber, 'Redis pub/sub subscriber client');

  await pubsubSubscriber.subscribe('metrics_test_channel', () => {});

  const streamName = 'metrics_test_stream';
  const consumerGroup = 'metrics_test_group';
  const consumerName = 'metrics_generator';

  try {
    await client.xGroupCreate(streamName, consumerGroup, '0', {
      MKSTREAM: true
    });
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    if (!message.includes('BUSYGROUP')) {
      throw error;
    }
  }

  let cscClient: RedisClient | null = null;
  try {
    cscClient = createClient({
      url: redisUrl,
      RESP: 3,
      clientSideCache: {
        ttl: 0,
        maxEntries: 100,
        evictPolicy: 'LRU'
      }
    });
    cscClient.on('error', (error) => {
      console.error('Redis CSC client error:', error);
    });
    await connectClient(cscClient, 'Redis CSC client');
    await cscClient.ping();
    console.log('Redis CSC client ready for client-side caching');
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    console.log(`CSC not available: ${message}`);
    if (cscClient) {
      cscClient.destroy();
      cscClient = null;
    }
  }

  console.log('\nGenerating metrics...\n');

  let shouldStop = false;
  const stop = () => {
    shouldStop = true;
  };

  process.on('SIGINT', stop);
  process.on('SIGTERM', stop);

  let iteration = 0;
  const startTime = Date.now();

  try {
    while (!shouldStop) {
      iteration += 1;

      const key = `metrics_test:${iteration % 1000}`;
      const value = `value_${iteration}`;

      await client.set(key, value);
      await client.get(key);
      await client.incr(`counter:${iteration % 10}`);
      await client.lPush(`list:${iteration % 5}`, value);
      await client.lPop(`list:${iteration % 5}`);
      await client.sAdd(`set:${iteration % 5}`, value);
      await client.hSet(`hash:${iteration % 5}`, 'field', value);

      if (iteration % 10 === 0) {
        await client.del(key);
      }

      await pubsubPublisher.publish('metrics_test_channel', `message_${iteration}`);
      await new Promise((resolve) => setTimeout(resolve, 1));

      await client.xAdd(streamName, '*', {
        data: `stream_data_${iteration}`,
        timestamp: String(Date.now())
      });

      const messages = await client.xReadGroup(
        consumerGroup,
        consumerName,
        {
          key: streamName,
          id: '>'
        },
        {
          COUNT: 5,
          BLOCK: 1
        }
      );

      if (messages) {
        for (const stream of (messages as any[])) {
          for (const entry of stream.messages) {
            await client.xAck(streamName, consumerGroup, entry.id);
          }
        }
      }

      if (cscClient) {
        const cscKey = `csc_test:${iteration % 50}`;
        await cscClient.set(cscKey, value);
        await cscClient.get(cscKey);
        await cscClient.get(cscKey);
      }

      if (iteration % 5 === 0) {
        const multi = client.multi();
        for (let i = 0; i < 10; i += 1) {
          multi.set(`pipe:${i}`, `val_${iteration}`);
        }
        await multi.exec();
      }

      if (iteration % 50 === 0) {
        try {
          await client.sendCommand(['INVALID_COMMAND']);
        } catch {
          // Expected error for resiliency metrics.
        }
      }

      if (iteration % 100 === 0) {
        const elapsedSeconds = (Date.now() - startTime) / 1000;
        const rate = iteration / elapsedSeconds;
        console.log(
          `Iteration ${iteration.toLocaleString()} | ${rate.toFixed(1)} iterations/sec | Elapsed: ${elapsedSeconds.toFixed(0)}s`
        );
      }

      await new Promise((resolve) => setTimeout(resolve, 10));
    }

    console.log('\n\nStopping metrics generation...');
    const elapsedSeconds = (Date.now() - startTime) / 1000;
    console.log(`\n${'='.repeat(60)}`);
    console.log('SUMMARY');
    console.log(`${'='.repeat(60)}`);
    console.log(`Total iterations: ${iteration.toLocaleString()}`);
    console.log(`Total time: ${elapsedSeconds.toFixed(0)}s (${(elapsedSeconds / 60).toFixed(1)} minutes)`);
    console.log(`Average rate: ${(iteration / elapsedSeconds).toFixed(1)} iterations/sec`);
    console.log(`${'='.repeat(60)}`);
  } finally {
    process.off('SIGINT', stop);
    process.off('SIGTERM', stop);

    console.log('\nCleaning up...');

    try {
      await pubsubSubscriber.unsubscribe('metrics_test_channel');
    } catch {}

    try {
      pubsubSubscriber.destroy();
    } catch {}

    try {
      pubsubPublisher.destroy();
    } catch {}

    try {
      client.destroy();
    } catch {}

    if (cscClient) {
      try {
        cscClient.destroy();
      } catch {}
    }
  }
};

const main = async () => {
  let host: string;
  let port: number;
  let exportInterval: number;
  let endpoint: string;
  let redisUrl: string | undefined;

  try {
    ({
      host,
      port,
      exportInterval,
      endpoint,
      redisUrl
    } = parseCommandLineArgs());
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    console.error(message);
    return 1;
  }

  const resolvedRedisUrl = redisUrl ?? `redis://${host}:${port}`;
  const meterProvider = await setupOtel(exportInterval, endpoint);

  try {
    await runRedisOperations(resolvedRedisUrl);
    await meterProvider.forceFlush();
    return 0;
  } finally {
    await meterProvider.shutdown();
  }
};

process.exitCode = await main();
