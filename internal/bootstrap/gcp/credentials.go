// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/credentials"
)

const (
	cloudPlatformScope   = "https://www.googleapis.com/auth/cloud-platform"
	tokenFetchAttempts   = 5
	tokenFetchRetryDelay = 5 * time.Second
)

// detectCredentials resolves the default credentials once so every GCP client shares one
// cached token instead of minting its own per call.
func detectCredentials(onRetry func()) (*auth.Credentials, error) {
	detected, err := credentials.DetectDefault(&credentials.DetectOptions{Scopes: []string{cloudPlatformScope}})
	if err != nil {
		return nil, fmt.Errorf("failed to detect GCP credentials: %w", err)
	}

	return withTokenFetchRetry(detected, waitOrCancel, onRetry), nil
}

func withTokenFetchRetry(creds *auth.Credentials, wait waitFunc, onRetry func()) *auth.Credentials {
	return auth.NewCredentials(&auth.CredentialsOptions{
		TokenProvider:          &retryingTokenProvider{provider: creds, wait: wait, onRetry: onRetry},
		JSON:                   creds.JSON(),
		ProjectIDProvider:      auth.CredentialsPropertyFunc(creds.ProjectID),
		QuotaProjectIDProvider: auth.CredentialsPropertyFunc(creds.QuotaProjectID),
		UniverseDomainProvider: auth.CredentialsPropertyFunc(creds.UniverseDomain),
	})
}

// retryingTokenProvider retries every failed token fetch. With workload identity federation a
// token takes three requests (OIDC token, STS exchange, impersonation), and their errors carry no
// typed status, so transient 5xx cannot be told apart from permanent failures.
type retryingTokenProvider struct {
	provider auth.TokenProvider
	wait     waitFunc
	onRetry  func()
}

func (p *retryingTokenProvider) Token(ctx context.Context) (*auth.Token, error) {
	var err error

	for attempt := range tokenFetchAttempts {
		var token *auth.Token

		token, err = p.provider.Token(ctx)
		if err == nil {
			return token, nil
		}

		if ctx.Err() != nil {
			return nil, fmt.Errorf("failed to fetch GCP access token: %w", err)
		}

		if attempt < tokenFetchAttempts-1 {
			p.onRetry()

			if waitErr := p.wait(ctx, tokenFetchRetryDelay); waitErr != nil {
				return nil, fmt.Errorf("stopped retrying GCP access token fetch: %w (last error: %w)", waitErr, err)
			}
		}
	}

	return nil, fmt.Errorf("failed to fetch GCP access token after %d attempts: %w", tokenFetchAttempts, err)
}

type waitFunc func(ctx context.Context, d time.Duration) error

func waitOrCancel(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return fmt.Errorf("wait cancelled: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}
