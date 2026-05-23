# Отчет: сравнение типов кеширования

## Описание стенда

Реализована одна система в трех вариантах: `Lazy Loading / Cache-Aside`, `Write-Through` и `Write-Back`.

- `load-generator`: функция `build_plan`, которая создает одинаковый план запросов для всех стратегий.
- `application`: классы `LazyLoadingApplication`, `WriteThroughApplication`, `WriteBackApplication`.
- `cache`: in-memory кеш `Cache` с подсчетом `hits`, `misses`, `hit rate`.
- `БД`: SQLite in-memory `Database` с подсчетом обращений на чтение, запись и batch-запись.

Для корректности в варианте `Lazy Loading` запись идет в БД, а существующее значение в кеше инвалидируется. В варианте `Write-Back` запись сначала попадает в кеш и в очередь отложенных записей, а затем сбрасывается в БД batch-операцией.

## Описание тестов

Для каждой стратегии использовался один и тот же набор данных и один и тот же план операций внутри каждого сценария.

- начальный набор данных: `1000` записей;
- диапазон ключей: `1..1000`;
- seed генератора: `42`;
- сценарии: `read-heavy` = 80% read / 20% write, `balanced` = 50% read / 50% write, `write-heavy` = 20% read / 80% write;
- метрики: `throughput`, средняя задержка, P95 задержка, количество обращений в БД, cache hit rate.

## Таблица результатов

| Workload | Strategy | Req | Throughput, req/sec | Avg latency, ms | P95, ms | DB ops | DB reads | DB writes | Cache hit rate |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| read-heavy | lazy-loading | 3000 | 13455.44 | 0.074 | 0.176 | 1646 | 1080 | 566 | 55.63% |
| read-heavy | write-through | 3000 | 14556.80 | 0.068 | 0.191 | 1331 | 765 | 566 | 68.57% |
| read-heavy | write-back | 3000 | 25550.77 | 0.038 | 0.110 | 1251 | 765 | 486 | 68.57% |
| balanced | lazy-loading | 3000 | 7736.97 | 0.129 | 0.196 | 2506 | 953 | 1553 | 34.14% |
| balanced | write-through | 3000 | 8592.23 | 0.116 | 0.202 | 2023 | 470 | 1553 | 67.52% |
| balanced | write-back | 3000 | 27673.74 | 0.036 | 0.101 | 1820 | 470 | 1350 | 67.52% |
| write-heavy | lazy-loading | 3000 | 6157.26 | 0.162 | 0.202 | 2914 | 557 | 2357 | 13.37% |
| write-heavy | write-through | 3000 | 6397.04 | 0.155 | 0.212 | 2560 | 203 | 2357 | 68.43% |
| write-heavy | write-back | 3000 | 28615.47 | 0.034 | 0.100 | 2271 | 203 | 2068 | 68.43% |

## Накопление записей в Write-Back

| Workload | Max pending writes | Flushes | Final flush, ms | DB write batches |
|---|---:|---:|---:|---:|
| read-heavy | 250 | 2 | 6.658 | 2 |
| balanced | 250 | 6 | 2.831 | 6 |
| write-heavy | 250 | 9 | 1.945 | 9 |

## Фрагмент консоли

```text
workload     | strategy      | metrics
--------------------------------------------------------------------------------------------------------
read-heavy  | lazy-loading  | throughput=13455.44 req/s | avg= 0.074 ms | db_ops= 1646 | hit_rate= 55.63% | pending_max=  0 | flushes= 0
read-heavy  | write-through | throughput=14556.80 req/s | avg= 0.068 ms | db_ops= 1331 | hit_rate= 68.57% | pending_max=  0 | flushes= 0
read-heavy  | write-back    | throughput=25550.77 req/s | avg= 0.038 ms | db_ops= 1251 | hit_rate= 68.57% | pending_max=250 | flushes= 2
balanced    | lazy-loading  | throughput= 7736.97 req/s | avg= 0.129 ms | db_ops= 2506 | hit_rate= 34.14% | pending_max=  0 | flushes= 0
balanced    | write-through | throughput= 8592.23 req/s | avg= 0.116 ms | db_ops= 2023 | hit_rate= 67.52% | pending_max=  0 | flushes= 0
balanced    | write-back    | throughput=27673.74 req/s | avg= 0.036 ms | db_ops= 1820 | hit_rate= 67.52% | pending_max=250 | flushes= 6
write-heavy | lazy-loading  | throughput= 6157.26 req/s | avg= 0.162 ms | db_ops= 2914 | hit_rate= 13.37% | pending_max=  0 | flushes= 0
write-heavy | write-through | throughput= 6397.04 req/s | avg= 0.155 ms | db_ops= 2560 | hit_rate= 68.43% | pending_max=  0 | flushes= 0
write-heavy | write-back    | throughput=28615.47 req/s | avg= 0.034 ms | db_ops= 2271 | hit_rate= 68.43% | pending_max=250 | flushes= 9
```

## Выводы

Для чтения лучше всего подходят `Lazy Loading` и `Write-Through`: после прогрева кеша чтения обслуживаются из кеша, поэтому hit rate высокий, а обращений в БД становится меньше. `Write-Through` дополнительно держит кеш актуальным после записей.

Для записи самый быстрый пользовательский путь показывает `Write-Back`, потому что запись сначала фиксируется в кеше и попадает в БД позже batch-операцией. Минус этой стратегии: до flush данные есть в кеше, но еще не сохранены в БД, поэтому при сбое возможна потеря последних изменений.

Для смешанной нагрузки практичнее выбирать по требованиям к консистентности. Если нужна строгая актуальность БД сразу после записи, лучше `Write-Through`. Если допустима eventual consistency и важна скорость записи, лучше `Write-Back`. Если записи не должны засорять кеш и чтения преобладают, подходит `Lazy Loading / Cache-Aside`.
