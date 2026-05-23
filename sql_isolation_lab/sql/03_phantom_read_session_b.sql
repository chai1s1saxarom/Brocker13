USE isolation_lab;

-- Start this file immediately after 03_phantom_read_session_a.sql.
SET SESSION TRANSACTION ISOLATION LEVEL READ COMMITTED;
START TRANSACTION;

SELECT SLEEP(2) AS wait_until_session_a_first_count;
INSERT INTO bookings (room_id, customer_name, status)
VALUES (101, 'Phantom customer', 'active');

SELECT 'T2: inserted and committed a new row matching T1 predicate' AS step;
SELECT id, room_id, customer_name, status
FROM bookings
WHERE room_id = 101 AND status = 'active'
ORDER BY id;

COMMIT;
