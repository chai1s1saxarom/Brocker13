USE isolation_lab;

-- Session A reads the same row twice inside one transaction.
-- At READ COMMITTED the second read can return a newer committed value.
UPDATE accounts SET balance = 100 WHERE id = 1;

SET SESSION TRANSACTION ISOLATION LEVEL READ COMMITTED;
START TRANSACTION;

SELECT 'T1: first read of Alice balance' AS step;
SELECT id, owner_name, balance FROM accounts WHERE id = 1;

SELECT SLEEP(6) AS wait_for_session_b_commit;

SELECT 'T1: second read of the same row in the same transaction' AS step;
SELECT id, owner_name, balance FROM accounts WHERE id = 1;

COMMIT;
