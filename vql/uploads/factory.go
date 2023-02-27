package uploads

import (
	"sync"
)

var (
	mu               sync.Mutex
	gUploaderFactory CloudUploader
)

func SetUploaderService(uploader CloudUploader) error {
	mu.Lock()
	defer mu.Unlock()

	gUploaderFactory = uploader
	return nil
}
