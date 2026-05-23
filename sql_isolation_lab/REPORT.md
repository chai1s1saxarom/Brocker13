# Отчет: аномалии изоляции в SQL

## Цель

Показать на практике, что при параллельной работе с SQL-БД могут возникать аномалии изоляции транзакций.

## Стенд

Использована SQL-БД `MySQL 8.0` с движком `InnoDB`.

- `accounts` - таблица счетов, используется для `dirty read`, `non-repeatable read` и `lost update`.
- `bookings` - таблица бронирований, используется для `phantom read`.
- Для воспроизведения открываются две параллельные сессии MySQL: `Session A` и `Session B`.
- Начальная схема и данные находятся в `sql/00_schema.sql`.

Запуск стенда:

```powershell
cd .\sql_isolation_lab
docker compose up -d
Get-Content .\sql\00_schema.sql | docker exec -i sql-isolation-lab-mysql mysql -uroot -proot
```

## Выбранные аномалии

| Аномалия | Уровень изоляции в примере | Файлы |
|---|---|---|
| `dirty read` | `READ UNCOMMITTED` | `sql/01_dirty_read_session_a.sql`, `sql/01_dirty_read_session_b.sql` |
| `non-repeatable read` | `READ COMMITTED` | `sql/02_non_repeatable_read_session_a.sql`, `sql/02_non_repeatable_read_session_b.sql` |
| `phantom read` | `READ COMMITTED` | `sql/03_phantom_read_session_a.sql`, `sql/03_phantom_read_session_b.sql` |
| `lost update` | `READ COMMITTED` | `sql/04_lost_update_session_a.sql`, `sql/04_lost_update_session_b.sql` |

## 1. Dirty Read

`Dirty read` возникает, когда одна транзакция читает данные, измененные другой транзакцией, но еще не зафиксированные через `COMMIT`.

Шаги воспроизведения:

1. `Session A` выставляет `READ UNCOMMITTED`, начинает транзакцию и меняет баланс Alice со `100` на `50`.
2. `Session A` не делает `COMMIT`, а держит транзакцию открытой.
3. `Session B` тоже работает в `READ UNCOMMITTED` и читает баланс Alice.
4. `Session B` видит `50`, хотя это значение еще не зафиксировано.
5. `Session A` делает `ROLLBACK`, после чего реальное зафиксированное значение снова `100`.

Результат из лога `logs/dirty-read.log`:

```text
Session B:
T2: dirty read, sees uncommitted Alice balance
| id | owner_name | balance |
|  1 | Alice      |      50 |

Session A:
T1: rollback, Alice is back to committed value
| id | owner_name | balance |
|  1 | Alice      |     100 |
```

Вывод: `Session B` прочитала значение `50`, которого в итоге не было в БД после отката.

Как избежать:

- не использовать `READ UNCOMMITTED`;
- применять минимум `READ COMMITTED`, чтобы читать только зафиксированные данные.

## 2. Non-Repeatable Read

`Non-repeatable read` возникает, когда транзакция два раза читает одну и ту же строку, но получает разные значения из-за `COMMIT` другой транзакции между чтениями.

Шаги воспроизведения:

1. `Session A` выставляет `READ COMMITTED`, начинает транзакцию и читает баланс Alice: `100`.
2. `Session B` обновляет баланс Alice до `150` и делает `COMMIT`.
3. `Session A` повторно читает ту же строку в той же транзакции.
4. Второе чтение возвращает `150`.

Результат из лога `logs/non-repeatable-read.log`:

```text
Session A:
T1: first read of Alice balance
| id | owner_name | balance |
|  1 | Alice      |     100 |

Session A:
T1: second read of the same row in the same transaction
| id | owner_name | balance |
|  1 | Alice      |     150 |
```

Вывод: внутри одной транзакции одна и та же строка была прочитана с разными значениями.

Как избежать:

- использовать `REPEATABLE READ`, если в транзакции нужен стабильный снимок данных;
- использовать блокирующее чтение `SELECT ... FOR UPDATE`, если после чтения строка будет изменяться.

## 3. Phantom Read

`Phantom read` возникает, когда транзакция повторяет запрос по условию, а в результат попадают новые строки, добавленные другой транзакцией.

Шаги воспроизведения:

1. В `bookings` остается одно активное бронирование для комнаты `101`.
2. `Session A` выставляет `READ COMMITTED`, начинает транзакцию и считает активные бронирования комнаты `101`: `1`.
3. `Session B` вставляет новое активное бронирование комнаты `101` и делает `COMMIT`.
4. `Session A` повторяет тот же запрос по предикату.
5. Второй запрос возвращает `2`.

Результат из лога `logs/phantom-read.log`:

```text
Session A:
T1: first count of active bookings for room 101
| active_bookings |
|               1 |

Session A:
T1: second count of the same predicate in the same transaction
| active_bookings |
|               2 |
```

Вывод: при повторном чтении по тому же условию появилась новая строка-фантом.

Как избежать:

- использовать `REPEATABLE READ` или `SERIALIZABLE`;
- для строгой защиты диапазона применять блокировки диапазонов или `SERIALIZABLE`, когда нельзя допускать появление новых строк по условию.

## 4. Lost Update

`Lost update` возникает, когда две транзакции читают одно значение, независимо рассчитывают новое значение и затем одна запись перезаписывает результат другой.

Шаги воспроизведения:

1. Баланс Bob равен `100`.
2. `Session A` читает `100` и на стороне приложения рассчитывает `100 - 10 = 90`.
3. `Session B` тоже читает `100` и рассчитывает `100 - 30 = 70`.
4. `Session B` записывает `70` и делает `COMMIT`.
5. `Session A` записывает устаревшее расчетное значение `90` и делает `COMMIT`.
6. Итоговый баланс становится `90`, хотя при последовательном применении обеих операций ожидалось бы `60`.

Результат из лога `logs/lost-update.log`:

```text
Session B:
T2: committed Bob balance = 70
| id | owner_name | balance |
|  2 | Bob        |      70 |

Session A:
T1: final value is 90, so T2 update to 70 was lost
| id | owner_name | balance |
|  2 | Bob        |      90 |
```

Вывод: изменение `Session B` было потеряно, потому что `Session A` записала значение, рассчитанное на устаревших данных.

Как избежать:

- использовать атомарные обновления вида `UPDATE accounts SET balance = balance - 10 WHERE id = 2`;
- перед расчетом нового значения блокировать строку через `SELECT ... FOR UPDATE`;
- использовать `SERIALIZABLE`, чтобы конфликтующие транзакции ожидали друг друга или завершались ошибкой сериализации.

## Итоговая таблица

| Аномалия | Что произошло | Как избежать |
|---|---|---|
| `dirty read` | Прочитано незафиксированное значение `50`, которое потом откатилось | Не использовать `READ UNCOMMITTED`, выбрать `READ COMMITTED` или выше |
| `non-repeatable read` | В одной транзакции Alice сначала `100`, потом `150` | Использовать `REPEATABLE READ` или блокирующее чтение |
| `phantom read` | Повторный `COUNT` вернул `1`, затем `2` | Использовать `REPEATABLE READ`/`SERIALIZABLE`, блокировки диапазонов |
| `lost update` | Итоговый баланс Bob `90` вместо ожидаемого `60` | Атомарный `UPDATE`, `SELECT ... FOR UPDATE`, `SERIALIZABLE` |

## Вывод

Низкие уровни изоляции повышают параллелизм, но допускают неконсистентные результаты чтения и записи. Для операций чтения стабильного набора данных подходит `REPEATABLE READ`, для критичных денежных или складских операций лучше использовать атомарные изменения, блокировки строк или `SERIALIZABLE`.
