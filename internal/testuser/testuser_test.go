// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package testuser

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestSignupCreatesTokenUsingAuthenticatedInternalAPI(t *testing.T) {
	var calls []string

	opts := CreateTestUserOpts{Email: "test+api@codesphere.com", Password: "Aa1!secret"}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}

		var data map[string]any

		if r.URL.Path != "/session/v1/sign-out" {
			if r.Header.Get("Content-Type") != "application/json" {
				t.Error("missing JSON content type")
			}

			if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
				t.Error(err)
			}
		}

		switch r.URL.Path {
		case "/internal/v1/Auth/signUp":
			want := map[string]any{"email": opts.Email, "password": opts.Password, "recaptchaToken": "test"}
			if !reflect.DeepEqual(data["data"], want) {
				t.Error("incorrect signup payload")
			}

			_, _ = fmt.Fprint(w, `{"kind":12,"id":1}`)
		case "/internal/v1/Auth/signIn":
			want := map[string]any{"email": opts.Email, "password": opts.Password}
			if !reflect.DeepEqual(data["data"], want) {
				t.Error("incorrect sign-in payload")
			}

			_, _ = fmt.Fprint(w, `{"kind":12,"id":1,"data":{"sessionId":"session-secret","firstSignin":true}}`)
		case "/session/v1/gen-access-token":
			if data["id"] != "session-secret" {
				t.Error("missing session ID")
			}

			_, _ = fmt.Fprint(w, `{"accessToken":"access-secret","sessionId":"session-secret"}`)
		case "/internal/v1/ApiTokenService/createToken":
			if r.Header.Get("Authorization") != "Bearer access-secret" {
				t.Error("missing access token")
			}

			if !reflect.DeepEqual(data["data"], map[string]any{"name": "oms-test-user"}) {
				t.Error("incorrect token request")
			}

			_, _ = fmt.Fprint(w, `{"kind":12,"id":1,"data":{"token":"CS_from-service"}}`)
		case "/session/v1/sign-out":
			cookie, err := r.Cookie("sessionId")
			if err != nil || cookie.Value != "session-secret" {
				t.Error("missing logout cookie")
			}

			_, _ = fmt.Fprint(w, `{}`)
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
		}
	}))
	defer server.Close()

	result, err := createViaAPI(context.Background(), server.URL, opts)
	if err != nil {
		t.Fatal(err)
	}

	if result.Email != opts.Email || result.PlaintextPassword != opts.Password || result.PlaintextAPIToken != "CS_from-service" {
		t.Error("credentials differ from signup and token response")
	}

	want := []string{"/internal/v1/Auth/signUp", "/internal/v1/Auth/signIn", "/session/v1/gen-access-token", "/internal/v1/ApiTokenService/createToken", "/session/v1/sign-out"}
	if !reflect.DeepEqual(calls, want) {
		t.Errorf("calls = %v", calls)
	}
}

func TestSignupFailureStopsFlowAndDoesNotExposeResponseBody(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"HTTP failure", 500, `password-secret access-secret database-details`},
		{"RPC failure", 200, `{"kind":13,"id":1,"error":{"name":"InvalidArgument","data":"password-secret"}}`},
		{"malformed response", 200, `password-secret`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls++

				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			}))
			defer server.Close()

			_, err := createViaAPI(context.Background(), server.URL, CreateTestUserOpts{Email: "test@codesphere.com", Password: "password-secret"})
			if err == nil || !strings.Contains(err.Error(), "signup failed") {
				t.Fatalf("unexpected error %v", err)
			}

			if strings.Contains(err.Error(), "password-secret") || strings.Contains(err.Error(), "access-secret") {
				t.Error("error exposes secrets")
			}

			if calls != 1 {
				t.Errorf("made %d calls after failed signup", calls)
			}
		})
	}
}

func TestMissingCredentialsInResponsesAreRejected(t *testing.T) {
	for _, stage := range []string{"signIn", "gen-access-token", "createToken"} {
		t.Run(stage, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, stage):
					if stage == "gen-access-token" {
						_, _ = fmt.Fprint(w, `{}`)
					} else {
						_, _ = fmt.Fprint(w, `{"kind":12,"id":1,"data":{}}`)
					}
				case strings.HasSuffix(r.URL.Path, "signIn"):
					_, _ = fmt.Fprint(w, `{"kind":12,"id":1,"data":{"sessionId":"session"}}`)
				case strings.HasSuffix(r.URL.Path, "gen-access-token"):
					_, _ = fmt.Fprint(w, `{"accessToken":"access"}`)
				default:
					_, _ = fmt.Fprint(w, `{"kind":12,"id":1}`)
				}
			}))
			defer server.Close()

			_, err := createViaAPI(context.Background(), server.URL, CreateTestUserOpts{Email: "test@codesphere.com", Password: "password"})
			if err == nil {
				t.Fatal("missing response credential accepted")
			}
		})
	}
}

func TestGeneratedCredentialsAreUniqueAndCanBePersistedPrivately(t *testing.T) {
	a, err := (CreateTestUserOpts{}).defaults()
	if err != nil {
		t.Fatal(err)
	}

	b, err := (CreateTestUserOpts{}).defaults()
	if err != nil {
		t.Fatal(err)
	}

	if a.Email == b.Email || a.Password == b.Password || !strings.HasPrefix(a.Password, "Aa1!") {
		t.Error("generated credentials are not unique and strong")
	}

	dir := t.TempDir()

	file := filepath.Join(dir, "test-user.json")
	if err := os.WriteFile(file, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}

	want := &TestUserResult{Email: a.Email, PlaintextPassword: a.Password, PlaintextAPIToken: "CS_token"}
	if _, err := WriteResultToFile(want, dir); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}

	if info.Mode().Perm() != 0600 {
		t.Errorf("mode = %v", info.Mode().Perm())
	}

	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}

	var got TestUserResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}

	if got != *want {
		t.Error("persisted credentials differ")
	}
}

func TestServiceTargetPortResolvesNamedAndNumericPorts(t *testing.T) {
	pod := &corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Ports: []corev1.ContainerPort{{Name: "http", ContainerPort: 3010}}}}}}
	for _, target := range []intstr.IntOrString{intstr.FromInt32(3010), intstr.FromString("http"), intstr.FromInt32(0)} {
		svc := &corev1.Service{Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 3010, TargetPort: target}}}}

		port, err := serviceTargetPort(svc, pod, 3010)
		if err != nil || port != 3010 {
			t.Fatalf("target %v: port=%d err=%v", target, port, err)
		}

		if _, err := serviceTargetPort(svc, pod, 80); err == nil {
			t.Error("missing service port accepted")
		}
	}

	svc := &corev1.Service{Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 3010, TargetPort: intstr.FromString("missing")}}}}
	if _, err := serviceTargetPort(svc, pod, 3010); err == nil {
		t.Error("missing named container port accepted")
	}
}
