package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type OrderItemInput struct {
	ProductID int64
	Quantity  int
}

func placeOrderTx(ctx context.Context, db *sql.DB, customerID int64, items []OrderItemInput) (int64, error) {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var orderID int64
	if err := tx.QueryRowContext(
		ctx,
		`INSERT INTO Orders (CustomerID, OrderDate, TotalAmount)
		 VALUES ($1, $2, 0)
		 RETURNING OrderID`,
		customerID, time.Now(),
	).Scan(&orderID); err != nil {
		return 0, err
	}

	for _, it := range items {
		_, err := tx.ExecContext(
			ctx,
			`INSERT INTO OrderItems (OrderID, ProductID, Quantity, Subtotal)
			 SELECT
			   $1 AS OrderID,
			   p.ProductID,
			   $2::int AS Quantity,
			   ($2::int) * p.Price AS Subtotal
			 FROM Products p
			 WHERE p.ProductID = $3`,
			orderID, it.Quantity, it.ProductID,
		)
		if err != nil {
			return 0, err
		}
	}

	_, err = tx.ExecContext(
		ctx,
		`UPDATE Orders o
		 SET TotalAmount =
		   COALESCE((
		     SELECT SUM(oi.Subtotal)
		     FROM OrderItems oi
		     WHERE oi.OrderID = o.OrderID
		   ), 0)
		 WHERE o.OrderID = $1`,
		orderID,
	)
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return orderID, nil
}

func updateCustomerEmailTx(ctx context.Context, db *sql.DB, customerID int64, newEmail string) error {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(
		ctx,
		`UPDATE Customers
		 SET Email = $1
		 WHERE CustomerID = $2`,
		newEmail, customerID,
	)
	if err != nil {
		return err
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("update expected 1 row, got %d", affected)
	}

	return tx.Commit()
}

func addProductTx(ctx context.Context, db *sql.DB, productName string, price string) (int64, error) {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var productID int64
	if err := tx.QueryRowContext(
		ctx,
		`INSERT INTO Products (ProductName, Price)
		 VALUES ($1, $2)
		 RETURNING ProductID`,
		productName, price,
	).Scan(&productID); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return productID, nil
}

func ensureDemoData(ctx context.Context, db *sql.DB) (customerID int64, productIDs []int64, err error) {
	type customerSeed struct {
		first string
		last  string
		email string
	}
	cSeed := customerSeed{"PASHA", "Ivanova", "ppppp@example.com"}

	if err := db.QueryRowContext(
		ctx,
		`INSERT INTO Customers (FirstName, LastName, Email)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (Email) DO UPDATE
		   SET FirstName = EXCLUDED.FirstName,
		       LastName = EXCLUDED.LastName
		 RETURNING CustomerID`,
		cSeed.first, cSeed.last, cSeed.email,
	).Scan(&customerID); err != nil {
		return 0, nil, err
	}

	products := []struct {
		name  string
		price string
	}{
		{"Product A", "10.00"},
		{"Product B", "25.50"},
	}

	productIDs = make([]int64, 0, len(products))
	for _, p := range products {
		var pid int64
		if err := db.QueryRowContext(
			ctx,
			`INSERT INTO Products (ProductName, Price)
			 VALUES ($1, $2)
			 ON CONFLICT (ProductName) DO UPDATE
			   SET Price = EXCLUDED.Price
			 RETURNING ProductID`,
			p.name, p.price,
		).Scan(&pid); err != nil {
			return 0, nil, err
		}
		productIDs = append(productIDs, pid)
	}

	return customerID, productIDs, nil
}

func main() {
	 _ = godotenv.Load() 

    dsn := os.Getenv("DATABASE_URL")
    if dsn == "" {
        log.Fatal("DATABASE_URL is required")
    }

	ctx := context.Background()

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// На старте контейнеров БД может быть не готова сразу
	for i := 0; i < 20; i++ {
		if err := db.PingContext(ctx); err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	customerID, productIDs, err := ensureDemoData(ctx, db)
	if err != nil {
		log.Fatal(err)
	}

	//Сценарий 1
	orderID, err := placeOrderTx(ctx, db, customerID, []OrderItemInput{
		{ProductID: productIDs[0], Quantity: 2},
		{ProductID: productIDs[1], Quantity: 1},
	})
	if err != nil {
		log.Fatal("Scenario 1 failed:", err)
	}

	var total string
	if err := db.QueryRowContext(ctx, `SELECT TotalAmount::text FROM Orders WHERE OrderID = $1`, orderID).Scan(&total); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Scenario 1 OK: OrderID=%d TotalAmount=%s\n", orderID, total)

	//Сценарий 2
	newEmail := fmt.Sprintf("alena+%d@example.com", time.Now().Unix())
	if err := updateCustomerEmailTx(ctx, db, customerID, newEmail); err != nil {
		log.Fatal("Scenario 2 failed:", err)
	}
	fmt.Println("Scenario 2 OK: Email updated to", newEmail)

	//Сценарий 3
	productName := fmt.Sprintf("Product-%d", time.Now().Unix())
	price := "13.37"

	productID, err := addProductTx(ctx, db, productName, price)
	if err != nil {
		log.Fatal("Scenario 3 failed:", err)
	}
	fmt.Printf("Scenario 3 OK: ProductID=%d Name=%s Price=%s\n", productID, productName, price)
}