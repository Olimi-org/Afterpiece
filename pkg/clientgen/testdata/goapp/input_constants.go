-- go.mod --
module app

-- encore.app --
{"id": ""}

-- svc/svc.go --
package svc

type Request struct {
	Message string
}

-- svc/api.go --
package svc

import (
	"context"
	"net/http"
)

type Response struct {
	Status Status
}

// DummyAPI is a dummy endpoint.
//encore:api public
func DummyAPI(ctx context.Context, req *Request) (*Response, error) {
	if req.Message == "" {
		return &Response{Status: StatusInactive}, ErrInvalidInput
	}
    return &Response{Status: StatusActive}, nil
}

//encore:export
const (
	APIVersion  = "1.0.0"
	MaxRetries  = 3
	DebugMode   = false
	DefaultPort = 8080
)

//encore:export
const TimeoutSeconds = 30

// Status is a user status
type Status int

const (
	StatusActive Status = iota
	StatusInactive
	StatusPending
	StatusDeleted
)

// Priority represents importance level
type Priority int

//encore:export
const (
	PriorityLow Priority = iota
	PriorityMedium
	PriorityHigh
)

// ErrorCode represents an error categor
type ErrorCode string

//encore:export
const (
	ErrorCodeInvalidInput = "INVALID_INPUT"
	ErrorCodeNotFound     = "NOT_FOUND"
	ErrorCodeServerError  = "SERVER_ERROR"
)

// MultiNameMultiType tests multi-name const specs with different types
//encore:export
const (
	MultiNameMultiTypeA, MultiNameMultiTypeB = 100, 200
)

// MultiNameStrings tests multi-name const specs with strings
//encore:export
const (
	MultiNameStringX, MultiNameStringY, MultiNameStringZ = "x", "y", "z"
)

// MultiNameWithExplicitType tests multi-name with explicit type annotation
//encore:export
const (
	MultiNameWithExplicitTypeAlpha, MultiNameWithExplicitTypeBeta int = 10, 20
)

// IotaInheritance tests that inherited values work (like iota-based enums)
//encore:export
const (
	IotaInheritanceFirst = iota
	IotaInheritanceSecond
	IotaInheritanceThird
)
