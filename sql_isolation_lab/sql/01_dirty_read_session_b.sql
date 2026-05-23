USE isolation_lab;

-- Start this file immediately after 01_dirty_read_session_a.sql.
-- READ UNCOMMITTED allows the transaction to see data that Session A will roll back.
SET SESSION TRANSACTION ISOLATION LEVEL READ UNCOMMITTED;
START TRANSACTION;

SELECT SLEEP(2) AS wait_until_session_a_updates;
SELECT 'T2: dirty read, sees uncommitted Alice balance' AS step;
SELECT id, owner_name, balance FROM accounts WHERE id = 1;

COMMIT;

SELECT SLEEP(8) AS wait_until_session_a_rolls_back;
SELECT 'T2: after rollback the committed value is different' AS step;
SELECT id, owner_name, balance FROM accounts WHERE id = 1;
