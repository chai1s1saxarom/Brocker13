import pika
import redis
import json
import time
import argparse
import logging
import sys

logging.basicConfig(stream=sys.stdout, level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')

def produce_rabbitmq(host, num_messages, payload_size, queue_name):
    """Отправка сообщений в RabbitMQ с указанным размером тела"""
    connection = pika.BlockingConnection(pika.ConnectionParameters(host))
    channel = connection.channel()
    channel.queue_declare(queue=queue_name, durable=True)
    # Создаём тело сообщения с указанным размером (заполненный пробелами)
    message_body = bytearray(payload_size) 
    logging.info(f"Запуск отправки {num_messages} сообщений в RabbitMQ (размер: {payload_size} байт)...")
    start_time = time.time()
    for i in range(num_messages):
        # Отправляем сообщение с меткой времени отправки
        channel.basic_publish(
            exchange='',
            routing_key=queue_name,
            body=message_body,
            properties=pika.BasicProperties(
                delivery_mode=2,  # Сделать сообщение персистентным
                timestamp=int(time.time())
            )
        )
        if (i + 1) % 10000 == 0:
            logging.info(f"RabbitMQ: отправлено {i+1}/{num_messages} сообщений")
    channel.close()
    connection.close()
    duration = time.time() - start_time
    logging.info(f"RabbitMQ: отправка завершена за {duration:.2f} сек. (rate: {num_messages/duration:.2f} msg/s)")

def produce_redis(host, port, num_messages, payload_size, queue_key):
    """Отправка сообщений в Redis list (структура данных очереди) с указанным размером тела"""
    r = redis.Redis(host=host, port=port, decode_responses=False)
    # Создаём тело сообщения для заполнения пробелами
    message_body = b'x' * payload_size   # создаёт bytes из символов 'x'
    r.lpush(queue_key, message_body)
    start_time = time.time()
    for i in range(num_messages):
        # LPUSH добавляет сообщение в начало списка
        r.lpush(queue_key, message_body)
        if (i + 1) % 10000 == 0:
            logging.info(f"Redis: отправлено {i+1}/{num_messages} сообщений")
    duration = time.time() - start_time
    logging.info(f"Redis: отправка завершена за {duration:.2f} сек. (rate: {num_messages/duration:.2f} msg/s)")

def produce_redis_streams(host, port, num_messages, payload_size, stream_key):
    """Отправка сообщений в Redis Streams с указанным размером тела сообщения"""
    r = redis.Redis(host=host, port=port, decode_responses=True)
    # Для Redis Streams мы отправим словарь с данными, содержащий фиктивную полезную нагрузку
    message_body = "x" * payload_size
    logging.info(f"Запуск отправки {num_messages} сообщений в Redis Streams (размер: {payload_size} байт)...")
    start_time = time.time()
    for i in range(num_messages):
        r.xadd(stream_key, {'data': message_body})
        if (i + 1) % 10000 == 0:
            logging.info(f"Redis Streams: отправлено {i+1}/{num_messages} сообщений")
    duration = time.time() - start_time
    logging.info(f"Redis Streams: отправка завершена за {duration:.2f} сек. (rate: {num_messages/duration:.2f} msg/s)")

if __name__ == '__main__':
    parser = argparse.ArgumentParser(description="Нагрузочное тестирование брокера сообщений")
    parser.add_argument('--broker', type=str, default='rabbitmq', choices=['rabbitmq', 'redis', 'redis-streams'], help='Тип брокера')
    parser.add_argument('--messages', type=int, default=5000, help='Количество сообщений для отправки')
    parser.add_argument('--size', type=int, default=1024, help='Размер полезной нагрузки в байтах')
    parser.add_argument('--host', type=str, default='localhost', help='Хост брокера')
    parser.add_argument('--port', type=int, default=6379, help='Порт брокера (для Redis)')
    parser.add_argument('--queue', type=str, default='test_queue', help='Имя очереди/ключа для брокера')
    args = parser.parse_args()
    
    logging.info(f"Запуск с параметрами: broker={args.broker}, messages={args.messages}, payload_size={args.size} bytes")
    
    if args.broker == 'rabbitmq':
        produce_rabbitmq(args.host, args.messages, args.size, args.queue)
    elif args.broker == 'redis':
        produce_redis(args.host, args.port, args.messages, args.size, args.queue)
    elif args.broker == 'redis-streams':
        produce_redis_streams(args.host, args.port, args.messages, args.size, args.queue)