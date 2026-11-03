// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package gcp

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"cloud.google.com/go/auth"
	"github.com/codesphere-cloud/oms/internal/bootstrap"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type fakeTokenProvider struct {
	failures []error
	calls    int
}

func (f *fakeTokenProvider) Token(context.Context) (*auth.Token, error) {
	f.calls++
	if f.calls <= len(f.failures) {
		return nil, f.failures[f.calls-1]
	}

	return &auth.Token{Value: "minted"}, nil
}

type fakeWaiter struct {
	waited time.Duration
}

func (f *fakeWaiter) wait(_ context.Context, d time.Duration) error {
	f.waited += d
	return nil
}

var _ = Describe("Credentials - Unexported", func() {
	var (
		ctx     context.Context
		waiter  *fakeWaiter
		retries int
		unavail error
	)

	BeforeEach(func() {
		ctx = context.Background()
		waiter = &fakeWaiter{}
		retries = 0
		unavail = errors.New("credentials: status code 503: upstream connect error")
	})

	countRetry := func() { retries++ }

	Describe("retryingTokenProvider", func() {
		It("returns the token without retrying when the first fetch succeeds", func() {
			inner := &fakeTokenProvider{}
			p := &retryingTokenProvider{provider: inner, wait: waiter.wait, onRetry: countRetry}

			token, err := p.Token(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(token.Value).To(Equal("minted"))
			Expect(inner.calls).To(Equal(1))
			Expect(retries).To(Equal(0))
		})

		It("returns the token after a transient failure, retrying once with the delay", func() {
			inner := &fakeTokenProvider{failures: []error{unavail}}
			p := &retryingTokenProvider{provider: inner, wait: waiter.wait, onRetry: countRetry}

			token, err := p.Token(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(token.Value).To(Equal("minted"))
			Expect(inner.calls).To(Equal(2))
			Expect(retries).To(Equal(1))
			Expect(waiter.waited).To(Equal(tokenFetchRetryDelay))
		})

		It("fails with the last error once every attempt has failed", func() {
			lastErr := errors.New("credentials: status code 504: upstream request timeout")
			inner := &fakeTokenProvider{failures: []error{unavail, unavail, unavail, unavail, lastErr}}
			p := &retryingTokenProvider{provider: inner, wait: waiter.wait, onRetry: countRetry}

			_, err := p.Token(ctx)

			Expect(err).To(MatchError(lastErr))
			Expect(err.Error()).To(ContainSubstring("after 5 attempts"))
			Expect(inner.calls).To(Equal(tokenFetchAttempts))
			Expect(retries).To(Equal(tokenFetchAttempts - 1))
			Expect(waiter.waited).To(Equal((tokenFetchAttempts - 1) * tokenFetchRetryDelay))
		})

		It("fails without retrying when the context is cancelled before the first fetch", func() {
			cancelled, cancel := context.WithCancel(ctx)
			cancel()

			inner := &fakeTokenProvider{failures: []error{context.Canceled}}
			p := &retryingTokenProvider{provider: inner, wait: waiter.wait, onRetry: countRetry}

			_, err := p.Token(cancelled)

			Expect(err).To(MatchError(context.Canceled))
			Expect(inner.calls).To(Equal(1))
			Expect(retries).To(Equal(0))
		})

		It("stops without another fetch when the context is cancelled during the wait", func() {
			cancellable, cancel := context.WithCancel(ctx)
			cancelWhileWaiting := func(waitCtx context.Context, _ time.Duration) error {
				cancel()
				return waitCtx.Err()
			}
			inner := &fakeTokenProvider{failures: []error{unavail}}
			p := &retryingTokenProvider{provider: inner, wait: cancelWhileWaiting, onRetry: countRetry}

			_, err := p.Token(cancellable)

			Expect(err).To(MatchError(context.Canceled))
			Expect(err).To(MatchError(unavail))
			Expect(inner.calls).To(Equal(1))
		})
	})

	Describe("waitOrCancel", func() {
		It("returns the context error at once when the context is already cancelled", func() {
			cancelled, cancel := context.WithCancel(ctx)
			cancel()

			Expect(waitOrCancel(cancelled, time.Hour)).To(MatchError(context.Canceled))
		})

		It("returns nil once the delay has elapsed", func() {
			Expect(waitOrCancel(ctx, 0)).To(Succeed())
		})
	})

	Describe("withTokenFetchRetry", func() {
		var (
			detected *auth.Credentials
			inner    *fakeTokenProvider
		)

		BeforeEach(func() {
			inner = &fakeTokenProvider{failures: []error{unavail}}
			detected = auth.NewCredentials(&auth.CredentialsOptions{
				TokenProvider:          inner,
				JSON:                   []byte(`{"type":"external_account"}`),
				ProjectIDProvider:      auth.CredentialsPropertyFunc(func(context.Context) (string, error) { return "oms-infra", nil }),
				UniverseDomainProvider: auth.CredentialsPropertyFunc(func(context.Context) (string, error) { return "example.test", nil }),
			})
		})

		It("retries a failed token fetch of the detected credentials", func() {
			token, err := withTokenFetchRetry(detected, waiter.wait, countRetry).Token(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(token.Value).To(Equal("minted"))
			Expect(retries).To(Equal(1))
		})

		It("keeps the project ID, universe domain and JSON of the detected credentials", func() {
			wrapped := withTokenFetchRetry(detected, waiter.wait, countRetry)

			projectID, err := wrapped.ProjectID(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(projectID).To(Equal("oms-infra"))

			universe, err := wrapped.UniverseDomain(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(universe).To(Equal("example.test"))
			Expect(wrapped.JSON()).To(Equal(detected.JSON()))
		})
	})

	Describe("NewGCPClient", func() {
		It("fails when the credentials file does not exist", func() {
			missing := filepath.Join(GinkgoT().TempDir(), "missing.json")
			GinkgoT().Setenv("GOOGLE_APPLICATION_CREDENTIALS", missing)

			client, err := NewGCPClient(ctx, bootstrap.NewStepLogger(false), missing)

			Expect(err).To(MatchError(ContainSubstring("failed to detect GCP credentials")))
			Expect(client).To(BeNil())
		})
	})
})
