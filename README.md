# Fullstack Take-Home Assignment

A high-throughput inventory reservation system built for high concurrency. This project consists of a React (Vite) Frontend, a Go (Gin) Backend, and a PostgreSQL database.

## 🚀 Features
- **Live Inventory Tracker**: View real-time stock limits (Available, Reserved, Total).
- **Concurrent Safe**: Implements atomic SQL queries to prevent overselling and race conditions.
- **Two-Phase Checkout**: Temporary stock reservation with auto-expiry.
- **Lazy Expiry Cleanup**: JIT-based background sweeping for expired items without heavy cron jobs.

---

## 🛠️ Technology Stack
- **Frontend**: React.js, Vite, Axios, Tailwind-like custom CSS.
- **Backend**: Go (Golang), Gin Framework, GORM.
- **Database**: PostgreSQL 15 (Docker).

---

## ⚙️ Prerequisites
To run this project on your local machine, ensure you have the following installed:
- [Docker & Docker Compose](https://www.docker.com/)
- [Go (v1.20+)](https://go.dev/)
- [Node.js (v18+)](https://nodejs.org/) & NPM

---

## 📦 How to Run the Project

### 1. Start the Database
The PostgreSQL database runs in a Docker container and is automatically initialized.
```bash
# In the root directory of the project
docker compose up -d
```
> **Note:** The database handles its own initial seeding of products (`item_4021`, `item_8888`, `item_9999`) upon the first backend connection.

### 2. Start the Backend (Go)
```bash
cd backend
go mod tidy
go run main.go
```
The Go server will start on **http://localhost:8080**.

### 3. Start the Frontend (React)
Open a new terminal window:
```bash
cd frontend
npm install
npm run dev
```
The React frontend will be accessible at **http://localhost:5173** (or 5174).

---

## 🧪 Running Tests (Backend)
The backend codebase includes stress tests covering concurrent transactions to guarantee that race conditions do not occur. 

To run the unit tests and race detection (requires `CGO_ENABLED=1` on Windows):
```bash
cd backend
go test -race -v ./...
```

---

## 📂 Architecture
For detailed information regarding system design, graceful shutdown, atomic locking logic, and expiry handling, please refer to the **[ARCHITECTURE.md](./ARCHITECTURE.md)** file included in this repository.
