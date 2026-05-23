USE isolation_lab;

-- Session A holds an uncommitted update for 8 seconds and then rolls it back.
UPDATE accounts SET balance = 100 WHERE id = 1;

SET SESSION TRANSACTION ISOLATION LEVEL READ UNCOMMITTED;
START TRANSACTION;

UPDATE accounts SET balance = 50 WHERE id = 1;
SELECT 'T1: updated Alice to 50, but did not commit' AS step;
SELECT id, owner_name, balance FROM accounts WHERE id = 1;

SELECT SLEEP(8) AS wait_for_session_b_dirty_read;

ROLLBACK;
SELECT 'T1: rollback, Alice is back to committed value' AS step;
SELECT id, owner_name, balance FROM accounts WHERE id = 1;
