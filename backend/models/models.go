package models

import (
	"time"
)

type InventoryItem struct {
	ItemID         string `gorm:"primaryKey" json:"item_id"`
	TotalStock     int    `json:"total_stock"`
	ReservedStock  int    `json:"reserved_stock"`
	AvailableStock int    `json:"available_stock"`
	Version        int    `gorm:"default:1"` // For optimistic locking if needed, though we might use direct UPDATE logic
}

type Reservation struct {
	ReservationID string     `gorm:"primaryKey" json:"reservation_id"`
	UserID        string     `json:"user_id"`
	ItemID        string     `json:"item_id"`
	Quantity      int        `json:"quantity"`
	Status        string     `json:"status"` // pending, confirmed, expired
	ExpiresAt     time.Time  `json:"expires_at"`
	ConfirmedAt   *time.Time `json:"confirmed_at,omitempty"`
}

type ReserveRequest struct {
	UserID   string `json:"user_id" binding:"required"`
	ItemID   string `json:"item_id" binding:"required"`
	Quantity int    `json:"quantity" binding:"required,gt=0"`
}

type ConfirmRequest struct {
	ReservationID string `json:"reservation_id" binding:"required"`
}
