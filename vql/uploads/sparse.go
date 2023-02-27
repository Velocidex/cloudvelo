package uploads

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"

	"www.velocidex.com/golang/velociraptor/accessors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/uploads"
)

func UploadSparse(
	ctx context.Context,
	ospath *accessors.OSPath,
	idx_uploader CloudUploader,
	uploader CloudUploader,
	range_reader uploads.RangeReader) (*uploads.UploadResponse, error) {

	index := &actions_proto.Index{}

	// This is the response that will be passed into the VQL
	// engine.
	result := &uploads.UploadResponse{
		Path: ospath.String(),
	}

	md5_sum := md5.New()
	sha_sum := sha256.New()

	// Does the index contain any sparse runs?
	is_sparse := false

	// Adjust the expected size properly to the sum of all
	// non-sparse ranges and build the index file.
	ranges := range_reader.Ranges()

	// Inspect the ranges and prepare an index.
	expected_size := int64(0)
	real_size := int64(0)
	for _, rng := range ranges {
		file_length := rng.Length
		if rng.IsSparse {
			file_length = 0
		}

		index.Ranges = append(index.Ranges,
			&actions_proto.Range{
				FileOffset:     expected_size,
				OriginalOffset: rng.Offset,
				FileLength:     file_length,
				Length:         rng.Length,
			})

		if !rng.IsSparse {
			expected_size += rng.Length
		} else {
			is_sparse = true
		}

		if real_size < rng.Offset+rng.Length {
			real_size = rng.Offset + rng.Length
		}
	}

	// We need to buffer writes untilt they reach 5mb before we can
	// send them. This is managed by the BufferedWriter object which
	// wraps the uploader.
	buffer := NewBufferWriter(uploader)
	defer buffer.Close()

	for _, rng := range ranges {
		if rng.IsSparse {
			continue
		}

		_, err := range_reader.Seek(rng.Offset, 0)
		if err != nil {
			return nil, err
		}

		err = buffer.Copy(range_reader, uint64(rng.Length))
		if err != nil {
			return nil, err
		}
	}

	// If we are sparse upload the sparse file in one part.
	if is_sparse {
		serialized, err := json.Marshal(index)
		if err != nil {
			return nil, err
		}

		err = idx_uploader.Put(serialized)
		if err != nil {
			return nil, err
		}

		idx_uploader.Commit()

		// Set the index on the actual uploader
		uploader.SetIndex(index)
	}

	result.Size = uint64(real_size)

	// The actual amount of bytes uploaded
	result.StoredSize = buffer.total
	result.Sha256 = hex.EncodeToString(sha_sum.Sum(nil))
	result.Md5 = hex.EncodeToString(md5_sum.Sum(nil))

	return result, nil
}
