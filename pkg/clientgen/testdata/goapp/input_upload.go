-- go.mod --
module app

require (
	encore.dev v1.52.1
)

-- encore.app --
{"id": ""}

-- svc/svc.go --
package svc

import (
    "encore.dev/appruntime/apisdk/api"
)

// SingleUploadRequest contains a single file upload with metadata.
type SingleUploadRequest struct {
    File  *api.File `encore:"file"`
    Title string    `json:"title"`
}

// MultiUploadRequest contains multiple file uploads with metadata.
type MultiUploadRequest struct {
    Avatar     *api.File `encore:"file"`
    Background *api.File `encore:"file"`
    Username   string    `json:"username"`
    Bio        string    `json:"bio"`
}

type UploadResponse struct {
    URL      string `json:"url"`
    Filename string `json:"filename"`
}

-- svc/api.go --
package svc

import (
    "context"
)

// SingleUpload handles a single file upload.
//encore:api public upload
func SingleUpload(ctx context.Context, req *SingleUploadRequest) (*UploadResponse, error) {
    return nil, nil
}

// MultiUpload handles multiple file uploads.
//encore:api public upload
func MultiUpload(ctx context.Context, req *MultiUploadRequest) (*UploadResponse, error) {
    return nil, nil
}
