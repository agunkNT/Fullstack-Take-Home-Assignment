# System Architecture: High-Throughput Inventory Reservation

## 1. Overview
This project implements a high-throughput, concurrent-safe inventory reservation system. The architecture is designed to handle multiple concurrent users attempting to reserve a limited stock of items, ensuring data integrity, preventing overselling, and managing graceful expiries.

## 2. Technology Stack
- **Frontend**: React.js with Vite (Lightning fast HMR and optimized build).
- **Backend Framework**: Go (Golang) using the Gin HTTP framework for high-performance routing.
- **Database**: PostgreSQL 15, managed via Docker Compose.
- **ORM**: Gorm (Go Object Relational Mapper) for schema migration and query generation.

## 3. Core Mechanisms

### 3.1 Concurrency & Data Integrity
To prevent race conditions (overselling) in a high-throughput environment, the system leverages PostgreSQL's ACID compliance:
- **Atomic Updates**: When reserving stock, the backend does *not* read the stock, subtract it in Go, and save it. Instead, it executes an atomic SQL query:
  ```sql
  UPDATE inventory_items 
  SET available_stock = available_stock - ?, reserved_stock = reserved_stock + ? 
  WHERE item_id = ? AND available_stock >= ? 
  RETURNING *;
  ```
  This ensures that the database lock mechanisms handle concurrency at the row level, rejecting operations that exceed available stock.

### 3.2 Reservation Lifecycle (Two-Phase Checkout)
1. **Reserve (Pending)**: A user requests an item. The system atomically deducts `available_stock` and increments `reserved_stock`. A reservation record is created with a `PENDING` status and an expiration timestamp (e.g., 5 minutes).
2. **Confirm (Success)**: If the user completes the checkout within the time window, the system updates the reservation status to `CONFIRMED` and permanently deducts the quantity from `total_stock` and `reserved_stock`.
3. **Expire (Timeout/Failure)**: If the reservation window elapses, the cleanup mechanism identifies the `PENDING` order, reverts the `reserved_stock` back to `available_stock`, and marks the order as `EXPIRED`.

### 3.3 Expiry Cleanup Strategy
Instead of running a heavy CRON job that constantly queries the database, the system uses a **Lazy/JIT (Just-In-Time) Cleanup approach**:
- Every time an inventory check or reservation request occurs, the system briefly triggers a cleanup function. 
- This function scans for `PENDING` reservations where `expires_at < NOW()`, reverts their stock atomically, and marks them as `EXPIRED`. 
- This guarantees that available stock is strictly accurate the moment a user checks it, without wasting CPU cycles during idle periods.

### 3.4 Graceful Shutdown
To prevent data corruption during container restarts or deployments, the Go server implements a graceful shutdown. It listens for OS signals (SIGINT, SIGTERM) and allows active HTTP requests to finish processing (up to a 5-second timeout) before fully terminating the application and closing database connections.

## 4. API Endpoints
- `GET /api/v1/inventory/stock?item_id={id}`: Fetch live stock status (Total, Reserved, Available).
- `POST /api/v1/inventory/reserve`: Attempt to reserve a quantity of stock.
- `POST /api/v1/inventory/confirm`: Confirm an active reservation.
- `GET /api/v1/inventory/history?item_id={id}`: View the transaction history log.

## 5. Security & Testing
- **CORS Handling**: Properly configured in the Gin middleware to allow isolated frontend access.
- **Stress Testing**: The Go codebase includes `inventory_test.go`, featuring `TestConcurrentReservations_StressTest`. This spawns 100 concurrent goroutines attempting to reserve limited stock simultaneously to validate the atomic lock logic. Verified with Go's built-in race detector (`go test -race`).
