package uploads

import "io"

type BufferedWriter struct {
	buf        []byte
	buf_idx    uint64
	buf_length uint64

	uploader CloudUploader

	// Total amount of data stored
	total uint64
}

// Flush a buffer into the uploader
func (self *BufferedWriter) Flush() error {
	err := self.uploader.Put(self.buf[:self.buf_idx])
	if err != nil {
		return err
	}
	self.buf_idx = 0

	return nil
}

func (self *BufferedWriter) Close() error {
	err := self.Flush()
	if err != nil {
		return err
	}

	self.uploader.Commit()
	return self.uploader.Close()
}

func (self *BufferedWriter) Copy(reader io.Reader, length uint64) error {
	for length > 0 {
		// How much space is left in the buffer
		to_read := length
		if to_read > self.buf_length-self.buf_idx {
			to_read = self.buf_length - self.buf_idx
		}

		n, err := reader.Read(self.buf[self.buf_idx : self.buf_idx+to_read])
		if err != nil && err != io.EOF && n == 0 {
			return err
		}

		if n == 0 {
			return nil
		}

		self.total += uint64(n)
		self.buf_idx += uint64(n)
		length -= uint64(n)

		// Buffer is full - flush it
		if self.buf_idx >= self.buf_length {
			err = self.Flush()
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func NewBufferWriter(uploader CloudUploader) *BufferedWriter {
	return &BufferedWriter{
		buf:        make([]byte, BUFF_SIZE),
		buf_length: uint64(BUFF_SIZE),
		uploader:   uploader,
	}
}
