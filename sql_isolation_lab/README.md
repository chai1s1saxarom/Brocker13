# SQL Isolation Lab

Лабораторная работа показывает 4 аномалии изоляции SQL на MySQL/InnoDB:

- `dirty read`;
- `non-repeatable read`;
- `phantom read`;
- `lost update`.

## Запуск стенда

```powershell
cd .\sql_isolation_lab
docker compose up -d
Get-Content .\sql\00_schema.sql | docker exec -i sql-isolation-lab-mysql mysql -uroot -proot
```

Для каждой аномалии откройте два терминала. Сначала запускайте файл `session_a`,
сразу после него запускайте соответствующий файл `session_b`.

Пример для `dirty read`:

```powershell
# terminal 1
Get-Content .\sql\01_dirty_read_session_a.sql | docker exec -i sql-isolation-lab-mysql mysql -uroot -proot

# terminal 2
Get-Content .\sql\01_dirty_read_session_b.sql | docker exec -i sql-isolation-lab-mysql mysql -uroot -proot
```

Перед следующим сценарием можно снова выполнить `00_schema.sql`, чтобы вернуть
таблицы к исходному состоянию.

## Состав

- `sql/00_schema.sql` - создание БД, таблиц и тестовых данных.
- `sql/01_*` - сценарий `dirty read`.
- `sql/02_*` - сценарий `non-repeatable read`.
- `sql/03_*` - сценарий `phantom read`.
- `sql/04_*` - сценарий `lost update`.
- `sql/05_prevent_anomalies.sql` - примеры защиты от аномалий.
- `logs/*.log` - текстовые логи результатов, которые можно использовать вместо скриншотов консоли.
- `REPORT.md` - итоговый отчет.
