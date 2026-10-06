package handlers

import (
	"fmt"
	"net/http"
	"time"

	"indico-backend/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type InventoryHandler struct {
	DB *gorm.DB
}

func NewInventoryHandler(db *gorm.DB) *InventoryHandler {
	return &InventoryHandler{DB: db}
}

// 1. Reserve Stock
func (h *InventoryHandler) ReserveStock(c *gin.Context) {
	var req models.ReserveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid request payload"})
		return
	}

	tx := h.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Could not start transaction"})
		return
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Atomic update using RETURNING to ensure concurrency control
	var item models.InventoryItem
	result := tx.Raw(`
		UPDATE inventory_items 
		SET available_stock = available_stock - ?, reserved_stock = reserved_stock + ? 
		WHERE item_id = ? AND available_stock >= ? 
		RETURNING item_id, total_stock, reserved_stock, available_stock`, 
		req.Quantity, req.Quantity, req.ItemID, req.Quantity).Scan(&item)

	if result.Error != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Database error during reservation"})
		return
	}

	if result.RowsAffected == 0 {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"status": "error", "message": "Insufficient stock or item not found"})
		return
	}

	// Create Reservation record
	expiresAt := time.Now().Add(5 * time.Minute)
	reservationID := fmt.Sprintf("res_%d", time.Now().UnixNano()) // In a real system, use UUID

	reservation := models.Reservation{
		ReservationID: reservationID,
		UserID:        req.UserID,
		ItemID:        req.ItemID,
		Quantity:      req.Quantity,
		Status:        "pending",
		ExpiresAt:     expiresAt,
	}

	if err := tx.Create(&reservation).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Failed to record reservation"})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Transaction commit failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":         "success",
		"reservation_id": reservation.ReservationID,
		"item_id":        reservation.ItemID,
		"quantity":       reservation.Quantity,
		"expires_at":     reservation.ExpiresAt.Format(time.RFC3339),
	})
}

// 2. Confirm Reservation
func (h *InventoryHandler) ConfirmReservation(c *gin.Context) {
	var req models.ConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid request payload"})
		return
	}

	tx := h.DB.Begin()
	
	// Lock the reservation row
	var reservation models.Reservation
	if err := tx.Raw("SELECT * FROM reservations WHERE reservation_id = ? FOR UPDATE", req.ReservationID).Scan(&reservation).Error; err != nil || reservation.ReservationID == "" {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "message": "Reservation not found"})
		return
	}

	if reservation.Status != "pending" {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Reservation is not in pending state"})
		return
	}

	if time.Now().After(reservation.ExpiresAt) {
		tx.Rollback()
		c.JSON(http.StatusGone, gin.H{"status": "error", "message": "Reservation has expired"})
		return
	}

	// Update reservation status
	now := time.Now()
	reservation.Status = "confirmed"
	reservation.ConfirmedAt = &now
	
	if err := tx.Save(&reservation).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Failed to update reservation"})
		return
	}

	// Update inventory (convert reserved to confirmed/deducted)
	result := tx.Exec(`
		UPDATE inventory_items 
		SET reserved_stock = reserved_stock - ?, total_stock = total_stock - ? 
		WHERE item_id = ?`, 
		reservation.Quantity, reservation.Quantity, reservation.ItemID)

	if result.Error != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Failed to update inventory"})
		return
	}

	tx.Commit()

	c.JSON(http.StatusOK, gin.H{
		"status":         "success",
		"reservation_id": reservation.ReservationID,
		"confirmed_at":   reservation.ConfirmedAt.Format(time.RFC3339),
	})
}

// 3. Check Inventory Status
func (h *InventoryHandler) CheckInventoryStatus(c *gin.Context) {
	itemID := c.Query("item_id")
	if itemID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Missing item_id parameter"})
		return
	}

	// Run cleanup logic first to release expired stock before checking
	// In production, this should run asynchronously via a background worker or cron,
	// but here we do it on demand (or can use a goroutine).
	h.cleanupExpiredReservations(itemID)

	var item models.InventoryItem
	if err := h.DB.Where("item_id = ?", itemID).First(&item).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "message": "Item not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"item_id":         item.ItemID,
		"total_stock":     item.TotalStock,
		"reserved_stock":  item.ReservedStock,
		"available_stock": item.AvailableStock,
	})
}

// Cleanup helper function
func (h *InventoryHandler) cleanupExpiredReservations(itemID string) {
	// Find expired pending reservations
	var expiredReservations []models.Reservation
	h.DB.Where("item_id = ? AND status = ? AND expires_at < ?", itemID, "pending", time.Now()).Find(&expiredReservations)

	for _, res := range expiredReservations {
		tx := h.DB.Begin()
		
		// Attempt to update status to expired
		result := tx.Exec("UPDATE reservations SET status = 'expired' WHERE reservation_id = ? AND status = 'pending'", res.ReservationID)
		
		// Only restore stock if we successfully updated the row (prevents double counting if running concurrently)
		if result.RowsAffected > 0 {
			tx.Exec(`
				UPDATE inventory_items 
				SET reserved_stock = reserved_stock - ?, available_stock = available_stock + ? 
				WHERE item_id = ?`, 
				res.Quantity, res.Quantity, res.ItemID)
		}
		
		tx.Commit()
	}
}

// 4. Get Reservation History
func (h *InventoryHandler) GetHistory(c *gin.Context) {
	itemID := c.Query("item_id")
	if itemID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Missing item_id parameter"})
		return
	}

	var reservations []models.Reservation
	if err := h.DB.Where("item_id = ?", itemID).Order("expires_at DESC").Find(&reservations).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Failed to fetch history"})
		return
	}

	c.JSON(http.StatusOK, reservations)
}
