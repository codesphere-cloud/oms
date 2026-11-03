// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/credentials"
	"github.com/codesphere-cloud/oms/internal/util"
)

const (
	cloudPlatformScope   = "https://www.googleapis.com/auth/cloud-platform"
	tokenFetchAttempts   = 5
	tokenFetchRetryDelay = 5 * time.Second
)

// detectCredentials resolves the default credentials once so every GCP client shares one
// cached token instead of minting its own per call.
func detectCredentials(t util.Time, onRetry func()) (*auth.Credentials, error) {
	detected, err := credentials.DetectDefault(&credentials.DetectOptions{Scopes: []string{cloudPlatformScope}})
	if err != nil {
		return nil, fmt.Errorf("failed to detect GCP credentials: %w", err)
	}

	return withTokenFetchRetry(detected, t, onRetry), nil
}

func withTokenFetchRetry(creds *auth.Credentials, t util.Time, onRetry func()) *auth.Credentials {
	return auth.NewCredentials(&auth.CredentialsOptions{
		TokenProvider:          &retryingTokenProvider{provider: creds, time: t, onRetry: onRetry},
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
	time     util.Time
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
			p.time.Sleep(tokenFetchRetryDelay)
		}
	}

	return nil, fmt.Errorf("failed to fetch GCP access token after %d attempts: %w", tokenFetchAttempts, err)
}
