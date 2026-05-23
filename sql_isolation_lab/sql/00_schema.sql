DROP DATABASE IF EXISTS isolation_lab;
CREATE DATABASE isolation_lab;
USE isolation_lab;

CREATE TABLE accounts (
    id INT PRIMARY KEY,
    owner_name VARCHAR(50) NOT NULL,
    balance INT NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE = InnoDB;

CREATE TABLE bookings (
    id INT AUTO_INCREMENT PRIMARY KEY,
    room_id INT NOT NULL,
    customer_name VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_bookings_room_status (room_id, status)
) ENGINE = InnoDB;

INSERT INTO accounts (id, owner_name, balance) VALUES
    (1, 'Alice', 100),
    (2, 'Bob', 100);

INSERT INTO bookings (room_id, customer_name, status) VALUES
    (101, 'Initial customer', 'active'),
    (102, 'Another customer', 'active');

SELECT 'schema is ready' AS message;
SELECT * FROM accounts ORDER BY id;
SELECT id, room_id, customer_name, status FROM bookings ORDER BY id;
