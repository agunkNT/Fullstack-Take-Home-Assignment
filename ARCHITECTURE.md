# System Architecture & Technical Defense

This document addresses the three required architectural prompts for the High-Throughput Inventory Reservation System.

---

## 1. Architectural Design & Database Locking Strategy

### 1.1 PostgreSQL Schema Design Defense
The relational model consists of two core tables designed for write-optimized transactional integrity:
- **`inventory_items`** (`item_id` PK, `total_stock`, `reserved_stock`, `available_stock`, `version`):
  - *Design Defense*: We deliberately maintain explicit counters (`available_stock` and `reserved_stock`) directly on the item record rather than dynamically calculating available stock via `total_stock - SUM(pending_reservations)`. Dynamic aggregations require table/index range scans and aggregate locking, which collapses throughput under heavy read/write contention. The materialized counters allow $O(1)$ single-row updates.
- **`reservations`** (`reservation_id` PK, `user_id`, `item_id`, `quantity`, `status`, `expires_at`, `confirmed_at`):
  - *Design Defense*: Provides an immutable audit trail and state machine (`pending` $\rightarrow$ `confirmed` | `expired`). Segregating active inventory from reservation history prevents write contention on the inventory table while retaining traceability.

### 1.2 Database Synchronization & Concurrency Locking Choices
Under high concurrency, naive application-level read-modify-write patterns cause severe **Time-of-Check to Time-of-Use (TOCTOU)** race conditions and overselling. We evaluated three concurrency models:

1. **In-Memory Locks (`sync.Mutex` in Go)**: *Rejected*. Mutexes only synchronize goroutines within a single OS process. In multi-container/distributed environments, in-memory mutexes provide zero cross-instance protection.
2. **Pessimistic Locking (`SELECT ... FOR UPDATE`)**: Used selectively during `ConfirmReservation`. It serializes operations on specific reservation records to prevent double-confirmation or concurrent expiration.
3. **Atomic Conditional Updates (Chosen Strategy for Reservation)**:
   ```sql
   UPDATE inventory_items 
   SET available_stock = available_stock - ?, reserved_stock = reserved_stock + ? 
   WHERE item_id = ? AND available_stock >= ? 
   RETURNING item_id, total_stock, reserved_stock, available_stock;
   ```
   - *Why this choice*: PostgreSQL acquires a row-level exclusive lock (`X-lock`) on the single tuple during execution. The predicate `available_stock >= ?` guarantees at the storage engine level that stock never drops below zero. If available stock is insufficient, `RowsAffected` is `0`, and the transaction cleanly rolls back without acquiring unnecessary locks or causing deadlocks.

### 1.3 Gin Routing & Layered Structure
- **Separation of Concerns**: Divided into `models/` (data contracts & GORM entities), `db/` (connection management & seeding), and `handlers/` (HTTP controller & business logic).
- **API Versioning**: Grouped under `/api/v1/inventory` to support non-breaking contract evolution.
- **Middleware Pipeline**: Configured with CORS, structured logging, and panic recovery.
- **HTTP Semantics**: Clear status codes: `200 OK` (success), `400 Bad Request` (payload error), `404 Not Found` (item missing), `409 Conflict` (insufficient stock), and `410 Gone` (expired reservation).

---

## 2. Distributed Scaling & Failure Modes

### 2.1 What Breaks When Scaled Across 10 Instances?
Pointing 10 horizontal Go service instances directly to a single PostgreSQL instance exposes three critical failure points:

1. **Connection Pool Exhaustion**: Each Go instance typically defaults to 50–100 DB connections (`MaxOpenConns`). Ten instances create up to 1,000 concurrent database connections. PostgreSQL uses a process-per-connection architecture (~10MB RAM per connection), inducing severe context-switching overhead, connection pool exhaustion, and memory starvation on the database.
2. **Hot SKU Row-Lock Serialization**: If thousands of users concurrently contend for the same flash-sale item, all 10 instances fire `UPDATE` statements on the identical row in `inventory_items`. While atomic updates prevent overselling, transactions queue up on row-level locks, causing query queue buildup, connection wait timeouts, and elevated p99 latencies.
3. **JIT / On-Demand Cleanup Redundancy**: The current Just-In-Time (JIT) cleanup scans `reservations` where `expires_at < NOW()` on user queries. Under 10 active instances, multiple nodes concurrently attempt to expire the same reservations, creating redundant database scans and lock contention on the `reservations` table.

### 2.2 Redesign for a Stateless, Distributed Deployment
To scale horizontally to tens or hundreds of instances while maintaining strict consistency:

```
[Clients] -> [Load Balancer]
                  |
    +-------------+-------------+
    |                           |
[Go Node 1]                [Go Node 10]
    |                           |
    +-------------+-------------+
                  |
          [PgBouncer Pooler]
                  |
         [PostgreSQL Master] <--- (Replication) ---> [PostgreSQL Read Replicas]
                  |
          [Redis Cluster] (Distributed Caching & Expiry Queues)
```

1. **Connection Pooling via PgBouncer**:
   Deploy PgBouncer in `transaction` pooling mode in front of PostgreSQL. Go instances connect to PgBouncer, which multiplexes hundreds of client connections into a fixed pool of 50–100 physical PostgreSQL backend connections.
2. **Distributed Stock Reservation (Redis + Lua)**:
   Offload flash-sale hot-row contention to a Redis Cluster. Using an atomic Lua script:
   - Check if `available_stock >= requested_qty`.
   - Decrement in Redis within $O(1)$ sub-millisecond memory time.
   - Enqueue a reservation event into Kafka/RabbitMQ to be asynchronously persisted into PostgreSQL.
3. **Decoupled Expiry Worker (Outbox / Event-Driven Pattern)**:
   Remove JIT cleanup from HTTP request handlers. Implement delayed job queues via Redis TTL keyspace notifications, RabbitMQ Dead-Letter Exchanges (DLX), or a single leader-elected worker (e.g., using `pg_cron` or Temporal) to release expired inventory cleanly.
4. **Read/Write Splitting**:
   Route read-only queries (`/stock`, `/history`) to PostgreSQL Read Replicas, isolating transactional write capacity for reservations.

---

## 3. Engineering Trade-offs & AI Transparency

### 3.1 Trade-offs Made Within the 4–8 Hour Window
| Domain | Decision Made | Trade-Off Rationale & Rejected Alternative |
| :--- | :--- | :--- |
| **Backend ORM** | **GORM with Raw SQL fragments** | *Chosen*: Accelerated schema setup, migrations, and model binding within hours. Critical atomic logic executed via `tx.Raw()` / `tx.Exec()` to bypass ORM overhead.<br>*Rejected*: Pure SQLc / `pgx` would provide higher throughput and compile-time type safety, but requires significantly more boilerplate configuration within a tight timebox. |
| **Cleanup Engine** | **Just-In-Time (JIT) Lazy Cleanup** | *Chosen*: Kept the stack strictly self-contained (Postgres-only) without adding external infrastructure dependencies.<br>*Rejected*: Redis TTL or a background cron worker is architecturally superior for high volume, but adds operational complexity to the test deployment. |
| **Frontend Sync** | **Client Polling + Reactive Timer** | *Chosen*: Predictable, resilient to network drops, simple state management with React hooks.<br>*Rejected*: WebSockets or Server-Sent Events (SSE) offer real-time push updates, but introduce stateful connection management and reconnection complexity. |

### 3.2 AI Transparency: Flawed Suggestion & Resolution
- **The Scenario**:
  During the initial design of the reservation endpoint, an AI assistant was asked to generate a concurrent stock reservation workflow using GORM.
- **The Flawed AI Suggestion**:
  The AI produced standard ORM code that first retrieved the inventory item into Go memory, checked the balance, and updated it:
  ```go
  // Flawed AI Suggestion:
  db.Where("item_id = ?", req.ItemID).First(&item)
  if item.AvailableStock >= req.Quantity {
      item.AvailableStock -= req.Quantity
      item.ReservedStock += req.Quantity
      db.Save(&item)
  }
  ```
- **Why It Was Flawed**:
  This introduced a textbook **Time-of-Check to Time-of-Use (TOCTOU)** race condition. Under concurrent requests, 50 goroutines read the exact same `AvailableStock` simultaneously, all passed the `if` check, and all called `db.Save()`, resulting in catastrophic overselling (negative available stock).
- **How It Was Fixed**:
  We rejected the application-level check entirely and refactored the logic into an **atomic database-level conditional statement** (`UPDATE ... WHERE available_stock >= ? RETURNING *`) wrapped in a transaction with `RowsAffected` verification. We verified this fix by authoring `TestConcurrentReservations_StressTest` in `inventory_test.go`, which executed 100 concurrent goroutines competing for 5 items, proving that exactly 5 succeeded and 95 were cleanly rejected without race conditions.
