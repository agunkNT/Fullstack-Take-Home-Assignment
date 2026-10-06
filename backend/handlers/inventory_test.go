package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"indico-backend/db"
	"indico-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func setupRouter() (*gin.Engine, *InventoryHandler) {
	_ = godotenv.Load("../.env") // load if exists

	dbInstance := db.InitDB()
	dbInstance.AutoMigrate(&models.InventoryItem{}, &models.Reservation{})
	
	// Reset data for testing
	dbInstance.Exec("TRUNCATE TABLE inventory_items, reservations")
	db.SeedData(dbInstance)

	router := gin.Default()
	h := NewInventoryHandler(dbInstance)

	router.POST("/api/v1/inventory/reserve", h.ReserveStock)
	router.POST("/api/v1/inventory/confirm", h.ConfirmReservation)
	router.GET("/api/v1/inventory/stock", h.CheckInventoryStatus)

	return router, h
}

func TestConcurrentReservations_StressTest(t *testing.T) {
	router, h := setupRouter()

	// Initial stock should be 100
	var initialItem models.InventoryItem
	h.DB.First(&initialItem, "item_id = ?", "item_4021")
	if initialItem.TotalStock != 100 || initialItem.AvailableStock != 100 {
		t.Fatalf("Expected initial stock to be 100, got %d", initialItem.AvailableStock)
	}

	// We have 100 items available. We will fire 150 concurrent requests, each asking for 1 item.
	// Only exactly 100 should succeed, and 50 should fail due to insufficient stock.
	concurrentRequests := 150
	
	var wg sync.WaitGroup
	wg.Add(concurrentRequests)

	successCount := 0
	failCount := 0
	var mu sync.Mutex

	for i := 0; i < concurrentRequests; i++ {
		go func(userID string) {
			defer wg.Done()

			reqBody := models.ReserveRequest{
				UserID:   userID,
				ItemID:   "item_4021",
				Quantity: 1,
			}
			jsonValue, _ := json.Marshal(reqBody)

			req, _ := http.NewRequest("POST", "/api/v1/inventory/reserve", bytes.NewBuffer(jsonValue))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			mu.Lock()
			if w.Code == http.StatusOK {
				successCount++
			} else {
				failCount++
			}
			mu.Unlock()
		}(string(rune(i)))
	}

	wg.Wait()

	if successCount != 100 {
		t.Errorf("Expected exactly 100 successful reservations, got %d", successCount)
	}
	if failCount != 50 {
		t.Errorf("Expected exactly 50 failed reservations, got %d", failCount)
	}

	// Check final database state
	var finalItem models.InventoryItem
	h.DB.First(&finalItem, "item_id = ?", "item_4021")

	if finalItem.AvailableStock != 0 {
		t.Errorf("Expected available stock to be 0, got %d", finalItem.AvailableStock)
	}
	if finalItem.ReservedStock != 100 {
		t.Errorf("Expected reserved stock to be 100, got %d", finalItem.ReservedStock)
	}
}

func TestConfirmReservation(t *testing.T) {
	router, h := setupRouter()

	// 1. Make a reservation
	reqBody := models.ReserveRequest{
		UserID:   "usr_test",
		ItemID:   "item_4021",
		Quantity: 5,
	}
	jsonValue, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/v1/inventory/reserve", bytes.NewBuffer(jsonValue))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var res map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &res)
	resID := res["reservation_id"].(string)

	// 2. Confirm the reservation
	confirmBody := models.ConfirmRequest{
		ReservationID: resID,
	}
	confirmJson, _ := json.Marshal(confirmBody)
	req2, _ := http.NewRequest("POST", "/api/v1/inventory/confirm", bytes.NewBuffer(confirmJson))
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("Expected 200 OK on confirm, got %d", w2.Code)
	}

	// 3. Verify final stock state
	var finalItem models.InventoryItem
	h.DB.First(&finalItem, "item_id = ?", "item_4021")

	if finalItem.TotalStock != 95 { // 100 - 5
		t.Errorf("Expected total stock to be 95, got %d", finalItem.TotalStock)
	}
	if finalItem.ReservedStock != 0 {
		t.Errorf("Expected reserved stock to be 0, got %d", finalItem.ReservedStock)
	}
}

func TestExpirationCleanup(t *testing.T) {
	_, h := setupRouter()

	// Manually create an expired reservation
	expiredRes := models.Reservation{
		ReservationID: "res_expired",
		UserID:        "usr_1",
		ItemID:        "item_4021",
		Quantity:      10,
		Status:        "pending",
		ExpiresAt:     time.Now().Add(-10 * time.Minute), // Expired 10 mins ago
	}
	h.DB.Create(&expiredRes)

	// Manually update inventory to reflect this pending reservation
	h.DB.Exec("UPDATE inventory_items SET available_stock = 90, reserved_stock = 10 WHERE item_id = 'item_4021'")

	// Call cleanup
	h.cleanupExpiredReservations("item_4021")

	// Verify
	var finalItem models.InventoryItem
	h.DB.First(&finalItem, "item_id = ?", "item_4021")
	if finalItem.AvailableStock != 100 {
		t.Errorf("Expected available stock to revert to 100, got %d", finalItem.AvailableStock)
	}
	
	var res models.Reservation
	h.DB.First(&res, "reservation_id = ?", "res_expired")
	if res.Status != "expired" {
		t.Errorf("Expected reservation status to be expired, got %s", res.Status)
	}
}
