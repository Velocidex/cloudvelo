package utils

import (
	"encoding/hex"

	"github.com/google/uuid"
)

func GetId() string {
	u := uuid.New()
	return hex.EncodeToString(u[0:8])
}
