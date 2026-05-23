USE isolation_lab;

-- Start this file immediately after 04_lost_update_session_a.sql.
SET SESSION TRANSACTION ISOLATION LEVEL READ COMMITTED;
START TRANSACTION;

SELECT SLEEP(2) AS wait_until_session_a_reads;
SELECT 'T2: also reads Bob balance = 100, application calculates 100 - 30 = 70' AS step;
SELECT id, owner_name, balance FROM accounts WHERE id = 2;

UPDATE accounts SET balance = 70 WHERE id = 2;
COMMIT;

SELECT 'T2: committed Bob balance = 70' AS step;
SELECT id, owner_name, balance FROM accounts WHERE id = 2;
