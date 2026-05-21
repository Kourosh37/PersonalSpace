package model

import "time"

type User struct {
	ID               string    `json:"id"`
	Username         string    `json:"username"`
	Role             string    `json:"role"`
	IsActive         bool      `json:"isActive"`
	StorageQuota     *int64    `json:"storageQuotaBytes,omitempty"`
	UsedStorageBytes int64     `json:"usedStorageBytes"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}