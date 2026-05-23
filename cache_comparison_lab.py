#!/usr/bin/env python3
"""Benchmark Lazy Loading, Write-Through and Write-Back cache strategies.

The lab is intentionally self-contained: SQLite is used as the database,
an in-memory object is used as the cache, and this file also contains the
load generator plus Markdown/CSV report generation.
"""

from __future__ import annotations

import argparse
import csv
import random
import sqlite3
import statistics
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Dict, Iterable, List, Sequence, Tuple


DEFAULT_INITIAL_RECORDS = 1_000
DEFAULT_REQUESTS = 3_000
DEFAULT_KEY_SPACE = 1_000
DEFAULT_SEED = 42

WORKLOADS = (
    ("read-heavy", 0.80),
    ("balanced", 0.50),
    ("write-heavy", 0.20),
)

STRATEGIES = ("lazy-loading", "write-through", "write-back")

Operation = Tuple[str, int, int]
MISSING = object()


def simulate_io(seconds: float) -> None:
    """Small deterministic delay that represents IO/cache latency."""
    end = time.perf_counter() + seconds
    while time.perf_counter() < end:
        pass


class Database:
    def __init__(self) -> None:
        self.connection = sqlite3.connect(":memory:")
        self.db_reads = 0
        self.db_writes = 0
        self.db_write_batches = 0
        self.connection.execute(
            "CREATE TABLE items (id INTEGER PRIMARY KEY, value INTEGER NOT NULL)"
        )

    def seed(self, records: int) -> None:
        rows = ((item_id, item_id * 10) for item_id in range(1, records + 1))
        self.connection.executemany("INSERT INTO items (id, value) VALUES (?, ?)", rows)
        self.connection.commit()

    def read(self, key: int) -> int:
        self.db_reads += 1
        simulate_io(0.00008)
        row = self.connection.execute(
            "SELECT value FROM items WHERE id = ?",
            (key,),
        ).fetchone()
        if row is None:
            raise KeyError(f"key {key} does not exist in database")
        return int(row[0])

    def write(self, key: int, value: int) -> None:
        self.db_writes += 1
        self.db_write_batches += 1
        simulate_io(0.00016)
        self.connection.execute(
            """
            INSERT INTO items (id, value) VALUES (?, ?)
            ON CONFLICT(id) DO UPDATE SET value = excluded.value
            """,
            (key, value),
        )
        self.connection.commit()

    def write_many(self, rows: Dict[int, int]) -> None:
        if not rows:
            return

        self.db_writes += len(rows)
        self.db_write_batches += 1
        simulate_io(0.00012 + len(rows) * 0.000025)
        self.connection.executemany(
            """
            INSERT INTO items (id, value) VALUES (?, ?)
            ON CONFLICT(id) DO UPDATE SET value = excluded.value
            """,
            rows.items(),
        )
        self.connection.commit()


class Cache:
    def __init__(self) -> None:
        self.values: Dict[int, int] = {}
        self.gets = 0
        self.hits = 0
        self.misses = 0
        self.sets = 0
        self.deletes = 0

    def get(self, key: int) -> object:
        self.gets += 1
        simulate_io(0.000006)
        if key in self.values:
            self.hits += 1
            return self.values[key]
        self.misses += 1
        return MISSING

    def set(self, key: int, value: int) -> None:
        self.sets += 1
        simulate_io(0.000008)
        self.values[key] = value

    def delete(self, key: int) -> None:
        self.deletes += 1
        simulate_io(0.000004)
        self.values.pop(key, None)

    @property
    def hit_rate(self) -> float:
        return self.hits / self.gets if self.gets else 0.0


class Application:
    def __init__(self, database: Database, cache: Cache) -> None:
        self.database = database
        self.cache = cache
        self.max_pending_writes = 0
        self.flushes = 0

    def read(self, key: int) -> int:
        raise NotImplementedError

    def write(self, key: int, value: int) -> None:
        raise NotImplementedError

    def finish(self) -> float:
        return 0.0


class LazyLoadingApplication(Application):
    def read(self, key: int) -> int:
        value = self.cache.get(key)
        if value is not MISSING:
            return int(value)

        value = self.database.read(key)
        self.cache.set(key, value)
        return value

    def write(self, key: int, value: int) -> None:
        self.database.write(key, value)
        # Invalidate cached value so the next read cannot return stale data.
        self.cache.delete(key)


class WriteThroughApplication(Application):
    def read(self, key: int) -> int:
        value = self.cache.get(key)
        if value is not MISSING:
            return int(value)

        value = self.database.read(key)
        self.cache.set(key, value)
        return value

    def write(self, key: int, value: int) -> None:
        self.cache.set(key, value)
        self.database.write(key, value)


class WriteBackApplication(Application):
    def __init__(
        self,
        database: Database,
        cache: Cache,
        flush_threshold: int = 250,
    ) -> None:
        super().__init__(database, cache)
        self.flush_threshold = flush_threshold
        self.pending_writes: Dict[int, int] = {}

    def read(self, key: int) -> int:
        value = self.cache.get(key)
        if value is not MISSING:
            return int(value)

        value = self.database.read(key)
        self.cache.set(key, value)
        return value

    def write(self, key: int, value: int) -> None:
        self.cache.set(key, value)
        self.pending_writes[key] = value
        self.max_pending_writes = max(self.max_pending_writes, len(self.pending_writes))
        if len(self.pending_writes) >= self.flush_threshold:
            self.flush()

    def flush(self) -> None:
        if not self.pending_writes:
            return
        self.database.write_many(self.pending_writes)
        self.pending_writes.clear()
        self.flushes += 1

    def finish(self) -> float:
        start = time.perf_counter()
        self.flush()
        return (time.perf_counter() - start) * 1_000


@dataclass(frozen=True)
class BenchmarkResult:
    workload: str
    strategy: str
    read_percent: int
    write_percent: int
    requests: int
    elapsed_seconds: float
    throughput_rps: float
    avg_latency_ms: float
    p95_latency_ms: float
    db_reads: int
    db_writes: int
    db_operations: int
    db_write_batches: int
    cache_gets: int
    cache_hits: int
    cache_misses: int
    cache_hit_rate_percent: float
    max_pending_writes: int
    flushes: int
    final_flush_ms: float


def create_application(strategy: str, database: Database, cache: Cache) -> Application:
    if strategy == "lazy-loading":
        return LazyLoadingApplication(database, cache)
    if strategy == "write-through":
        return WriteThroughApplication(database, cache)
    if strategy == "write-back":
        return WriteBackApplication(database, cache)
    raise ValueError(f"unknown strategy: {strategy}")


def build_plan(
    read_ratio: float,
    requests: int,
    key_space: int,
    seed: int,
) -> List[Operation]:
    rng = random.Random(seed)
    plan: List[Operation] = []
    for _ in range(requests):
        key = rng.randint(1, key_space)
        if rng.random() < read_ratio:
            plan.append(("read", key, 0))
        else:
            plan.append(("write", key, rng.randint(1, 1_000_000)))
    return plan


def percentile(values: Sequence[float], percent: float) -> float:
    if not values:
        return 0.0
    ordered = sorted(values)
    index = int(round((len(ordered) - 1) * percent))
    return ordered[index]


def run_benchmark(
    workload: str,
    read_ratio: float,
    strategy: str,
    plan: Iterable[Operation],
    initial_records: int,
) -> BenchmarkResult:
    database = Database()
    database.seed(initial_records)
    cache = Cache()
    app = create_application(strategy, database, cache)
    latencies_ms: List[float] = []

    start = time.perf_counter()
    request_count = 0
    for operation, key, value in plan:
        request_count += 1
        request_start = time.perf_counter()
        if operation == "read":
            app.read(key)
        else:
            app.write(key, value)
        latencies_ms.append((time.perf_counter() - request_start) * 1_000)
    elapsed = time.perf_counter() - start

    final_flush_ms = app.finish()
    db_operations = database.db_reads + database.db_writes

    return BenchmarkResult(
        workload=workload,
        strategy=strategy,
        read_percent=round(read_ratio * 100),
        write_percent=round((1 - read_ratio) * 100),
        requests=request_count,
        elapsed_seconds=elapsed,
        throughput_rps=request_count / elapsed if elapsed else 0.0,
        avg_latency_ms=statistics.mean(latencies_ms) if latencies_ms else 0.0,
        p95_latency_ms=percentile(latencies_ms, 0.95),
        db_reads=database.db_reads,
        db_writes=database.db_writes,
        db_operations=db_operations,
        db_write_batches=database.db_write_batches,
        cache_gets=cache.gets,
        cache_hits=cache.hits,
        cache_misses=cache.misses,
        cache_hit_rate_percent=cache.hit_rate * 100,
        max_pending_writes=app.max_pending_writes,
        flushes=app.flushes,
        final_flush_ms=final_flush_ms,
    )


def result_line(result: BenchmarkResult) -> str:
    return (
        f"{result.workload:11} | {result.strategy:13} | "
        f"throughput={result.throughput_rps:8.2f} req/s | "
        f"avg={result.avg_latency_ms:6.3f} ms | "
        f"db_ops={result.db_operations:5} | "
        f"hit_rate={result.cache_hit_rate_percent:6.2f}% | "
        f"pending_max={result.max_pending_writes:3} | "
        f"flushes={result.flushes:2}"
    )


def write_csv(path: Path, results: Sequence[BenchmarkResult]) -> None:
    with path.open("w", newline="", encoding="utf-8") as file:
        writer = csv.DictWriter(file, fieldnames=list(BenchmarkResult.__dataclass_fields__))
        writer.writeheader()
        for result in results:
            writer.writerow(result.__dict__)


def markdown_table(results: Sequence[BenchmarkResult]) -> str:
    rows = [
        "| Workload | Strategy | Req | Throughput, req/sec | Avg latency, ms | P95, ms | DB ops | DB reads | DB writes | Cache hit rate |",
        "|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|",
    ]
    for result in results:
        rows.append(
            "| "
            f"{result.workload} | {result.strategy} | {result.requests} | "
            f"{result.throughput_rps:.2f} | {result.avg_latency_ms:.3f} | "
            f"{result.p95_latency_ms:.3f} | {result.db_operations} | "
            f"{result.db_reads} | {result.db_writes} | "
            f"{result.cache_hit_rate_percent:.2f}% |"
        )
    return "\n".join(rows)


def write_report(
    path: Path,
    results: Sequence[BenchmarkResult],
    console_lines: Sequence[str],
    initial_records: int,
    key_space: int,
    seed: int,
) -> None:
    write_back_rows = [result for result in results if result.strategy == "write-back"]
    write_back_table = [
        "| Workload | Max pending writes | Flushes | Final flush, ms | DB write batches |",
        "|---|---:|---:|---:|---:|",
    ]
    for result in write_back_rows:
        write_back_table.append(
            "| "
            f"{result.workload} | {result.max_pending_writes} | {result.flushes} | "
            f"{result.final_flush_ms:.3f} | {result.db_write_batches} |"
        )

    content = f"""# Отчет: сравнение типов кеширования

## Описание стенда

Реализована одна система в трех вариантах: `Lazy Loading / Cache-Aside`, `Write-Through` и `Write-Back`.

- `load-generator`: функция `build_plan`, которая создает одинаковый план запросов для всех стратегий.
- `application`: классы `LazyLoadingApplication`, `WriteThroughApplication`, `WriteBackApplication`.
- `cache`: in-memory кеш `Cache` с подсчетом `hits`, `misses`, `hit rate`.
- `БД`: SQLite in-memory `Database` с подсчетом обращений на чтение, запись и batch-запись.

Для корректности в варианте `Lazy Loading` запись идет в БД, а существующее значение в кеше инвалидируется. В варианте `Write-Back` запись сначала попадает в кеш и в очередь отложенных записей, а затем сбрасывается в БД batch-операцией.

## Описание тестов

Для каждой стратегии использовался один и тот же набор данных и один и тот же план операций внутри каждого сценария.

- начальный набор данных: `{initial_records}` записей;
- диапазон ключей: `1..{key_space}`;
- seed генератора: `{seed}`;
- сценарии: `read-heavy` = 80% read / 20% write, `balanced` = 50% read / 50% write, `write-heavy` = 20% read / 80% write;
- метрики: `throughput`, средняя задержка, P95 задержка, количество обращений в БД, cache hit rate.

## Таблица результатов

{markdown_table(results)}

## Накопление записей в Write-Back

{chr(10).join(write_back_table)}

## Фрагмент консоли

```text
{chr(10).join(console_lines)}
```

## Выводы

Для чтения лучше всего подходят `Lazy Loading` и `Write-Through`: после прогрева кеша чтения обслуживаются из кеша, поэтому hit rate высокий, а обращений в БД становится меньше. `Write-Through` дополнительно держит кеш актуальным после записей.

Для записи самый быстрый пользовательский путь показывает `Write-Back`, потому что запись сначала фиксируется в кеше и попадает в БД позже batch-операцией. Минус этой стратегии: до flush данные есть в кеше, но еще не сохранены в БД, поэтому при сбое возможна потеря последних изменений.

Для смешанной нагрузки практичнее выбирать по требованиям к консистентности. Если нужна строгая актуальность БД сразу после записи, лучше `Write-Through`. Если допустима eventual consistency и важна скорость записи, лучше `Write-Back`. Если записи не должны засорять кеш и чтения преобладают, подходит `Lazy Loading / Cache-Aside`.
"""
    path.write_text(content, encoding="utf-8")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--requests", type=int, default=DEFAULT_REQUESTS)
    parser.add_argument("--initial-records", type=int, default=DEFAULT_INITIAL_RECORDS)
    parser.add_argument("--key-space", type=int, default=DEFAULT_KEY_SPACE)
    parser.add_argument("--seed", type=int, default=DEFAULT_SEED)
    parser.add_argument("--out", type=Path, default=Path("cache_results"))
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    args.out.mkdir(parents=True, exist_ok=True)

    console_lines = [
        "workload     | strategy      | metrics",
        "-" * 104,
    ]
    results: List[BenchmarkResult] = []

    for workload_index, (workload, read_ratio) in enumerate(WORKLOADS):
        plan = build_plan(
            read_ratio=read_ratio,
            requests=args.requests,
            key_space=args.key_space,
            seed=args.seed + workload_index,
        )
        for strategy in STRATEGIES:
            result = run_benchmark(
                workload=workload,
                read_ratio=read_ratio,
                strategy=strategy,
                plan=plan,
                initial_records=args.initial_records,
            )
            results.append(result)
            console_lines.append(result_line(result))

    for line in console_lines:
        print(line)

    write_csv(args.out / "cache-comparison-results.csv", results)
    (args.out / "cache-comparison-console.log").write_text(
        "\n".join(console_lines) + "\n",
        encoding="utf-8",
    )
    write_report(
        path=args.out / "cache-comparison-report.md",
        results=results,
        console_lines=console_lines,
        initial_records=args.initial_records,
        key_space=args.key_space,
        seed=args.seed,
    )

    print()
    print(f"CSV:    {args.out / 'cache-comparison-results.csv'}")
    print(f"LOG:    {args.out / 'cache-comparison-console.log'}")
    print(f"REPORT: {args.out / 'cache-comparison-report.md'}")


if __name__ == "__main__":
    main()
