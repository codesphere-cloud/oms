// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package postgres installs and provides bootstrap access to a CloudNativePG database.
package postgres

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	streamhttp "k8s.io/streaming/pkg/httpstream"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Forward exposes a PostgreSQL pod on a loopback port for the lifetime of a bootstrap step.
type Forward struct {
	Port   int
	stop   chan struct{}
	done   chan struct{}
	runErr error
	once   sync.Once
}

// Close stops the port-forward and waits for its goroutine to exit.
func (f *Forward) Close() error {
	f.once.Do(func() { close(f.stop) })
	<-f.done

	if f.runErr != nil {
		return fmt.Errorf("PostgreSQL port-forward stopped: %w", f.runErr)
	}

	return nil
}

// StartPortForward follows kubectl's client-go WebSocket/SPDY forwarding path.
func (b *Installer) StartPortForward(ctx context.Context, config *rest.Config) (*Forward, error) {
	namespace, serviceName := b.namespace(), b.serviceName()

	service := &corev1.Service{}
	if err := b.KubeClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: serviceName}, service); err != nil {
		return nil, fmt.Errorf("get PostgreSQL service %s/%s: %w", namespace, serviceName, err)
	}

	if len(service.Spec.Selector) == 0 {
		return nil, fmt.Errorf("PostgreSQL service %s/%s has no pod selector", namespace, serviceName)
	}

	pods := &corev1.PodList{}
	if err := b.KubeClient.List(ctx, pods, client.InNamespace(namespace), client.MatchingLabels(service.Spec.Selector)); err != nil {
		return nil, fmt.Errorf("list PostgreSQL pods: %w", err)
	}

	var podName string

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning && podReady(&pod) {
			podName = pod.Name
			break
		}
	}

	if podName == "" {
		return nil, fmt.Errorf("no ready PostgreSQL pod backs service %s/%s", namespace, serviceName)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create Kubernetes client for PostgreSQL port-forward: %w", err)
	}

	request := clientset.CoreV1().RESTClient().Post().Resource("pods").Namespace(namespace).Name(podName).SubResource("portforward")

	transport, upgrader, err := spdy.RoundTripperFor(config)
	if err != nil {
		return nil, fmt.Errorf("create PostgreSQL SPDY transport: %w", err)
	}

	spdyDialer := spdy.NewDialerForStreaming(upgrader, &http.Client{Transport: transport}, http.MethodPost, request.URL())

	websocketDialer, err := portforward.NewSPDYOverWebsocketDialerForStreaming(request.URL(), config)
	if err != nil {
		return nil, fmt.Errorf("create PostgreSQL WebSocket dialer: %w", err)
	}

	dialer := portforward.NewFallbackDialerForStreaming(websocketDialer, spdyDialer, func(err error) bool {
		return streamhttp.IsUpgradeFailure(err) || streamhttp.IsHTTPSProxyError(err)
	})
	stop := make(chan struct{})
	ready := make(chan struct{})

	forwarder, err := portforward.NewOnAddressesForStreaming(dialer, []string{"127.0.0.1"}, []string{"0:5432"}, stop, ready, io.Discard, io.Discard)
	if err != nil {
		return nil, fmt.Errorf("create PostgreSQL port-forward: %w", err)
	}

	forward := &Forward{stop: stop, done: make(chan struct{})}
	go func() {
		forward.runErr = forwarder.ForwardPorts()
		close(forward.done)
	}()

	select {
	case <-ready:
		ports, err := forwarder.GetPorts()
		if err != nil || len(ports) != 1 {
			close(stop)
			<-forward.done

			if err != nil {
				return nil, fmt.Errorf("get PostgreSQL forwarded port: %w", err)
			}

			return nil, fmt.Errorf("expected one PostgreSQL forwarded port, got %d", len(ports))
		}

		forward.Port = int(ports[0].Local)

		go func() {
			select {
			case <-ctx.Done():
				forward.once.Do(func() { close(stop) })
			case <-forward.done:
			}
		}()

		return forward, nil
	case <-forward.done:
		return nil, fmt.Errorf("start PostgreSQL port-forward: %w", forward.runErr)
	case <-ctx.Done():
		close(stop)
		<-forward.done

		return nil, fmt.Errorf("start PostgreSQL port-forward: %w", ctx.Err())
	}
}

func podReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}

	return false
}
