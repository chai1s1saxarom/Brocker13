USE isolation_lab;

-- 1. Dirty read:
-- Do not use READ UNCOMMITTED. READ COMMITTED or stricter levels read only committed data.
SET SESSION TRANSACTION ISOLATION LEVEL READ COMMITTED;

-- 2. Non-repeatable read and phantom read:
-- Use REPEATABLE READ when a transaction needs a stable snapshot.
SET SESSION TRANSACTION ISOLATION LEVEL REPEATABLE READ;
START TRANSACTION;
SELECT id, owner_name, balance FROM accounts WHERE id = 1;
SELECT COUNT(*) AS active_bookings
FROM bookings
WHERE room_id = 101 AND status = 'active';
COMMIT;

-- 3. Lost update, option A:
-- Use an atomic update so the database applies the change to the latest row value.
UPDATE accounts
SET balance = balance - 10
WHERE id = 2;

-- 4. Lost update, option B:
-- Lock the row before calculating the new value in the application.
START TRANSACTION;
SELECT balance
FROM accounts
WHERE id = 2
FOR UPDATE;
-- Application calculates the next value while the row is locked.
UPDATE accounts
SET balance = 90
WHERE id = 2;
COMMIT;

-- 5. Strictest option:
-- SERIALIZABLE makes conflicting transactions wait or fail with a serialization error.
SET SESSION TRANSACTION ISOLATION LEVEL SERIALIZABLE;
