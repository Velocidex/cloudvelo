package api

type NotificationRecord struct {
	Key       string `json:"key,omitempty"`
	Timestamp int64  `json:"timestamp,omitempty"`
}
