// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"errors"
	"strings"

	"google.golang.org/api/googleapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// IsNotFoundError checks if the error indicates a "not found" condition.
// It handles both gRPC status errors (from Compute API) and googleapi.Error (from DNS/HTTP APIs).
func IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	if status.Code(err) == codes.NotFound {
		return true
	}
	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) {
		return apiErr.Code == 404
	}
	return false
}

// IsSpotCapacityError checks if the error is related to spot VM capacity issues.
func IsSpotCapacityError(err error) bool {
	if err == nil {
		return false
	}
	if status.Code(err) == codes.ResourceExhausted {
		return true
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "zone_resource_pool_exhausted") ||
		strings.Contains(errStr, "unsupported_operation") ||
		strings.Contains(errStr, "stockout") ||
		strings.Contains(errStr, "does not have enough resources")
}

// IsAlreadyExistsError checks if the error indicates the resource already exists.
func IsAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	return status.Code(err) == codes.AlreadyExists || strings.Contains(err.Error(), "already exists")
}
