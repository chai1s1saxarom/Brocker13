USE isolation_lab;

-- Lost update with an application-side read-modify-write pattern.
-- T1 reads Bob = 100 and plans to subtract 10.
-- T2 also reads Bob = 100 and commits Bob = 70.
-- T1 then writes its stale calculated value 90 and overwrites T2's update.
UPDATE accounts SET balance = 100 WHERE id = 2;

SET SESSION TRANSACTION ISOLATION LEVEL READ COMMITTED;
START TRANSACTION;

SELECT 'T1: read Bob balance = 100, application calculates 100 - 10 = 90' AS step;
SELECT id, owner_name, balance FROM accounts WHERE id = 2;

SELECT SLEEP(6) AS wait_for_session_b_commit;

UPDATE accounts SET balance = 90 WHERE id = 2;
COMMIT;

SELECT 'T1: final value is 90, so T2 update to 70 was lost' AS step;
SELECT id, owner_name, balance FROM accounts WHERE id = 2;
