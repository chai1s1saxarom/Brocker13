import pika
import redis
import json
import time
import argparse
import logging
import sys
from datetime import datetime

logging.basicConfig(stream=sys.stdout, level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')

def consume_rabbitmq(host, queue_name, duration_seconds):
    """Потребление сообщений из RabbitMQ в течение указанного времени"""
    connection = pika.BlockingConnection(pika.ConnectionParameters(host))
    channel = connection.channel()
    channel.queue_declare(queue=queue_name, durable=True)
    
    messages_processed = 0
    total_latency = 0
    max_latency = 0
    
    def callback(ch, method, properties, body):
        nonlocal messages_processed, total_latency, max_latency
        messages_processed += 1
        if properties.timestamp:
            latency = time.time() - properties.timestamp
            total_latency += latency
            if latency > max_latency:
                max_latency = latency
        ch.basic_ack(delivery_tag=method.delivery_tag)
        if messages_processed % 1000 == 0:
            avg_latency = total_latency / messages_processed if messages_processed > 0 else 0
            logging.info(f"RabbitMQ: обработано сообщений {messages_processed}, avg latency: {avg_latency:.4f} sec, max latency: {max_latency:.4f} sec")
    
    channel.basic_qos(prefetch_count=100)
    channel.basic_consume(queue=queue_name, on_message_callback=callback)
    
    logging.info(f"Запуск потребления RabbitMQ на {duration_seconds} секунд...")
    start_time = time.time()
    try:
        channel.start_consuming()
    except KeyboardInterrupt:
        pass
    finally:
        duration = time.time() - start_time
        channel.stop_consuming()
        connection.close()
        if messages_processed > 0:
            avg_latency = total_latency / messages_processed
            logging.info(f"RabbitMQ статистика: processed={messages_processed}, duration={duration:.2f}s, rate={messages_processed/duration:.2f} msg/s, avg latency={avg_latency:.4f}s, max latency={max_latency:.4f}s")
        return messages_processed, duration

def consume_redis(host, port, queue_key, duration_seconds):
    """Потребление сообщений из Redis очереди в течение указанного времени (BRPOP)"""
    r = redis.Redis(host=host, port=port)
    messages_processed = 0
    start_time = time.time()
    logging.info(f"Запуск потребления Redis на {duration_seconds} секунд...")
    while (time.time() - start_time) < duration_seconds:
        try:
            # Ожидание сообщения в очереди в течение 1 секунды
            result = r.brpop(queue_key, timeout=1)
            if result is None:
                continue
            messages_processed += 1
            if messages_processed % 1000 == 0:
                logging.info(f"Redis: обработано сообщений {messages_processed}")
        except Exception as e:
            logging.error(f"Ошибка при потреблении из Redis: {e}")
            break
    duration = time.time() - start_time
    logging.info(f"Redis статистика: processed={messages_processed}, duration={duration:.2f}s, rate={messages_processed/duration:.2f} msg/s")
    return messages_processed, duration

def consume_redis_streams(host, port, stream_key, group_name, consumer_name, duration_seconds):
    """Потребление сообщений из Redis Streams с использованием Consumer Groups"""
    r = redis.Redis(host=host, port=port, decode_responses=True)
    try:
        r.xgroup_create(stream_key, group_name, id='0', mkstream=True)
    except redis.exceptions.ResponseError as e:
        if "BUSYGROUP" not in str(e):
            raise
    messages_processed = 0
    start_time = time.time()
    logging.info(f"Запуск потребления Redis Streams (consumer={consumer_name}) на {duration_seconds} секунд...")
    while (time.time() - start_time) < duration_seconds:
        try:
            results = r.xreadgroup(group_name, consumer_name, {stream_key: '>'}, count=100, block=1000)
            for stream, messages in results:
                for message_id, data in messages:
                    messages_processed += 1
                    r.xack(stream_key, group_name, message_id)
            if messages_processed % 1000 == 0:
                logging.info(f"Redis Streams: обработано сообщений {messages_processed}")
        except Exception as e:
            logging.error(f"Ошибка при потреблении из Redis Streams: {e}")
            break
    duration = time.time() - start_time
    logging.info(f"Redis Streams статистика: processed={messages_processed}, duration={duration:.2f}s, rate={messages_processed/duration:.2f} msg/s")
    return messages_processed, duration

if __name__ == '__main__':
    parser = argparse.ArgumentParser(description="Запуск потребителя для брокера сообщений")
    parser.add_argument('--broker', type=str, default='rabbitmq', choices=['rabbitmq', 'redis', 'redis-streams'], help='Тип брокера')
    parser.add_argument('--duration', type=int, default=60, help='Длительность потребления в секундах')
    parser.add_argument('--host', type=str, default='localhost', help='Хост брокера')
    parser.add_argument('--port', type=int, default=6379, help='Порт брокера (для Redis)')
    parser.add_argument('--queue', type=str, default='test_queue', help='Имя очереди / ключ / stream для брокера')
    parser.add_argument('--group', type=str, default='test_group', help='Имя consumer group (для Redis Streams)')
    parser.add_argument('--consumer', type=str, default='consumer_1', help='Имя consumer (для Redis Streams)')
    args = parser.parse_args()
    
    logging.info(f"Запуск потребителя с параметрами: broker={args.broker}, duration={args.duration}s")
    
    if args.broker == 'rabbitmq':
        count, dur = consume_rabbitmq(args.host, args.queue, args.duration)
    elif args.broker == 'redis':
        count, dur = consume_redis(args.host, args.port, args.queue, args.duration)
    elif args.broker == 'redis-streams':
        count, dur = consume_redis_streams(args.host, args.port, args.queue, args.group, args.consumer, args.duration)