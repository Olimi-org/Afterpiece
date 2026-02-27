package api

import "io"

// File represents a file in an upload API endpoint.
// It contains the streaming file content and associated metadata.
//
// File fields must be tagged with `encore:"file"` in the request struct.
//
// Example:
//
//	type UploadRequest struct {
//	    Avatar *api.File `encore:"file"`
//	    Description string `json:"description"`
//	}
//
//	//encore:api public upload
//	func Upload(ctx context.Context, req *UploadRequest) error { ... }
type File struct {
	// Reader provides streaming access to the file content.
	// It reads directly from the multipart body without buffering the entire file.
	Reader io.Reader

	// Filename is the original filename as provided by the client.
	Filename string

	// ContentType is the MIME type of the file, as declared by the client
	// or detected from the file content.
	ContentType string

	// Size is the size of the file in bytes.
	// It is -1 if the size is not known.
	Size int64
}
