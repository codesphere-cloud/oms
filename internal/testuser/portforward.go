// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

// Package testuser provisions smoke-test credentials through the internal Codesphere API.
package testuser

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"sync"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

func forwardAuthService(ctx context.Context, cfg *rest.Config, opts CreateTestUserOpts) (string, func(), error) {
	if cfg == nil {
		return "", nil, fmt.Errorf("kubernetes REST config is required")
	}

	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return "", nil, fmt.Errorf("creating Kubernetes client: %w", err)
	}

	svc, err := client.CoreV1().Services(opts.Namespace).Get(ctx, opts.Service, metav1.GetOptions{})
	if err != nil {
		return "", nil, fmt.Errorf("reading auth service %s/%s: %w", opts.Namespace, opts.Service, err)
	}

	if len(svc.Spec.Selector) == 0 {
		return "", nil, fmt.Errorf("auth service must select pods for port-forwarding")
	}

	pods, err := client.CoreV1().Pods(opts.Namespace).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(svc.Spec.Selector).String()})
	if err != nil {
		return "", nil, fmt.Errorf("listing auth service pods: %w", err)
	}

	sort.Slice(pods.Items, func(i, j int) bool { return pods.Items[i].Name < pods.Items[j].Name })

	var pod *corev1.Pod

	for i := range pods.Items {
		candidate := &pods.Items[i]
		if candidate.DeletionTimestamp != nil || candidate.Status.Phase != corev1.PodRunning {
			continue
		}

		for _, condition := range candidate.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				pod = candidate
				break
			}
		}

		if pod != nil {
			break
		}
	}

	if pod == nil {
		return "", nil, fmt.Errorf("no ready auth service pod found in namespace %s", opts.Namespace)
	}

	targetPort, err := serviceTargetPort(svc, pod, opts.Port)
	if err != nil {
		return "", nil, err
	}

	transport, upgrader, err := spdy.RoundTripperFor(cfg)
	if err != nil {
		return "", nil, fmt.Errorf("creating port-forward transport: %w", err)
	}

	url := client.CoreV1().RESTClient().Post().Namespace(opts.Namespace).Resource("pods").Name(pod.Name).SubResource("portforward").URL()
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, url)
	stop, ready := make(chan struct{}), make(chan struct{})

	var once sync.Once

	closeForward := func() { once.Do(func() { close(stop) }) }

	fw, err := portforward.NewOnAddresses(dialer, []string{"127.0.0.1"}, []string{"0:" + strconv.Itoa(targetPort)}, stop, ready, io.Discard, io.Discard)
	if err != nil {
		return "", nil, fmt.Errorf("creating auth service port-forward: %w", err)
	}

	done := make(chan error, 1)
	go func() { done <- fw.ForwardPorts() }()
	go func() {
		select {
		case <-ctx.Done():
			closeForward()
		case <-stop:
		}
	}()

	select {
	case err := <-done:
		closeForward()
		return "", nil, fmt.Errorf("auth service port-forward stopped before becoming ready: %v", err)
	case <-ctx.Done():
		closeForward()
		return "", nil, fmt.Errorf("waiting for auth service port-forward: %w", ctx.Err())
	case <-ready:
	}

	ports, err := fw.GetPorts()
	if err != nil || len(ports) != 1 {
		closeForward()
		return "", nil, fmt.Errorf("could not determine auth service forwarded port")
	}

	return "http://127.0.0.1:" + strconv.Itoa(int(ports[0].Local)), closeForward, nil
}

func serviceTargetPort(svc *corev1.Service, pod *corev1.Pod, port int) (int, error) {
	for _, servicePort := range svc.Spec.Ports {
		if int(servicePort.Port) != port {
			continue
		}

		if servicePort.Protocol != "" && servicePort.Protocol != corev1.ProtocolTCP {
			return 0, fmt.Errorf("auth service port must use TCP")
		}

		if servicePort.TargetPort.Type == intstr.Int {
			if servicePort.TargetPort.IntVal == 0 {
				return port, nil
			}

			return int(servicePort.TargetPort.IntVal), nil
		}

		for _, container := range pod.Spec.Containers {
			for _, containerPort := range container.Ports {
				if containerPort.Name == servicePort.TargetPort.StrVal && (containerPort.Protocol == "" || containerPort.Protocol == corev1.ProtocolTCP) {
					return int(containerPort.ContainerPort), nil
				}
			}
		}

		return 0, fmt.Errorf("auth service target port %q not found in pod", servicePort.TargetPort.StrVal)
	}

	return 0, fmt.Errorf("auth service has no port %d", port)
}
