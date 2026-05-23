USE isolation_lab;

-- Start this file immediately after 02_non_repeatable_read_session_a.sql.
SET SESSION TRANSACTION ISOLATION LEVEL READ COMMITTED;
START TRANSACTION;

SELECT SLEEP(2) AS wait_until_session_a_first_read;
UPDATE accounts SET balance = 150 WHERE id = 1;
SELECT 'T2: committed Alice balance = 150' AS step;
SELECT id, owner_name, balance FROM accounts WHERE id = 1;

COMMIT;
