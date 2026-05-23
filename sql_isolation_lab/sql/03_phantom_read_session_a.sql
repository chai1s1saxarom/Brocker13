USE isolation_lab;

-- Session A counts active bookings for room 101 twice.
-- At READ COMMITTED the second predicate read can include a newly inserted row.
DELETE FROM bookings WHERE room_id = 101;
INSERT INTO bookings (room_id, customer_name, status)
VALUES (101, 'Initial customer', 'active');

SET SESSION TRANSACTION ISOLATION LEVEL READ COMMITTED;
START TRANSACTION;

SELECT 'T1: first count of active bookings for room 101' AS step;
SELECT COUNT(*) AS active_bookings
FROM bookings
WHERE room_id = 101 AND status = 'active';

SELECT SLEEP(6) AS wait_for_session_b_insert;

SELECT 'T1: second count of the same predicate in the same transaction' AS step;
SELECT COUNT(*) AS active_bookings
FROM bookings
WHERE room_id = 101 AND status = 'active';

COMMIT;
