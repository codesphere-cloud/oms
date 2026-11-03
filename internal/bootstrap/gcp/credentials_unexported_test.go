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
	"github.com/codesphere-cloud/oms/internal/util"
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

var _ = Describe("Credentials - Unexported", func() {
	var (
		ctx       context.Context
		fakeTime  *util.FakeTime
		startTime time.Time
		retries   int
		unavail   error
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeTime = util.NewFakeTime()
		startTime = fakeTime.Now()
		retries = 0
		unavail = errors.New("credentials: status code 503: upstream connect error")
	})

	countRetry := func() { retries++ }

	Describe("retryingTokenProvider", func() {
		It("returns the token without retrying when the first fetch succeeds", func() {
			inner := &fakeTokenProvider{}
			p := &retryingTokenProvider{provider: inner, time: fakeTime, onRetry: countRetry}

			token, err := p.Token(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(token.Value).To(Equal("minted"))
			Expect(inner.calls).To(Equal(1))
			Expect(retries).To(Equal(0))
		})

		It("returns the token after a transient failure, retrying once with the delay", func() {
			inner := &fakeTokenProvider{failures: []error{unavail}}
			p := &retryingTokenProvider{provider: inner, time: fakeTime, onRetry: countRetry}

			token, err := p.Token(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(token.Value).To(Equal("minted"))
			Expect(inner.calls).To(Equal(2))
			Expect(retries).To(Equal(1))
			Expect(fakeTime.Now().Sub(startTime)).To(Equal(tokenFetchRetryDelay))
		})

		It("fails with the last error once every attempt has failed", func() {
			lastErr := errors.New("credentials: status code 504: upstream request timeout")
			inner := &fakeTokenProvider{failures: []error{unavail, unavail, unavail, unavail, lastErr}}
			p := &retryingTokenProvider{provider: inner, time: fakeTime, onRetry: countRetry}

			_, err := p.Token(ctx)

			Expect(err).To(MatchError(lastErr))
			Expect(err.Error()).To(ContainSubstring("after 5 attempts"))
			Expect(inner.calls).To(Equal(tokenFetchAttempts))
			Expect(retries).To(Equal(tokenFetchAttempts - 1))
			Expect(fakeTime.Now().Sub(startTime)).To(Equal((tokenFetchAttempts - 1) * tokenFetchRetryDelay))
		})

		It("fails without retrying when the context is cancelled", func() {
			cancelled, cancel := context.WithCancel(ctx)
			cancel()

			inner := &fakeTokenProvider{failures: []error{context.Canceled}}
			p := &retryingTokenProvider{provider: inner, time: fakeTime, onRetry: countRetry}

			_, err := p.Token(cancelled)

			Expect(err).To(MatchError(context.Canceled))
			Expect(inner.calls).To(Equal(1))
			Expect(retries).To(Equal(0))
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
			token, err := withTokenFetchRetry(detected, fakeTime, countRetry).Token(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(token.Value).To(Equal("minted"))
			Expect(retries).To(Equal(1))
		})

		It("keeps the project ID, universe domain and JSON of the detected credentials", func() {
			wrapped := withTokenFetchRetry(detected, fakeTime, countRetry)

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
