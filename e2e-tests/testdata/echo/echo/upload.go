package echo

import (
	"context"
	"io"

	"encore.dev/appruntime/apisdk/api"
)

// UploadRequest contains a single file upload with metadata.
type UploadRequest struct {
	File  *api.File `encore:"file"`
	Title string    `json:"title"`
}

// UploadResponse returns metadata about the uploaded file.
type UploadResponse struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int    `json:"size"`
	Title       string `json:"title"`
	Content     string `json:"content"`
}

// Upload handles a single file upload and echoes back file metadata and content.
//
//encore:api public upload
func Upload(ctx context.Context, req *UploadRequest) (*UploadResponse, error) {
	var content []byte
	if req.File != nil && req.File.Reader != nil {
		var err error
		content, err = io.ReadAll(req.File.Reader)
		if err != nil {
			return nil, err
		}
	}

	resp := &UploadResponse{
		Title:   req.Title,
		Content: string(content),
	}
	if req.File != nil {
		resp.Filename = req.File.Filename
		resp.ContentType = req.File.ContentType
		resp.Size = len(content)
	}
	return resp, nil
}
